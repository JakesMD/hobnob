package runner

import (
	"fmt"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
)

func execSet(step config.Step, scope *cli.Scope) error {
	for _, setEntry := range step.SetEntries {
		val, err := eval.EvalTemplate(setEntry.ValTmpl, scope.Vars)
		if err != nil {
			return fmt.Errorf("set value for %q: %w", setEntry.Key, err)
		}
		scope.Set(setEntry.Key, val, setEntry.Secret)
	}
	return nil
}
