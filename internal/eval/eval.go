package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/theory/jsonpath"
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

// decodeJSON parses a JSON string into an interface{} tree, preserving
// numbers as json.Number so pluck/keys/values round-trip them exactly
// (no float64 precision loss, no unwanted ".0" suffix).
func decodeJSON(s string) (interface{}, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// stringifyJSONValue renders a decoded JSON leaf as a hobnob scope string:
// strings pass through unquoted, numbers/bools use their plain text form,
// and nested objects/arrays re-marshal to compact JSON so results stay
// chainable (pluck | pluck, pluck | loop, ...).
func stringifyJSONValue(v interface{}) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", nil
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case json.Number:
		return t.String(), nil
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

// jsonPathParser parses pluck's path argument as an RFC 9535 JSONPath query
// (https://www.rfc-editor.org/rfc/rfc9535.html). Reused across calls — safe
// for concurrent use, parsing is the expensive part.
var jsonPathParser = jsonpath.NewParser()

// normalizeJSONPath lets pluck callers write "profile.name" or "[2]" instead
// of the RFC-required "$.profile.name" / "$[2]" — pluck always addresses
// from the root, so the "$" is implied rather than typed out each time.
func normalizeJSONPath(path string) string {
	switch {
	case strings.HasPrefix(path, "$"):
		return path
	case strings.HasPrefix(path, "["):
		return "$" + path
	default:
		return "$." + path
	}
}

// sortedMapEntries decodes s as a JSON object and returns its keys (sorted)
// alongside their stringified values, so iteration order is deterministic.
func sortedMapEntries(s string) (keys []string, values []string, err error) {
	v, err := decodeJSON(s)
	if err != nil {
		return nil, nil, err
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("not a JSON object")
	}
	keys = make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	values = make([]string, 0, len(keys))
	for _, k := range keys {
		sv, err := stringifyJSONValue(m[k])
		if err != nil {
			return nil, nil, err
		}
		values = append(values, sv)
	}
	return keys, values, nil
}

// IsJSONObject reports whether s (trimmed) looks like a JSON object, used
// by execFor to pick list vs. map loop iteration.
func IsJSONObject(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "{")
}

// ParseMapEntries decodes a JSON object string into sorted keys and their
// stringified values, for loop: map iteration (.KEY / .VALUE per pass).
func ParseMapEntries(s string) (keys []string, values []string, err error) {
	keys, values, err = sortedMapEntries(s)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid map %q: %w", s, err)
	}
	return keys, values, nil
}

// templateFuncs is computed once and reused across EvalTemplate calls —
// EvalTemplate is the hottest path in the codebase (called per var/condition/
// dir template), and rebuilding this map on every call showed up as overhead
// under loop-heavy tasks.
var templateFuncs = template.FuncMap{
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
	// pluck takes the piped JSON value as its last arg (Go template
	// pipeline convention). An optional default before it — pluck "path"
	// "fallback" — swallows a missing key/index or invalid JSON and
	// returns the fallback instead of erroring; omit it to fail fast.
	"pluck": func(path string, rest ...string) (string, error) {
		var s string
		var def *string
		switch len(rest) {
		case 1:
			s = rest[0]
		case 2:
			d := rest[0]
			def = &d
			s = rest[1]
		default:
			return "", fmt.Errorf("pluck: expected a value and an optional default, got %d extra args", len(rest))
		}
		v, err := decodeJSON(s)
		if err != nil {
			if def != nil {
				return *def, nil
			}
			return "", fmt.Errorf("pluck: %w", err)
		}
		p, err := jsonPathParser.Parse(normalizeJSONPath(path))
		if err != nil {
			return "", fmt.Errorf("pluck %q: %w", path, err)
		}
		nodes := p.Select(v)
		switch len(nodes) {
		case 0:
			if def != nil {
				return *def, nil
			}
			return "", fmt.Errorf("pluck %q: no match", path)
		case 1:
			return stringifyJSONValue(nodes[0])
		default:
			b, err := json.Marshal(nodes)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
	},
	"keys": func(s string) (string, error) {
		keys, _, err := sortedMapEntries(s)
		if err != nil {
			return "", fmt.Errorf("keys: %w", err)
		}
		jsonBytes, err := json.Marshal(keys)
		if err != nil {
			return "", err
		}
		return string(jsonBytes), nil
	},
	"values": func(s string) (string, error) {
		_, values, err := sortedMapEntries(s)
		if err != nil {
			return "", fmt.Errorf("values: %w", err)
		}
		jsonBytes, err := json.Marshal(values)
		if err != nil {
			return "", err
		}
		return string(jsonBytes), nil
	},
}

func EvalTemplate(tmpl string, vars map[string]string) (string, error) {
	parsed, err := template.New("").Funcs(templateFuncs).Parse(tmpl)
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
// dir sets the working directory the condition runs in; empty uses the
// current process's working directory. Returns true if the shell command
// exits 0.
func EvalCondition(condTmpl string, vars map[string]string, dir string) (bool, error) {
	rendered, err := EvalTemplate(condTmpl, vars)
	if err != nil {
		return false, err
	}
	cmd := exec.Command("sh", "-c", rendered)
	cmd.Dir = dir
	err = cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SourceShellFile sources path in a subshell (dir sets its working directory)
// and returns the vars it set or changed, compared against a baseline `env`
// snapshot taken the same way. This filters out ambient shell noise (e.g.
// SHLVL) that a plain post-source `env` dump would otherwise include.
func SourceShellFile(path, dir string) (map[string]string, error) {
	baseline, err := captureEnv(dir, "env -0")
	if err != nil {
		return nil, fmt.Errorf("capture baseline env: %w", err)
	}
	sourced, err := captureEnv(dir, fmt.Sprintf("set -a; . %s; env -0", shellQuote(path)))
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	diff := make(map[string]string)
	for k, v := range sourced {
		if baseline[k] != v {
			diff[k] = v
		}
	}
	return diff, nil
}

// shellQuote wraps s in single quotes for safe interpolation into a POSIX
// `sh -c` script, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func captureEnv(dir, script string) (map[string]string, error) {
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil, err
	}
	vars := make(map[string]string)
	for _, entry := range strings.Split(stdout.String(), "\x00") {
		idx := strings.IndexByte(entry, '=')
		if idx <= 0 {
			continue
		}
		vars[entry[:idx]] = entry[idx+1:]
	}
	return vars, nil
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
