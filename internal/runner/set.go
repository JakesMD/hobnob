package runner

import (
	"fmt"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/value"
)

func execSet(step config.Step, scope *cli.Scope) error {
	for _, setEntry := range step.SetEntries {
		val, err := config.EvalSetEntry(setEntry, func(tmpl string) (value.Value, error) {
			return eval.EvalValue(tmpl, scope.Vars)
		})
		if err != nil {
			return fmt.Errorf("set value for %q: %w", setEntry.Key, err)
		}
		scope.Set(setEntry.Key, val, setEntry.Secret)
	}
	return nil
}
