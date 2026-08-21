package eval

import (
	"fmt"
	"strings"

	"hobnob/internal/value"
)

// EvalRunIntoPipe evaluates a run: into: pipe expression against captured
// stdout/stderr/exit code. expr format: "stdout"/"stderr"/"exit", each
// optionally followed by an accessor ([0].name) and/or a filter chain (|
// filter | filter ...) — e.g. "stdout[0].name" or "stdout | trim". stdout and
// stderr are captured once via value.Capture — sniffed into an Array or
// Object only when it decodes cleanly as one, else left a String; exit is
// wrapped as a typed Number via value.Num. Any accessor/filter chain then
// runs typed from there through evalChainOn (the same evaluator EvalValue
// uses), with vars (the caller's scope) available for a dynamic key
// (stdout[.KEY]). Capturing at the source rather than at the end of the
// chain matters: a chain that ends on a String leaf (stdout.cfg, where cfg's
// value is itself a JSON-looking string) must stay a String, not get
// re-sniffed into structure.
func EvalRunIntoPipe(expr, stdout, stderr string, exitCode int, vars map[string]value.Value) (value.Value, error) {
	source, chain, _ := strings.Cut(expr, " | ")
	name, accessor := SplitSourceAccessor(strings.TrimSpace(source))

	var src value.Value
	switch name {
	case "stdout":
		src = value.Capture(stdout)
	case "stderr":
		src = value.Capture(stderr)
	case "exit":
		src = value.Num(exitCode)
	default:
		return value.Value{}, fmt.Errorf("run into: unknown source %q: must be stdout, stderr or exit", name)
	}
	if accessor == "" && chain == "" {
		return src, nil
	}

	result, err := evalChainOn(src, accessor, chain, vars)
	if err != nil {
		return value.Value{}, fmt.Errorf("run into pipe: %w", err)
	}
	return result, nil
}
