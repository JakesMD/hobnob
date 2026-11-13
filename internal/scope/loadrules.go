package scope

import (
	"fmt"

	"hobnob/internal/config"
	"hobnob/internal/eval"
)

// checkLoadRules checks cfg's const:, vars: and env: blocks against the rules
// that follow from the layer order resolve walks: const: is a closed world,
// vars: is already the fallback so never reads itself, and env: paths can't
// read vars:, which reads them. Each file is checked against its own blocks
// only, before its chain evaluates anything.
func checkLoadRules(cfg *config.ConfigFile) error {
	if err := checkConstClosedWorld(cfg.ConstEntries); err != nil {
		return err
	}
	if err := checkVarsNoSelfReference(cfg.VarEntries); err != nil {
		return err
	}
	return checkEnvPathsDontReferenceVars(cfg)
}

// The built-in variables: Load sets both on the base layer, before any
// file's chain runs, so they are the only names a const: may read besides
// earlier const: entries.
const (
	fileDirVar       = "HOBNOB_FILE_DIR"
	invocationDirVar = "HOBNOB_INVOCATION_DIR"
)

var builtinVars = map[string]bool{
	fileDirVar:       true,
	invocationDirVar: true,
}

// checkConstClosedWorld enforces that a const: entry may reference only
// earlier const: entries in the same block plus the builtins — never a
// lower-priority var (env, env files, CLI args) and never itself, since
// "itself" is never among the earlier keys. This is the check that keeps
// const: from rotting back into the old (pre-v0.3) vars: block, where an
// entry silently reading a lower layer (a defaults-hack in disguise) was
// indistinguishable from a real constant.
func checkConstClosedWorld(entries []config.SetEntry) error {
	allowed := make(map[string]bool, len(entries)+len(builtinVars))
	declared := make(map[string]bool, len(entries))
	for key := range builtinVars {
		allowed[key] = true
	}
	for _, entry := range entries {
		// A duplicate key inside one const: block is always a mistake —
		// unlike set:, where a step re-assigning a var later in the timeline
		// is ordinary, const: is a one-time declaration, so two entries with
		// the same key can only be a copy-paste error.
		if declared[entry.Key] {
			return fmt.Errorf("const: %s declared twice in the same block", entry.Key)
		}
		declared[entry.Key] = true
		refs, err := entry.Val.Refs()
		if err != nil {
			return fmt.Errorf("const: %s: %w", entry.Key, err)
		}
		for _, ref := range refs {
			if !allowed[ref] {
				return fmt.Errorf("const: %s references .%s, which is not a file constant\n  hint: const: entries are fixed values — use vars: for an overridable one", entry.Key, ref)
			}
		}
		allowed[entry.Key] = true
	}
	return nil
}

// checkVarsNoSelfReference rejects a vars: entry that references its own
// key — the old {{ .HOST | default "localhost" }} defaults-hack rewritten
// under the new name. vars: already IS the fallback layer, so the pattern
// is always redundant, never meaningful: at the point this entry evaluates,
// referencing its own name would only ever see whatever the OS-environment
// base layer holds, since vars: sits directly above it and nothing else has
// run yet.
func checkVarsNoSelfReference(entries []config.SetEntry) error {
	for _, entry := range entries {
		refs, err := entry.Val.Refs()
		if err != nil {
			return fmt.Errorf("vars: %s: %w", entry.Key, err)
		}
		for _, ref := range refs {
			if ref == entry.Key {
				return fmt.Errorf("vars: %s references itself — vars: is already the fallback layer\n  hint: write `- %s: <value>`", entry.Key, entry.Key)
			}
		}
	}
	return nil
}

// checkEnvPathsDontReferenceVars rejects an env: file path template that
// references a vars: name. Upward reads let vars: read env: files (see
// docs/adr/0001-upward-reads-between-scope-layers.md), which makes the
// reverse — an env: path driven by a vars: default — a cycle if it were
// allowed, so it's a load-time error naming the rule instead of a runtime
// "path not found" that wouldn't explain why.
func checkEnvPathsDontReferenceVars(cfg *config.ConfigFile) error {
	if len(cfg.VarEntries) == 0 {
		return nil
	}
	varNames := make(map[string]bool, len(cfg.VarEntries))
	for _, entry := range cfg.VarEntries {
		varNames[entry.Key] = true
	}
	for _, entry := range cfg.EnvFileTmpls {
		refs, err := eval.ReferencedVars(entry.PathTmpl)
		if err != nil {
			return fmt.Errorf("env: %q: %w", entry.PathTmpl, err)
		}
		for _, ref := range refs {
			if varNames[ref] {
				return fmt.Errorf("env: %q references .%s, declared in vars: — an env: path can only read CLI args, const:, the OS env and the built-in variables\n  hint: select the file with a CLI arg, the OS env, or const: instead", entry.PathTmpl, ref)
			}
		}
	}
	return nil
}
