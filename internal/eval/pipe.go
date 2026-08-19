package eval

import (
	"fmt"
	"strings"
)

// EvalRunIntoPipe evaluates a run: into: pipe expression against captured
// stdout/stderr. expr format: "stdout" | "stderr" [| filter | filter ...].
// The filter chain (if any) is spliced onto ".SRC" and run through
// EvalTemplate against templateFuncs — the same registry {{ }} templates use
// — so every template filter (trim, upper, lines, pluck, split, ...) works
// here too without a second, separately-maintained implementation.
func EvalRunIntoPipe(expr, stdout, stderr string) (string, error) {
	source, chain, _ := strings.Cut(expr, " | ")
	source = strings.TrimSpace(source)

	var val string
	switch source {
	case "stdout":
		val = stdout
	case "stderr":
		val = stderr
	default:
		return "", fmt.Errorf("run into: unknown source %q: must be stdout or stderr", source)
	}
	if chain == "" {
		return val, nil
	}

	rendered, err := EvalTemplate("{{ .SRC | "+chain+" }}", map[string]string{"SRC": val})
	if err != nil {
		return "", fmt.Errorf("run into pipe: %w", err)
	}
	return rendered, nil
}
