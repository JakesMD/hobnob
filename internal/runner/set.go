package runner

import (
	"fmt"

	"hobnob/internal/config"
	"hobnob/internal/scope"
)

func execSet(step config.Step, scope *scope.Scope) error {
	for _, setEntry := range step.SetEntries {
		val, err := setEntry.Val.Value(scope.Vars())
		if err != nil {
			return fmt.Errorf("%s=%s: %w", setEntry.Key, setEntry.Val.String(), err)
		}
		scope.Set(setEntry.Key, val, setEntry.Secret)
	}
	return nil
}
