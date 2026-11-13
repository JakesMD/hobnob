package config

import (
	"fmt"

	"hobnob/internal/eval"
	"hobnob/internal/value"
)

// The call contract: what crosses from a called task back into its caller.
// Load-time checks and runner.finishCall both ask this file, so the rule
// (ADR-0002 results, ADR-0003 private variables) lives in one place.

// CallInto is the projection of a call: step onto its caller: the into:
// entries it applies, whose ParentKeys are the caller names it writes. A
// written into:, even into: [], is taken as is; with none, the target's
// results: come back as bare entries. A target unknown at load time (a
// module or templated one) passes as the zero Task, so only into: counts.
func CallInto(step Step, target Task) []IntoEntry {
	if step.HasInto {
		return step.IntoEntries
	}
	entries := make([]IntoEntry, 0, len(target.Results))
	for _, name := range target.Results {
		entries = append(entries, BareIntoEntry(name))
	}
	return entries
}

// CallSource checks target's results: promise against the child's final
// vars, then returns the into: source the projection reads from. A name
// the child never set is a deferred "not found", catchable by | default; a
// private (_-prefixed) name is a hard error that | default does not catch.
func CallSource(target Task, childVars map[string]value.Value) (eval.IntoSource, error) {
	for _, name := range target.Results {
		if _, ok := childVars[name]; !ok {
			return nil, fmt.Errorf("result %q was not set (listed in its results:)", name)
		}
	}
	return func(name string) (value.Value, error) {
		if IsPrivateVar(name) {
			return value.Value{}, fmt.Errorf("%q is private to the called task (a _ prefix keeps a variable out of into:)", name)
		}
		if val, ok := childVars[name]; ok {
			return val, nil
		}
		return value.Missing(fmt.Sprintf("%q not found", name)), nil
	}, nil
}
