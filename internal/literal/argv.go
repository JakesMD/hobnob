package literal

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"hobnob/internal/eval"
	"hobnob/internal/value"
)

// Argv is run:'s list form: one template per element, executed without a
// shell.
type Argv struct {
	elems []string
}

// ParseArgv parses run:'s list form. Each element takes a reference like any
// templated field; a trailing ... on one is accepted and changes nothing,
// since argv already splices lists.
func ParseArgv(n *yaml.Node) (Argv, error) {
	if len(n.Content) == 0 {
		return Argv{}, fmt.Errorf("run: list form needs at least one element")
	}
	elems := make([]string, len(n.Content))
	for i, item := range n.Content {
		elems[i], _ = templateLeaf(item.Value)
	}
	return Argv{elems: elems}, nil
}

// IsZero reports whether a is the zero Argv: the step has no list form.
func (a Argv) IsZero() bool {
	return len(a.elems) == 0
}

// Resolve evaluates each element, typed, and assembles the argv. A list
// splices into one argument per item (an empty list into none); anything
// else is exactly one argument. An empty element stays one empty argument,
// so later positions never shift.
func (a Argv) Resolve(vars map[string]value.Value) ([]string, error) {
	var argv []string
	for i, tmpl := range a.elems {
		resolved, err := eval.EvalValue(tmpl, vars)
		if err != nil {
			return nil, fmt.Errorf("run argv[%d]: %w", i, err)
		}
		args, err := argvElements(resolved, i)
		if err != nil {
			return nil, err
		}
		argv = append(argv, args...)
	}
	if len(argv) == 0 || argv[0] == "" {
		return nil, fmt.Errorf("run: no command to execute")
	}
	return argv, nil
}

func argvElements(v value.Value, index int) ([]string, error) {
	switch v.Kind() {
	case value.KindObject:
		return nil, errArgvObject(index)
	case value.KindArray:
		arr := v.Any().([]any)
		args := make([]string, 0, len(arr))
		for _, elem := range arr {
			ev := value.Of(elem)
			if ev.Kind() == value.KindObject || ev.Kind() == value.KindArray {
				return nil, errArgvObject(index)
			}
			args = append(args, ev.String())
		}
		return args, nil
	default:
		return []string{v.String()}, nil
	}
}

func errArgvObject(index int) error {
	return fmt.Errorf("run: argument %d resolved to an object; use an accessor (.VAR.field) to select the field you mean to pass", index)
}
