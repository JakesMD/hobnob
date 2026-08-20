package config

import (
	"fmt"

	"hobnob/internal/eval"
)

// referencedVarsInSetEntry returns every scope-var name entry's value
// references — walking into a map/list literal's leaves when entry holds
// one (ValNode), or parsing the plain template text otherwise (ValTmpl).
// Shared by const:'s closed-world check and vars:'s self-reference check,
// the two file-level rules that need to see every name a const:/vars: entry
// touches, not just its outermost token.
func referencedVarsInSetEntry(entry SetEntry) ([]string, error) {
	if entry.ValNode != nil {
		return referencedVarsInJSONNode(*entry.ValNode)
	}
	return eval.ReferencedVars(entry.ValTmpl)
}

func referencedVarsInJSONNode(n JSONNode) ([]string, error) {
	var out []string
	switch n.Kind {
	case JSONString:
		refs, err := eval.ReferencedVars(n.Tmpl)
		if err != nil {
			return nil, err
		}
		out = append(out, refs...)
	case JSONObject:
		for _, field := range n.Fields {
			refs, err := referencedVarsInJSONNode(field.Node)
			if err != nil {
				return nil, err
			}
			out = append(out, refs...)
		}
	case JSONArray:
		for _, elem := range n.Elements {
			refs, err := referencedVarsInJSONNode(elem)
			if err != nil {
				return nil, err
			}
			out = append(out, refs...)
		}
	}
	return out, nil
}

// builtinVars are the only names const: may reference besides earlier
// const: entries — the two vars BuildScope always sets before any file-level
// layer runs (see cli.BuildScope).
var builtinVars = map[string]bool{
	"HOBNOB_FILE_DIR":       true,
	"HOBNOB_INVOCATION_DIR": true,
}

// checkConstClosedWorld enforces that a const: entry may reference only
// earlier const: entries in the same block plus the builtins — never a
// lower-priority var (env, env files, CLI args) and never itself, since
// "itself" is never among the earlier keys. This is the check that keeps
// const: from rotting back into the old (pre-v0.3) vars: block, where an
// entry silently reading a lower layer (a defaults-hack in disguise) was
// indistinguishable from a real constant.
func checkConstClosedWorld(entries []SetEntry) error {
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
		refs, err := referencedVarsInSetEntry(entry)
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
func checkVarsNoSelfReference(entries []SetEntry) error {
	for _, entry := range entries {
		refs, err := referencedVarsInSetEntry(entry)
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

// checkConstNamesNotShadowed rejects a set:/get:/into:/loop: target
// anywhere in cfg's own tasks that collides with one of cfg's own const:
// names. Without this, const: would only be constant from outside the
// file — the timeline still outranks it in the precedence chain, so a
// task's own set: could quietly overwrite one. Scoped per-file: a module's
// tasks are checked against the module's own const: block, not the
// parent's, matching every other file-scoped rule (env:, vars:, const:
// itself).
func checkConstNamesNotShadowed(cfg *ConfigFile) error {
	if len(cfg.ConstEntries) == 0 {
		return nil
	}
	names := make(map[string]bool, len(cfg.ConstEntries))
	for _, entry := range cfg.ConstEntries {
		names[entry.Key] = true
	}
	for _, taskName := range cfg.TaskNames {
		if err := checkStepsDontShadow(taskName, cfg.Tasks[taskName].Steps, names); err != nil {
			return err
		}
	}
	return nil
}

func checkStepsDontShadow(taskName string, steps []Step, names map[string]bool) error {
	shadow := func(target string) error {
		if names[target] {
			return fmt.Errorf("task %s sets %s, declared in const: — pick another name", taskName, target)
		}
		return nil
	}
	for _, step := range steps {
		for _, entry := range step.SetEntries {
			if err := shadow(entry.Key); err != nil {
				return err
			}
		}
		for _, entry := range step.GetEntries {
			if err := shadow(entry.VarName); err != nil {
				return err
			}
		}
		for _, entry := range step.IntoEntries {
			if err := shadow(entry.ParentKey); err != nil {
				return err
			}
		}
		for _, entry := range step.ForMatrix {
			if err := shadow(entry.VarName); err != nil {
				return err
			}
		}
		if len(step.ForSteps) > 0 {
			if err := checkStepsDontShadow(taskName, step.ForSteps, names); err != nil {
				return err
			}
		}
	}
	return nil
}
