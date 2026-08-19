package eval

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"text/template"
)

// templateFuncs is computed once and reused across EvalTemplate calls —
// EvalTemplate is the hottest path in the codebase (called per var/condition/
// dir template), and rebuilding this map on every call showed up as overhead
// under loop-heavy tasks. EvalRunIntoPipe (pipe.go) reuses this same map via
// EvalTemplate rather than maintaining a second filter switch, so every
// filter here works in both {{ }} templates and run: into: pipes.
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
	"trim": func(value string) string {
		return strings.TrimSpace(value)
	},
	"upper": func(value string) string {
		return strings.ToUpper(value)
	},
	"lower": func(value string) string {
		return strings.ToLower(value)
	},
	"split": func(separator, value string) (string, error) {
		var parts []string
		for _, part := range strings.Split(value, separator) {
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
	"lines": func(value string) (string, error) {
		return splitLinesJSON(value)
	},
	"first": func(value string) (string, error) {
		var items []string
		if err := json.Unmarshal([]byte(value), &items); err != nil {
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
		var value string
		var fallback *string
		switch len(rest) {
		case 1:
			value = rest[0]
		case 2:
			fallbackValue := rest[0]
			fallback = &fallbackValue
			value = rest[1]
		default:
			return "", fmt.Errorf("pluck: expected a value and an optional default, got %d extra args", len(rest))
		}
		decoded, err := decodeJSON(value)
		if err != nil {
			if fallback != nil {
				return *fallback, nil
			}
			return "", fmt.Errorf("pluck: %w", err)
		}
		parsedPath, err := jsonPathParser.Parse(normalizeJSONPath(path))
		if err != nil {
			return "", fmt.Errorf("pluck %q: %w", path, err)
		}
		nodes := parsedPath.Select(decoded)
		switch len(nodes) {
		case 0:
			if fallback != nil {
				return *fallback, nil
			}
			return "", fmt.Errorf("pluck %q: no match", path)
		case 1:
			return stringifyJSONValue(nodes[0])
		default:
			jsonBytes, err := json.Marshal(nodes)
			if err != nil {
				return "", err
			}
			return string(jsonBytes), nil
		}
	},
	"keys": func(value string) (string, error) {
		keys, _, err := sortedMapEntries(value)
		if err != nil {
			return "", fmt.Errorf("keys: %w", err)
		}
		jsonBytes, err := json.Marshal(keys)
		if err != nil {
			return "", err
		}
		return string(jsonBytes), nil
	},
	"values": func(value string) (string, error) {
		_, values, err := sortedMapEntries(value)
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

// templateCache holds parsed templates keyed by source string. EvalTemplate
// is called per var/condition/dir template, often repeatedly for the same
// template text under loops — reparsing every call showed up as overhead
// under loop-heavy tasks. A parsed *template.Template is safe to Execute
// concurrently once built, so callers only ever race on the initial parse,
// which sync.Map.LoadOrStore resolves by keeping whichever copy won.
var templateCache sync.Map // string -> *template.Template

func parseTemplateCached(tmpl string) (*template.Template, error) {
	if cached, ok := templateCache.Load(tmpl); ok {
		return cached.(*template.Template), nil
	}
	parsed, err := template.New("").Funcs(templateFuncs).Parse(tmpl)
	if err != nil {
		return nil, err
	}
	actual, _ := templateCache.LoadOrStore(tmpl, parsed)
	return actual.(*template.Template), nil
}

func EvalTemplate(tmpl string, vars map[string]string) (string, error) {
	parsed, err := parseTemplateCached(tmpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := parsed.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
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
