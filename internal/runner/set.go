package runner

import (
	"fmt"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
)

func execSet(step config.Step, scope *cli.Scope) error {
	for _, setEntry := range step.SetEntries {
		var val string
		var err error
		if setEntry.ValNode != nil {
			val, err = evalJSONNodeToJSON(*setEntry.ValNode, func(tmpl string) (string, error) {
				return eval.EvalTemplate(tmpl, scope.Vars)
			})
		} else {
			val, err = eval.EvalTemplate(setEntry.ValTmpl, scope.Vars)
		}
		if err != nil {
			return fmt.Errorf("set value for %q: %w", setEntry.Key, err)
		}
		scope.Set(setEntry.Key, val, setEntry.Secret)
	}
	return nil
}
