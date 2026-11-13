package value

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Filter is a scope template filter: args holds every argument in the order
// written, with the piped value always last (Go template pipe convention —
// {{ .DATA | trim }} calls trim with args = [DATA]).
type Filter func(args []Value) (Value, error)

// Filters is the single registry every template filter is defined in.
// internal/eval adapts these into text/template's FuncMap; EvalValue calls
// them directly for the type-preserving evaluation path. Reachable from
// into: leaves too, via EvalIntoLeaf — one registry, two callers.
var Filters = map[string]Filter{
	"default":    filterDefault,
	"trim":       filterTrim,
	"upper":      filterUpper,
	"lower":      filterLower,
	"split":      filterSplit,
	"lines":      filterLines,
	"join":       filterJoin,
	"keys":       filterKeys,
	"jsonEscape": filterJSONEscape,
	"json":       filterJSON,
	"string":     filterString,
	"len":        filterLen,
	"quote":      filterQuote,
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

// requireKind guards a filter that needs structure. Only a String gets the
// | json hint: it is the one kind that might be unparsed structure, and
// suggesting a parse for nil or a number would mislead.
func requireKind(v Value, want Kind, name string) error {
	got := v.Kind()
	if got == want {
		return nil
	}
	article := "a"
	if strings.ContainsRune("aeiou", rune(want.String()[0])) {
		article = "an"
	}
	msg := fmt.Sprintf("%s: expected %s %s, got %s", name, article, want, got)
	if got == KindString {
		msg += "; pipe through | json to parse it first"
	}
	return errors.New(msg)
}

// filterDefault returns the piped value unchanged when it's non-empty,
// preserving its type — falling back to a String/Array/etc only stringifies
// the piped side, never the default. Also falls back when the piped value
// is a deferred "path not found" sentinel (see Missing) — the one and only
// way an accessor's missing-path error is caught rather than raised. Both
// triggers produce the same result, so there's nothing new to reason about.
func filterDefault(args []Value) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("default: expected a fallback and a value")
	}
	def := args[0]
	value := piped(args)
	if value.IsMissing() || value.IsEmpty() {
		return def, nil
	}
	return value, nil
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

// filterJoin is the inverse of split. Every element must be text, a number or
// a bool: a null, list or object element errors rather than joining as "" or
// compact JSON, and the error names the accessor that picks out the text.
func filterJoin(args []Value) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("join: expected a separator and a value")
	}
	sep := args[0].String()
	value := piped(args)
	if err := requireKind(value, KindArray, "join"); err != nil {
		return Value{}, err
	}
	elems := value.Any().([]any)
	parts := make([]string, len(elems))
	for i, elem := range elems {
		v := Of(elem)
		switch v.Kind() {
		case KindNil:
			return Value{}, fmt.Errorf("join: element [%d] is null, not text", i)
		case KindObject:
			return Value{}, fmt.Errorf("join: element [%d] is an object, not text; access the field you meant, e.g. .LIST[*].name", i)
		case KindArray:
			return Value{}, fmt.Errorf("join: element [%d] is an array, not text; access the element you meant, e.g. .LIST[*][0]", i)
		}
		parts[i] = v.String()
	}
	return Str(strings.Join(parts, sep)), nil
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
	if err := requireKind(value, KindObject, "keys"); err != nil {
		return Value{}, err
	}
	keys := sortedKeys(value.Any().(map[string]any))
	arr := make([]any, len(keys))
	for i, k := range keys {
		arr[i] = k
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
// structured).
func filterJSON(args []Value) (Value, error) {
	if len(args) == 0 {
		return Value{}, fmt.Errorf("json: expected a value")
	}
	value := piped(args)
	if value.Kind() != KindString {
		return value, nil
	}
	parsed, err := Parse(value.String())
	if err != nil {
		return Value{}, fmt.Errorf("json: %w", err)
	}
	return parsed, nil
}

func filterString(args []Value) (Value, error) {
	return Str(piped(args).String()), nil
}

// ShellQuote wraps s in POSIX single quotes for safe interpolation into a
// `sh -c` script, escaping any embedded single quote.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func filterQuote(args []Value) (Value, error) {
	return Str(ShellQuote(piped(args).String())), nil
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
