package eval

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
)

func splitLinesJSON(s string) (string, error) {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	jsonBytes, err := json.Marshal(lines)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func EvalTemplate(tmpl string, vars map[string]string) (string, error) {
	funcs := template.FuncMap{
		"default": func(def string, given ...interface{}) string {
			if len(given) == 0 || given[0] == nil {
				return def
			}
			if s, ok := given[0].(string); ok && s != "" {
				return s
			}
			return def
		},
		"trim": func(s string) string {
			return strings.TrimSpace(s)
		},
		"upper": func(s string) string {
			return strings.ToUpper(s)
		},
		"lower": func(s string) string {
			return strings.ToLower(s)
		},
		"split": func(sep, s string) (string, error) {
			var parts []string
			for _, part := range strings.Split(s, sep) {
				if part != "" {
					parts = append(parts, part)
				}
			}
			jsonBytes, err := json.Marshal(parts)
			if err != nil {
				return "", err
			}
			return string(jsonBytes), nil
		},
		"lines": func(s string) (string, error) {
			return splitLinesJSON(s)
		},
		"first": func(s string) (string, error) {
			var items []string
			if err := json.Unmarshal([]byte(s), &items); err != nil {
				return "", fmt.Errorf("first: %w", err)
			}
			if len(items) == 0 {
				return "", nil
			}
			return items[0], nil
		},
	}
	parsed, err := template.New("").Funcs(funcs).Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := parsed.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// EvalCondition renders condTmpl then passes the result directly to sh -c.
// Returns true if the shell command exits 0.
func EvalCondition(condTmpl string, vars map[string]string) (bool, error) {
	rendered, err := EvalTemplate(condTmpl, vars)
	if err != nil {
		return false, err
	}
	cmd := exec.Command("sh", "-c", rendered)
	err = cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ParseList parses a JSON array string or a single value into a []string.
// Returns nil for empty input.
func ParseList(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if strings.HasPrefix(s, "[") {
		var items []string
		if err := json.Unmarshal([]byte(s), &items); err != nil {
			return nil, fmt.Errorf("invalid list %q: %w", s, err)
		}
		return items, nil
	}
	return []string{s}, nil
}

func CopyVars(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// EvalRunIntoPipe evaluates a run: into: pipe expression against captured stdout/stderr.
// expr format: "stdout" | "stderr" [| "lines" | "trim" | "upper" | "lower" ...]
func EvalRunIntoPipe(expr, stdout, stderr string) (string, error) {
	tokens := strings.Split(expr, " | ")
	if len(tokens) == 0 {
		return "", fmt.Errorf("run into: empty expression")
	}
	var val string
	switch strings.TrimSpace(tokens[0]) {
	case "stdout":
		val = stdout
	case "stderr":
		val = stderr
	default:
		return "", fmt.Errorf("run into: unknown source %q: must be stdout or stderr", strings.TrimSpace(tokens[0]))
	}
	for _, fn := range tokens[1:] {
		fn = strings.TrimSpace(fn)
		switch fn {
		case "lines":
			result, err := splitLinesJSON(val)
			if err != nil {
				return "", fmt.Errorf("run into pipe %q: %w", fn, err)
			}
			val = result
		case "trim":
			val = strings.TrimSpace(val)
		case "upper":
			val = strings.ToUpper(val)
		case "lower":
			val = strings.ToLower(val)
		default:
			return "", fmt.Errorf("run into: unknown pipe function %q", fn)
		}
	}
	return val, nil
}

// ResolveFromItems renders a literal list or a template that resolves to a JSON
// array into a []string. Used by execGet and execFor.
func ResolveFromItems(fromList []string, fromTmpl string, vars map[string]string, ctxName string) ([]string, error) {
	if len(fromList) > 0 {
		items := make([]string, 0, len(fromList))
		for _, item := range fromList {
			rendered, err := EvalTemplate(item, vars)
			if err != nil {
				return nil, fmt.Errorf("%s from item: %w", ctxName, err)
			}
			items = append(items, rendered)
		}
		return items, nil
	}
	if fromTmpl != "" {
		rendered, err := EvalTemplate(fromTmpl, vars)
		if err != nil {
			return nil, fmt.Errorf("%s from template: %w", ctxName, err)
		}
		items, err := ParseList(rendered)
		if err != nil {
			return nil, fmt.Errorf("%s from list: %w", ctxName, err)
		}
		return items, nil
	}
	return nil, nil
}
