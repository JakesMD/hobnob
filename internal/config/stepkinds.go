package config

import (
	"fmt"
	"slices"
	"strings"
)

// stepKind is one step kind's rules: the YAML key that selects it, the keys
// it accepts besides that key and if:, and the caller names it writes.
// Parsing (stepKindOf) and the const: shadow check (Step.Writes) both read
// this table, so a kind's rules live in its row.
type stepKind struct {
	key    string
	keys   []string
	writes func(step Step, tasks map[string]Task) []string
}

var stepKinds = [...]stepKind{
	KindRun: {
		key:  "run",
		keys: []string{"dir", "soft", "quiet", "into"},
		writes: func(step Step, _ map[string]Task) []string {
			return intoNames(step.IntoEntries)
		},
	},
	KindSet: {
		key: "set",
		writes: func(step Step, _ map[string]Task) []string {
			names := make([]string, len(step.SetEntries))
			for i, entry := range step.SetEntries {
				names[i] = entry.Key
			}
			return names
		},
	},
	KindCall: {
		key:  "call",
		keys: []string{"dir", "with", "into", "soft"},
		writes: func(step Step, tasks map[string]Task) []string {
			return intoNames(CallInto(step, tasks[step.CallTarget]))
		},
	},
	KindFor: {
		key:    "loop",
		keys:   []string{"steps"},
		writes: loopIterators,
	},
	KindGet: {
		key: "get",
		writes: func(step Step, _ map[string]Task) []string {
			names := make([]string, len(step.GetEntries))
			for i, entry := range step.GetEntries {
				names[i] = entry.VarName
			}
			return names
		},
	},
}

// Writes returns every caller name step may write, a loop: body's included.
// tasks is the step's own file's tasks: a call: to one of them counts its
// results: (see CallInto); any other target counts only its into:.
func (step Step) Writes(tasks map[string]Task) []string {
	names := stepKinds[step.Kind].writes(step, tasks)
	for _, sub := range step.ForSteps {
		names = append(names, sub.Writes(tasks)...)
	}
	return names
}

// loopIterators are the names a loop: binds. A literal list binds ITEM and a
// matrix its own names. A templated source may resolve to an object, which
// binds KEY and VALUE instead, and that isn't known until it runs, so it
// counts as writing all three.
func loopIterators(step Step, _ map[string]Task) []string {
	switch {
	case len(step.ForMatrix) > 0:
		names := make([]string, len(step.ForMatrix))
		for i, entry := range step.ForMatrix {
			names[i] = entry.VarName
		}
		return names
	case strings.Contains(step.ForTarget.String(), "{{"):
		return []string{"ITEM", "KEY", "VALUE"}
	default:
		return []string{"ITEM"}
	}
}

func intoNames(entries []IntoEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.ParentKey
	}
	return names
}

// stepKindOf picks the kind of a step from its keys: exactly one kind key,
// and nothing that kind doesn't take.
func stepKindOf(entries []mapEntry) (StepKind, error) {
	var found []string
	var kind StepKind
	for _, entry := range entries {
		switch entry.Key {
		case "use":
			return 0, fmt.Errorf("use: was removed — use call: with into:, and once: true on the target task")
		case "rerun":
			return 0, fmt.Errorf("rerun: was removed along with use: — omit once: on the target task instead")
		}
		for k, spec := range stepKinds {
			if spec.key == entry.Key {
				found = append(found, entry.Key+":")
				kind = StepKind(k)
			}
		}
	}
	switch len(found) {
	case 0:
		return 0, fmt.Errorf("step needs one of %s", kindKeyList())
	case 1:
	default:
		return 0, fmt.Errorf("step has %s; a step takes exactly one of %s", strings.Join(found, " and "), kindKeyList())
	}
	spec := stepKinds[kind]
	for _, entry := range entries {
		if entry.Key == spec.key || entry.Key == "if" || slices.Contains(spec.keys, entry.Key) {
			continue
		}
		msg := fmt.Sprintf("%s: step doesn't take %s:", spec.key, entry.Key)
		if owners := kindsTaking(entry.Key); owners != "" {
			msg += fmt.Sprintf(" (only %s steps do)", owners)
		}
		return 0, fmt.Errorf("%s; it takes if:%s", msg, keyList(spec.keys))
	}
	return kind, nil
}

// kindsTaking lists the kinds that accept key, as "run:/call:".
func kindsTaking(key string) string {
	var owners []string
	for _, spec := range stepKinds {
		if slices.Contains(spec.keys, key) {
			owners = append(owners, spec.key+":")
		}
	}
	return strings.Join(owners, "/")
}

func keyList(keys []string) string {
	var out string
	for _, key := range keys {
		out += ", " + key + ":"
	}
	return out
}

func kindKeyList() string {
	keys := make([]string, len(stepKinds))
	for i, spec := range stepKinds {
		keys[i] = spec.key + ":"
	}
	return strings.Join(keys, ", ")
}
