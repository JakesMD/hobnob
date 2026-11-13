package runner

import (
	"fmt"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/scope"
	"hobnob/internal/value"
)

// captureInto evaluates a run: or call: step's into: entries, top to bottom,
// and writes each result into scope. source resolves a leaf's head name (see
// eval.EvalIntoLeaf); a map/list literal entry evaluates each leaf the same
// way and assembles them typed. kind names the step in errors.
//
// child is the call: target's final scope, nil for run:. Every result is
// adopted from it, so a child secret inside the value stays masked in scope,
// and the target name is flagged secret whenever the value carries one,
// whatever shape the leaf had.
func captureInto(kind string, entries []config.IntoEntry, source eval.IntoSource, scope, child *scope.Scope) error {
	for _, intoEntry := range entries {
		val, err := intoEntry.Val.Eval(source, scope.Vars())
		if err != nil {
			return fmt.Errorf("%s into %q: %w", kind, intoEntry.ParentKey, err)
		}
		secret := child != nil && scope.Adopt(child, val)
		scope.Set(intoEntry.ParentKey, val, secret)
	}
	return nil
}

// runSource is a run: step's into: source: stdout, stderr or exit. Each is
// built only when a leaf names it, so value.Capture sniffs only the stream
// actually used.
func runSource(stdout, stderr string, exitCode int) eval.IntoSource {
	return func(name string) (value.Value, error) {
		switch name {
		case "stdout":
			return value.Capture(stdout), nil
		case "stderr":
			return value.Capture(stderr), nil
		case "exit":
			return value.Num(exitCode), nil
		}
		return value.Value{}, fmt.Errorf("unknown source %q: must be stdout, stderr or exit", name)
	}
}
