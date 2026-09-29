package scope

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/value"
)

// FileScopes holds every module's file scope, keyed on the module's own
// parsed ConfigFile — the value a module task carries as Task.Cfg. The root
// file has no entry: its file scope is already the scope Load returns.
type FileScopes map[*config.ConfigFile]*fileScope

// fileScope is what one module file's own const:, env: files and vars:
// contributed on top of its importer's scope, split by how each lands at run
// time. parent is the importing module's file scope (nil when the importer is
// the root file), so Apply can lay the whole chain down, outermost first.
type fileScope struct {
	consts         map[string]value.Value
	constSecrets   map[string]bool
	defaults       map[string]value.Value
	defaultSecrets map[string]bool
	parent         *fileScope
}

func newFileScope(parent *fileScope) *fileScope {
	return &fileScope{
		consts:         make(map[string]value.Value),
		constSecrets:   make(map[string]bool),
		defaults:       make(map[string]value.Value),
		defaultSecrets: make(map[string]bool),
		parent:         parent,
	}
}

// Apply lays cfg's file scope onto scope before a task belonging to cfg runs,
// preceded by every enclosing module's, outermost first — a module's file
// scope covers its whole subtree, so a nested module task sees what its
// parent module contributed too. const: always overwrites (the nearest
// declaration wins); env: files and vars: fill only names nothing higher has
// claimed. A cfg with no file scope (the root file, or nil) is a no-op.
// Re-applying on a nested call into the same module is idempotent: a filled
// default is no longer ambient, and const: always writes the same value.
func (fileScopes FileScopes) Apply(scope *Scope, cfg *config.ConfigFile) {
	link := fileScopes[cfg]
	var chain []*fileScope
	for ; link != nil; link = link.parent {
		chain = append(chain, link)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		link := chain[i]
		for key, val := range link.defaults {
			scope.SetIfDefault(key, val, link.defaultSecrets[key])
		}
		for key, val := range link.consts {
			scope.Set(key, val, link.constSecrets[key])
		}
	}
}

// Load builds the root scope for cfg and resolves the file scope of every
// module it imports, recursively. The root chain's base is the OS env plus
// the two built-ins, and it alone takes cliVars. Each module's chain is
// resolved against its importer's resolved scope — never a caller's with: or
// set:, which don't exist yet at load — and so is every load-time template
// on the import itself (path:, show:, hide:, flatten:).
func Load(ctx context.Context, cfg *config.ConfigFile, cliVars map[string]string, invocationDir string) (*Scope, FileScopes, error) {
	base := &Scope{
		Vars:    make(map[string]value.Value),
		Secrets: make(map[string]bool),
		Ambient: make(map[string]bool),
	}
	for _, envEntry := range os.Environ() {
		if key, val, ok := eval.SplitKV(envEntry); ok {
			base.Vars[key] = value.Str(val)
			base.Ambient[key] = true
		}
	}
	base.Vars["HOBNOB_FILE_DIR"] = value.Str(cfg.TaskfileDir)
	base.Vars["HOBNOB_INVOCATION_DIR"] = value.Str(invocationDir)

	root, _, err := resolve(ctx, cfg, base, cliVars, nil)
	if err != nil {
		return nil, nil, err
	}

	fileScopes := FileScopes{}
	ancestors := map[string]bool{}
	if cfg.FilePath != "" {
		ancestors[cfg.FilePath] = true
	}
	if err := loadModules(ctx, cfg, root, nil, ancestors, fileScopes); err != nil {
		return nil, nil, err
	}
	return root, fileScopes, nil
}

// loadModules walks cfg's imports: for each, evaluate its path against
// importer, parse it, resolve its file scope, recurse into its own imports,
// then register its tasks into cfg.
func loadModules(ctx context.Context, cfg *config.ConfigFile, importer *Scope, importerFS *fileScope, ancestors map[string]bool, fileScopes FileScopes) error {
	for _, module := range cfg.Modules {
		filePath, err := eval.EvalTemplate(module.FileTmpl, importer.Vars)
		if err != nil {
			return fmt.Errorf("module %q file path: %w", module.Prefix, err)
		}
		filePath = eval.ResolvePath(filePath, cfg.TaskfileDir)
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return fmt.Errorf("module %q: resolve path: %w", module.Prefix, err)
		}
		if ancestors[absPath] {
			return fmt.Errorf("module %q: circular import: %s", module.Prefix, absPath)
		}

		moduleCfg, err := config.ParseConfig(filePath)
		if err != nil {
			return fmt.Errorf("module %q: %w", module.Prefix, err)
		}
		moduleScope, moduleFS, err := resolve(ctx, moduleCfg, importer, nil, importerFS)
		if err != nil {
			return fmt.Errorf("module %q: %w", module.Prefix, err)
		}
		fileScopes[moduleCfg] = moduleFS

		moduleAncestors := eval.CloneMap(ancestors)
		moduleAncestors[absPath] = true
		if err := loadModules(ctx, moduleCfg, moduleScope, moduleFS, moduleAncestors, fileScopes); err != nil {
			return fmt.Errorf("module %q: %w", module.Prefix, err)
		}

		if err := config.RegisterModuleTasks(cfg, module, moduleCfg, importer.Vars); err != nil {
			return err
		}
	}
	return nil
}

// resolve is the one copy of the ADR-0001 chain (docs/adr/0001), shared by
// the root file and every module. It resolves cfg's own layers on a copy of
// base in the reverse of precedence order, so each layer's templates read the
// final values of every layer above it: const: first (a closed world, checked
// at load by config), then cliVars (root only; plain text, never sniffed),
// then env: files (whose paths may read anything above but never a vars:
// name, also checked at load), then vars: last. Every layer below const:
// writes a name only when it's unclaimed (Scope.claimed), and a claimed vars:
// entry is skipped without being evaluated, since a template that only makes
// sense standing alone shouldn't run just because it lost.
//
// For a module, base is its importer's resolved scope, so a module's env:
// file loses to anything the importer set that isn't still ambient —
// including the importer's own vars: defaults. The returned fileScope records
// what this file itself wrote, for FileScopes.Apply to replay at run time.
func resolve(ctx context.Context, cfg *config.ConfigFile, base *Scope, cliVars map[string]string, parent *fileScope) (*Scope, *fileScope, error) {
	scope := base.Copy()
	own := newFileScope(parent)

	for _, entry := range cfg.ConstEntries {
		val, err := evalSetEntry(scope, entry)
		if err != nil {
			return nil, nil, fmt.Errorf("const: %s: %w", entry.Key, err)
		}
		scope.Set(entry.Key, val, entry.Secret)
		delete(scope.Ambient, entry.Key)
		own.consts[entry.Key] = val
		own.constSecrets[entry.Key] = entry.Secret
	}

	for key, val := range cliVars {
		scope.SetIfDefault(key, value.Str(val), false)
	}

	envFileVars, envFileSecrets, err := loadEnvFiles(ctx, cfg.EnvFileTmpls, cfg.TaskfileDir, scope.Vars)
	if err != nil {
		return nil, nil, err
	}
	for key, raw := range envFileVars {
		if scope.claimed(key) {
			continue
		}
		scope.SetIfDefault(key, value.Str(raw), envFileSecrets[key])
		own.defaults[key] = value.Str(raw)
		own.defaultSecrets[key] = envFileSecrets[key]
	}

	for _, entry := range cfg.VarEntries {
		if scope.claimed(entry.Key) {
			continue
		}
		val, err := evalSetEntry(scope, entry)
		if err != nil {
			return nil, nil, fmt.Errorf("vars: %s: %w", entry.Key, err)
		}
		scope.SetIfDefault(entry.Key, val, entry.Secret)
		own.defaults[entry.Key] = val
		own.defaultSecrets[entry.Key] = entry.Secret
	}

	return scope, own, nil
}

func evalSetEntry(scope *Scope, entry config.SetEntry) (value.Value, error) {
	return config.EvalSetEntry(entry, func(tmpl string) (value.Value, error) {
		return eval.EvalValue(tmpl, scope.Vars)
	})
}
