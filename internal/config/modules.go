package config

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"hobnob/internal/eval"
	"hobnob/internal/value"

	"gopkg.in/yaml.v3"
)

// applyModuleFlag applies a show/hide/flatten key-value pair to m.
// Used for both sibling-key form (- docker: ./path\n  show: [...]) and
// nested object form (- docker: {path: ..., show: [...]}).
func applyModuleFlag(module *ModuleEntry, key string, val *yaml.Node) error {
	switch key {
	case "show":
		if val.Kind != yaml.SequenceNode {
			return fmt.Errorf("module show must be a sequence")
		}
		for _, child := range val.Content {
			module.ShowTmpls = append(module.ShowTmpls, child.Value)
		}
	case "hide":
		if val.Kind != yaml.SequenceNode {
			return fmt.Errorf("module hide must be a sequence")
		}
		for _, child := range val.Content {
			module.HideTmpls = append(module.HideTmpls, child.Value)
		}
	case "flatten":
		module.FlattenTmpl = val.Value
	}
	return nil
}

func parseModulesNode(node *yaml.Node) ([]ModuleEntry, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("modules must be a sequence")
	}
	var moduleEntries []ModuleEntry
	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("module entry must be a mapping")
		}
		var module ModuleEntry
		for _, entry := range mapEntries(item.Content) {
			key, val := entry.Key, entry.Val
			// "show", "hide", "flatten" are reserved at the top-level mapping key
			// position. Using them as module prefixes always produces a parse error
			// (not a silent shadow): "show"/"hide" fail the sequence check; "flatten"
			// sets FlattenTmpl, leaving Prefix empty → "module entry missing prefix key".
			switch key {
			case "show", "hide", "flatten":
				if err := applyModuleFlag(&module, key, val); err != nil {
					return nil, err
				}
			default:
				module.Prefix = key
				if val.Kind == yaml.MappingNode {
					for _, sub := range mapEntries(val.Content) {
						if sub.Key == "path" {
							module.FileTmpl = sub.Val.Value
						} else if err := applyModuleFlag(&module, sub.Key, sub.Val); err != nil {
							return nil, err
						}
					}
				} else {
					module.FileTmpl = val.Value
				}
			}
		}
		if module.Prefix == "" {
			return nil, fmt.Errorf("module entry missing prefix key")
		}
		moduleEntries = append(moduleEntries, module)
	}
	return moduleEntries, nil
}

// LoadModules resolves module entries against scope, loads each module file,
// and merges the resulting tasks into cfg. Must be called after BuildScope.
// A module's own env: block is sourced for that module's own subtree only —
// same rule as vars: (see GUIDE.md Modules > Scoping), so it never leaks into
// the parent's vars/secrets.
func LoadModules(ctx context.Context, cfg *ConfigFile, vars map[string]value.Value, secrets map[string]bool) error {
	ancestors := map[string]bool{}
	if cfg.FilePath != "" {
		ancestors[cfg.FilePath] = true
	}
	return loadModules(ctx, cfg, vars, secrets, ancestors)
}

func loadModules(ctx context.Context, cfg *ConfigFile, vars map[string]value.Value, secrets map[string]bool, ancestors map[string]bool) error {
	for _, module := range cfg.Modules {
		moduleCfg, moduleVars, moduleSecrets, absPath, err := resolveModuleFile(ctx, cfg, module, vars, secrets, ancestors)
		if err != nil {
			return err
		}

		newAncestors := eval.CloneMap(ancestors)
		newAncestors[absPath] = true

		if err := loadModules(ctx, moduleCfg, moduleVars, moduleSecrets, newAncestors); err != nil {
			return fmt.Errorf("module %q: %w", module.Prefix, err)
		}

		if err := registerModuleTasks(cfg, module, moduleCfg, vars); err != nil {
			return err
		}
	}
	return nil
}

// resolveModuleFile locates and parses a module's file, then builds the
// module-local scope (vars/secrets copied from the parent, plus anything the
// module's own env: block sources) — everything loadModules needs before it
// can recurse into the module's own sub-modules.
func resolveModuleFile(ctx context.Context, cfg *ConfigFile, module ModuleEntry, vars map[string]value.Value, secrets map[string]bool, ancestors map[string]bool) (moduleCfg *ConfigFile, moduleVars map[string]value.Value, moduleSecrets map[string]bool, absPath string, err error) {
	filePath, err := eval.EvalTemplate(module.FileTmpl, vars)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("module %q file path: %w", module.Prefix, err)
	}
	filePath = eval.ResolvePath(filePath, cfg.TaskfileDir)
	absPath, err = filepath.Abs(filePath)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("module %q: resolve path: %w", module.Prefix, err)
	}
	if ancestors[absPath] {
		return nil, nil, nil, "", fmt.Errorf("module %q: circular import: %s", module.Prefix, absPath)
	}

	moduleCfg, err = ParseConfig(filePath)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("module %q: %w", module.Prefix, err)
	}

	// moduleVars/moduleSecrets stay local to this module's own subtree — a
	// module's env:/vars:/const: blocks are private the same way its tasks
	// are, unless flatten:/show: makes a task visible.
	moduleVars = eval.CloneMap(vars)
	moduleSecrets = eval.CloneMap(secrets)

	// const: first, into its own hard-override layer — a module's own const:
	// always wins in its subtree, never just a default (see
	// ConfigFile.ModuleConstLayer). Evaluated first, same as the root chain
	// (docs/adr/0001), since it's a closed world: it can only reference
	// earlier const: entries and the builtins, never env: files or vars:.
	moduleCfg.ModuleConstLayer = make(map[string]value.Value)
	moduleCfg.ModuleConstLayerSecrets = make(map[string]bool)
	if err := applyModuleSetEntries(moduleCfg.ConstEntries, moduleVars, moduleSecrets, moduleCfg.ModuleConstLayer, moduleCfg.ModuleConstLayerSecrets, nil); err != nil {
		return nil, nil, nil, "", fmt.Errorf("module %q: %w", module.Prefix, err)
	}

	// locked tracks every name this module's own const: has claimed —
	// resolved before env: files and vars:, so its final values are what
	// both can read (an env: path referencing a vars: name is still a
	// load-time error, checked by checkEnvPathsDontReferenceVars on this
	// same moduleCfg since ParseConfig above already ran it).
	locked := make(map[string]bool, len(moduleCfg.ConstEntries))
	for _, entry := range moduleCfg.ConstEntries {
		locked[entry.Key] = true
	}

	moduleCfg.ModuleLayer = make(map[string]value.Value)
	moduleCfg.ModuleLayerSecrets = make(map[string]bool)

	envVars, envSecrets, err := LoadEnvFiles(ctx, moduleCfg.EnvFileTmpls, moduleCfg.TaskfileDir, moduleVars)
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("module %q: %w", module.Prefix, err)
	}
	for key, envVal := range envVars {
		if locked[key] {
			continue // this module's own const: already claimed it
		}
		moduleVars[key] = value.Str(envVal)
		moduleCfg.ModuleLayer[key] = value.Str(envVal)
		locked[key] = true
		if envSecrets[key] {
			moduleSecrets[key] = true
			moduleCfg.ModuleLayerSecrets[key] = true
		}
	}

	// vars: last, since it's the fallback layer and needs to see everything
	// above it — this module's own const: and env: files — to build a real
	// default from. Skipped entirely (not evaluated) for a name locked
	// already claims; still free to overwrite whatever this module inherited
	// from its parent scope, same as today (moduleVars carries no ambient
	// tracking for that inherited layer, unlike the root chain's OS env).
	if err := applyModuleSetEntries(moduleCfg.VarEntries, moduleVars, moduleSecrets, moduleCfg.ModuleLayer, moduleCfg.ModuleLayerSecrets, locked); err != nil {
		return nil, nil, nil, "", fmt.Errorf("module %q: %w", module.Prefix, err)
	}

	return moduleCfg, moduleVars, moduleSecrets, absPath, nil
}

// applyModuleSetEntries evaluates entries (a module's own const: or vars:
// block) top-to-bottom into vars/secrets — same sequential rule set:
// follows at step scope, each entry seeing everything set before it,
// including the parent scope this module inherited — and records every
// entry into layer/layerSecrets too, so the caller can also apply it, via
// whichever runtime rule fits that block, to the scope a module task
// actually executes with (see runner.applyModuleLayer). locked, when
// non-nil, is both a skip check and a write target: an entry whose name
// locked already claims (this module's own const: or env: files, resolved
// before vars:) is skipped without being evaluated, rather than
// unconditionally overwriting it — see resolveModuleFile — and every entry
// this call does evaluate claims its own name in locked too, so a later
// call (vars: after const:/env: files) sees it.
func applyModuleSetEntries(entries []SetEntry, vars map[string]value.Value, secrets map[string]bool, layer map[string]value.Value, layerSecrets map[string]bool, locked map[string]bool) error {
	for _, entry := range entries {
		if locked != nil && locked[entry.Key] {
			continue
		}
		val, err := EvalSetEntry(entry, func(tmpl string) (value.Value, error) {
			return eval.EvalValue(tmpl, vars)
		})
		if err != nil {
			return fmt.Errorf("%s: %w", entry.Key, err)
		}
		vars[entry.Key] = val
		layer[entry.Key] = val
		if locked != nil {
			locked[entry.Key] = true
		}
		if entry.Secret {
			secrets[entry.Key] = true
			layerSecrets[entry.Key] = true
		}
	}
	return nil
}

// registerModuleTasks applies module's show/hide/flatten filters to moduleCfg's
// tasks and registers the survivors into cfg under their prefixed (and,
// if flattened, bare) names.
func registerModuleTasks(cfg *ConfigFile, module ModuleEntry, moduleCfg *ConfigFile, vars map[string]value.Value) error {
	showSet, err := evalStringSet(module.ShowTmpls, vars)
	if err != nil {
		return fmt.Errorf("module %q show: %w", module.Prefix, err)
	}
	hideSet, err := evalStringSet(module.HideTmpls, vars)
	if err != nil {
		return fmt.Errorf("module %q hide: %w", module.Prefix, err)
	}

	flatten := false
	if module.FlattenTmpl != "" {
		flatVal, err := eval.EvalTemplate(module.FlattenTmpl, vars)
		if err != nil {
			return fmt.Errorf("module %q flatten: %w", module.Prefix, err)
		}
		flatten = flatVal == "true"
	}

	isInternal := strings.HasPrefix(module.Prefix, "_")

	for _, taskName := range moduleCfg.TaskNames {
		if len(showSet) > 0 {
			if _, ok := showSet[taskName]; !ok {
				continue
			}
		}
		if _, ok := hideSet[taskName]; ok {
			continue
		}
		if strings.HasPrefix(taskName, "_") {
			continue
		}

		task := moduleCfg.Tasks[taskName]
		if task.Cfg == nil {
			task.Cfg = moduleCfg
		}
		if isInternal {
			task.Hidden = true
		}

		prefixedName := module.Prefix + ":" + taskName

		if flatten && !isInternal {
			if _, exists := cfg.Tasks[taskName]; !exists {
				// flat alias becomes the visible name; prefixed is a hidden alias
				cfg.Tasks[taskName] = task
				cfg.TaskNames = append(cfg.TaskNames, taskName)

				taskHidden := task
				taskHidden.Hidden = true
				cfg.Tasks[prefixedName] = taskHidden
				cfg.TaskNames = append(cfg.TaskNames, prefixedName)
			} else {
				// collision with existing task — register prefixed name as visible
				cfg.Tasks[prefixedName] = task
				cfg.TaskNames = append(cfg.TaskNames, prefixedName)
			}
		} else {
			cfg.Tasks[prefixedName] = task
			cfg.TaskNames = append(cfg.TaskNames, prefixedName)
		}
	}
	return nil
}

func evalStringSet(tmpls []string, scope map[string]value.Value) (map[string]struct{}, error) {
	if len(tmpls) == 0 {
		return nil, nil
	}
	set := make(map[string]struct{}, len(tmpls))
	for _, tmpl := range tmpls {
		val, err := eval.EvalTemplate(tmpl, scope)
		if err != nil {
			return nil, err
		}
		set[val] = struct{}{}
	}
	return set, nil
}
