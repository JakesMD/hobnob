package eval

import (
	"fmt"
	"strings"

	"hobnob/internal/value"
)

// EvalRunIntoPipe evaluates a run: into: pipe expression against captured
// stdout/stderr. expr format: "stdout" | "stderr" [| filter | filter ...].
// The source is captured once via value.Capture — sniffed into an Array or
// Object only when it decodes cleanly as one, else left a String — and any
// filter chain then runs typed from there through evalChainOn (the same
// evaluator EvalValue uses). Capturing at the source rather than at the end
// of the chain matters: a filter chain that ends on a String leaf (stdout |
// pluck "cfg", where cfg's value is itself a JSON-looking string) must stay
// a String, not get re-sniffed into structure.
func EvalRunIntoPipe(expr, stdout, stderr string) (value.Value, error) {
	source, chain, _ := strings.Cut(expr, " | ")
	source = strings.TrimSpace(source)

	var src value.Value
	switch source {
	case "stdout":
		src = value.Capture(stdout)
	case "stderr":
		src = value.Capture(stderr)
	default:
		return value.Value{}, fmt.Errorf("run into: unknown source %q: must be stdout or stderr", source)
	}
	if chain == "" {
		return src, nil
	}

	result, err := evalChainOn(src, chain)
	if err != nil {
		return value.Value{}, fmt.Errorf("run into pipe: %w", err)
	}
	return result, nil
}
