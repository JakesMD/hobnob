package value

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/theory/jsonpath"
)

// Filter is a scope template filter: args holds every argument in the order
// written, with the piped value always last (Go template pipe convention —
// {{ .DATA | pluck "path" }} calls pluck with args = [Str("path"), DATA]).
type Filter func(args []Value) (Value, error)

// Filters is the single registry every template filter is defined in.
// internal/eval adapts these into text/template's FuncMap; EvalValue calls
// them directly for the type-preserving evaluation path. Reachable from
// run: into: pipe expressions too, via EvalRunIntoPipe — one registry, two
// callers.
var Filters = map[string]Filter{
	"default":    filterDefault,
	"trim":       filterTrim,
	"upper":      filterUpper,
	"lower":      filterLower,
	"split":      filterSplit,
	"lines":      filterLines,
	"first":      filterFirst,
	"pluck":      filterPluck,
	"keys":       filterKeys,
	"values":     filterValues,
	"jsonEscape": filterJSONEscape,
	"json":       filterJSON,
	"string":     filterString,
	"len":        filterLen,
}

func piped(args []Value) Value {
	return args[len(args)-1]
}

func requireString(v Value, name string) (string, error) {
	if v.Kind() != KindString {
		return "", fmt.Errorf("%s: expected a string, got %s; pipe through | string to convert", name, v.Kind())
	}
	return v.String(), nil
}

// filterDefault returns the piped value unchanged when it's non-empty,
// preserving its type — falling back to a String/Array/etc only stringifies
// the piped side, never the default.
func filterDefault(args []Value) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("default: expected a fallback and a value")
	}
	def := args[0]
	value := piped(args)
	if !value.IsEmpty() {
		return value, nil
	}
	return def, nil
}

// filterTrim is identity on non-String kinds — into: RESP: stdout | trim
// must not stringify a captured object.
func filterTrim(args []Value) (Value, error) {
	value := piped(args)
	if value.Kind() != KindString {
		return value, nil
	}
	return Str(strings.TrimSpace(value.String())), nil
}

func filterUpper(args []Value) (Value, error) {
	s, err := requireString(piped(args), "upper")
	if err != nil {
		return Value{}, err
	}
	return Str(strings.ToUpper(s)), nil
}

func filterLower(args []Value) (Value, error) {
	s, err := requireString(piped(args), "lower")
	if err != nil {
		return Value{}, err
	}
	return Str(strings.ToLower(s)), nil
}

func filterSplit(args []Value) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("split: expected a separator and a value")
	}
	sep := args[0].String()
	s, err := requireString(piped(args), "split")
	if err != nil {
		return Value{}, err
	}
	var parts []any
	for _, part := range strings.Split(s, sep) {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return Of(parts), nil
}

func filterLines(args []Value) (Value, error) {
	s, err := requireString(piped(args), "lines")
	if err != nil {
		return Value{}, err
	}
	var lines []any
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return Of(lines), nil
}

// filterFirst requires an Array — a String never auto-parses (bug: pluck on
// a JSON-looking string silently re-sniffing). Empty array yields Nil.
func filterFirst(args []Value) (Value, error) {
	value := piped(args)
	if value.Kind() != KindArray {
		return Value{}, fmt.Errorf("first: expected an array, got %s; pipe through | json to parse it first", value.Kind())
	}
	arr := value.Any().([]any)
	if len(arr) == 0 {
		return Nil(), nil
	}
	return Of(arr[0]), nil
}

// jsonPathParser is reused across calls — safe for concurrent use, parsing
// is the expensive part.
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

// filterPluck requires the piped value to already be an Array/Object — it
// never auto-parses a String, even one holding valid JSON text. That
// discipline is what makes a JSON-string leaf ({"cfg":"[1,2]"}) stay a
// string end to end instead of silently becoming an array on the next pluck.
func filterPluck(args []Value) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("pluck: expected a path and a value")
	}
	value := piped(args)
	path := args[0].String()
	fallback := args[1 : len(args)-1] // present iff len == 1 (path, fallback, value)

	if value.Kind() != KindArray && value.Kind() != KindObject {
		if len(fallback) == 1 {
			return fallback[0], nil
		}
		return Value{}, fmt.Errorf("pluck %q: value is a %s; pipe through | json to parse it first", path, value.Kind())
	}

	parsedPath, err := jsonPathParser.Parse(normalizeJSONPath(path))
	if err != nil {
		return Value{}, fmt.Errorf("pluck %q: %w", path, err)
	}
	nodes := parsedPath.Select(value.Any())
	switch len(nodes) {
	case 0:
		if len(fallback) == 1 {
			return fallback[0], nil
		}
		return Value{}, fmt.Errorf("pluck %q: no match", path)
	case 1:
		return Of(nodes[0]), nil
	default:
		return Of([]any(nodes)), nil
	}
}

func sortedKeys(obj map[string]any) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func filterKeys(args []Value) (Value, error) {
	value := piped(args)
	if value.Kind() != KindObject {
		return Value{}, fmt.Errorf("keys: expected an object, got %s; pipe through | json to parse it first", value.Kind())
	}
	keys := sortedKeys(value.Any().(map[string]any))
	arr := make([]any, len(keys))
	for i, k := range keys {
		arr[i] = k
	}
	return Of(arr), nil
}

// filterValues returns real typed children — not their stringified form —
// so a nested object stays an object and chains into another pluck.
func filterValues(args []Value) (Value, error) {
	value := piped(args)
	if value.Kind() != KindObject {
		return Value{}, fmt.Errorf("values: expected an object, got %s; pipe through | json to parse it first", value.Kind())
	}
	obj := value.Any().(map[string]any)
	keys := sortedKeys(obj)
	arr := make([]any, len(keys))
	for i, k := range keys {
		arr[i] = obj[k]
	}
	return Of(arr), nil
}

func filterJSONEscape(args []Value) (Value, error) {
	jsonBytes, err := json.Marshal(piped(args).String())
	if err != nil {
		return Value{}, fmt.Errorf("jsonEscape: %w", err)
	}
	return Str(string(jsonBytes[1 : len(jsonBytes)-1])), nil
}

// filterJSON is the explicit escape hatch into structure: strictly parses a
// String, and is identity on every other kind (including one already
// structured). An optional fallback before the piped value — json
// "fallback" — swallows a parse failure and returns the fallback instead of
// erroring, the same convention pluck uses for a missing key.
func filterJSON(args []Value) (Value, error) {
	if len(args) == 0 {
		return Value{}, fmt.Errorf("json: expected a value")
	}
	value := piped(args)
	fallback := args[:len(args)-1] // present iff len(args) == 2

	if value.Kind() != KindString {
		return value, nil
	}
	parsed, err := Parse(value.String())
	if err != nil {
		if len(fallback) == 1 {
			return fallback[0], nil
		}
		return Value{}, fmt.Errorf("json: %w", err)
	}
	return parsed, nil
}

func filterString(args []Value) (Value, error) {
	return Str(piped(args).String()), nil
}

func filterLen(args []Value) (Value, error) {
	value := piped(args)
	switch value.Kind() {
	case KindArray:
		return Num(len(value.Any().([]any))), nil
	case KindObject:
		return Num(len(value.Any().(map[string]any))), nil
	case KindString:
		return Num(len([]rune(value.String()))), nil
	case KindNil:
		return Num(0), nil
	default:
		return Value{}, fmt.Errorf("len: expected a string, array, or object, got %s", value.Kind())
	}
}
