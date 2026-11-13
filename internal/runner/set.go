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
			return fmt.Errorf("set value for %q: %w", setEntry.Key, err)
		}
		scope.Set(setEntry.Key, val, setEntry.Secret)
	}
	return nil
}
