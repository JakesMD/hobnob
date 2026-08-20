package eval

import (
	"fmt"

	"hobnob/internal/value"
)

// ResolveArgv evaluates each run: list-form element template and assembles
// the resulting argv, typed. An Array element splices into one argument per
// item (empty Array -> zero arguments); anything else becomes exactly one
// argument. Unlike ItemsFromValue (loop:/options:), an empty/Nil element is
// preserved as one empty argument rather than dropped — dropping it would
// silently shift every later positional argument.
func ResolveArgv(tmpls []string, vars map[string]value.Value) ([]string, error) {
	var argv []string
	for i, tmpl := range tmpls {
		resolved, err := EvalValue(tmpl, vars)
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
		return nil, fmt.Errorf("run: argument %d resolved to an object; use an accessor (.VAR.field) to select the field you mean to pass", index)
	case value.KindArray:
		arr := v.Any().([]any)
		args := make([]string, 0, len(arr))
		for _, elem := range arr {
			ev := value.Of(elem)
			if ev.Kind() == value.KindObject || ev.Kind() == value.KindArray {
				return nil, fmt.Errorf("run: argument %d resolved to an object; use an accessor (.VAR.field) to select the field you mean to pass", index)
			}
			args = append(args, ev.String())
		}
		return args, nil
	default:
		return []string{v.String()}, nil
	}
}
