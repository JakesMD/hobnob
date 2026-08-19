package value

import (
	"encoding/json"
	"testing"
)

func call(t *testing.T, name string, args ...Value) (Value, error) {
	t.Helper()
	filter, ok := Filters[name]
	if !ok {
		t.Fatalf("no such filter: %q", name)
	}
	return filter(args)
}

func TestFilterDefault(t *testing.T) {
	tests := []struct {
		name       string
		def, piped Value
		want       string
	}{
		{"given empty piped value, when default called, then returns fallback", Str("fallback"), Str(""), "fallback"},
		{"given nil piped value, when default called, then returns fallback (why: global vars use | default for fallback)", Str("fallback"), Nil(), "fallback"},
		{"given non-empty piped value, when default called, then returns piped value unchanged", Str("fallback"), Str("set"), "set"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := call(t, "default", test.def, test.piped)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != test.want {
				t.Errorf("got %q, want %q", got.String(), test.want)
			}
		})
	}
}

func TestFilterDefault_PreservesTypeOfPipedValue(t *testing.T) {
	// given a non-empty Array piped in, when default called, then the Array
	// itself is returned, not its stringified form (why: default must not
	// silently flatten structure)
	// Arrange
	arr := Of([]any{"a", "b"})

	// Act
	got, err := call(t, "default", Str("fallback"), arr)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Kind() != KindArray {
		t.Errorf("kind: got %v, want Array", got.Kind())
	}
}

func TestFilterTrim(t *testing.T) {
	// given a string with padding, when trim called, then whitespace removed
	got, err := call(t, "trim", Str("  hello  "))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "hello" {
		t.Errorf("got %q, want %q", got.String(), "hello")
	}
}

func TestFilterTrim_IdentityOnNonString(t *testing.T) {
	// given a captured object, when trim called, then the object passes
	// through unchanged (why: into: RESP: stdout | trim must not stringify
	// a captured API response before the next pluck)
	// Arrange
	obj := Of(map[string]any{"a": "b"})

	// Act
	got, err := call(t, "trim", obj)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Kind() != KindObject {
		t.Errorf("kind: got %v, want Object", got.Kind())
	}
}

func TestFilterUpperLower(t *testing.T) {
	if got, err := call(t, "upper", Str("hello")); err != nil || got.String() != "HELLO" {
		t.Errorf("upper: got (%q, %v), want (%q, nil)", got.String(), err, "HELLO")
	}
	if got, err := call(t, "lower", Str("HELLO")); err != nil || got.String() != "hello" {
		t.Errorf("lower: got (%q, %v), want (%q, nil)", got.String(), err, "hello")
	}
}

func TestFilterUpper_ErrorsOnStructuredValue(t *testing.T) {
	// given an array, when upper called, then errors naming | string (why:
	// upper on structure is never meaningful — must not silently stringify)
	_, err := call(t, "upper", Of([]any{"a"}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFilterSplit(t *testing.T) {
	got, err := call(t, "split", Str(","), Str("a,,b"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Kind() != KindArray {
		t.Fatalf("kind: got %v, want Array", got.Kind())
	}
	if got.String() != `["a","b"]` {
		t.Errorf("got %q, want %q", got.String(), `["a","b"]`)
	}
}

func TestFilterLines(t *testing.T) {
	got, err := call(t, "lines", Str("  foo  \n\n  bar  \n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != `["foo","bar"]` {
		t.Errorf("got %q, want %q", got.String(), `["foo","bar"]`)
	}
}

func TestFilterFirst(t *testing.T) {
	tests := []struct {
		name  string
		input Value
		want  string
	}{
		{"given array of strings, when first called, then returns first element", Of([]any{"v0.3.0", "v0.2.1"}), "v0.3.0"},
		{"given array of numbers, when first called, then returns first element typed (why: bug — first used to error unmarshaling a number into string)", Of([]any{json.Number("1"), json.Number("2"), json.Number("3")}), "1"},
		{"given array of objects, when first called, then returns first element typed (why: bug — first used to error unmarshaling an object into string)", Of([]any{map[string]any{"a": "b"}}), `{"a":"b"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := call(t, "first", test.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != test.want {
				t.Errorf("got %q, want %q", got.String(), test.want)
			}
		})
	}
}

func TestFilterFirst_EmptyArrayReturnsNil(t *testing.T) {
	got, err := call(t, "first", Of([]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Kind() != KindNil {
		t.Errorf("kind: got %v, want Nil", got.Kind())
	}
}

func TestFilterFirst_ErrorsOnString(t *testing.T) {
	// given a String — even one holding valid JSON array text — when first
	// called, then errors naming | json (why: first must never auto-parse;
	// that's the discipline that keeps type confusion from creeping back in)
	_, err := call(t, "first", Str(`["a","b","c"]`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFilterPluck(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		input Value
		want  string
	}{
		{"given top-level key, when plucked, then returns string value", "name", Of(map[string]any{"name": "hobnob"}), "hobnob"},
		{"given nested path with array index, when plucked, then traverses object and array", "data.items[0].name",
			Of(map[string]any{"data": map[string]any{"items": []any{map[string]any{"name": "first"}, map[string]any{"name": "second"}}}}), "first"},
		{"given numeric leaf, when plucked, then returns plain number text", "count", Of(map[string]any{"count": json.Number("3")}), "3"},
		{"given a slice path, when plucked, then returns matched elements as JSON array", "items[1:3]",
			Of(map[string]any{"items": []any{"a", "b", "c", "d"}}), `["b","c"]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := call(t, "pluck", Str(test.path), test.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != test.want {
				t.Errorf("got %q, want %q", got.String(), test.want)
			}
		})
	}
}

func TestFilterPluck_ErrorsOnString(t *testing.T) {
	// given a String piped in, when plucked, then errors naming | json (why:
	// pluck must never auto-parse — this is the fix for silent type
	// confusion where {"a":"[1,2,3]"} | pluck "a" | pluck "[0]" used to
	// return 1 by re-sniffing a string leaf as an array)
	_, err := call(t, "pluck", Str("a"), Str(`{"a":1}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFilterPluck_FallbackOnStringInput(t *testing.T) {
	// given a String piped in with a fallback, when plucked, then returns
	// the fallback rather than attempting to parse the string
	got, err := call(t, "pluck", Str("a"), Str("fallback"), Str(`{"a":1}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "fallback" {
		t.Errorf("got %q, want %q", got.String(), "fallback")
	}
}

func TestFilterPluck_MissingKeyErrors(t *testing.T) {
	_, err := call(t, "pluck", Str("missing"), Of(map[string]any{"name": "hobnob"}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFilterPluck_MissingKeyWithFallback(t *testing.T) {
	got, err := call(t, "pluck", Str("missing"), Str("fallback"), Of(map[string]any{"name": "hobnob"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "fallback" {
		t.Errorf("got %q, want %q", got.String(), "fallback")
	}
}

func TestFilterKeysValues(t *testing.T) {
	obj := Of(map[string]any{"us": "us-east-1", "eu": "eu-west-1"})

	keys, err := call(t, "keys", obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keys.String() != `["eu","us"]` {
		t.Errorf("keys: got %q, want %q", keys.String(), `["eu","us"]`)
	}

	values, err := call(t, "values", obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if values.String() != `["eu-west-1","us-east-1"]` {
		t.Errorf("values: got %q, want %q", values.String(), `["eu-west-1","us-east-1"]`)
	}
}

func TestFilterValues_ChildrenStayTypedForFurtherPluck(t *testing.T) {
	// given an object whose values are themselves objects, when values then
	// pluck called, then the nested field is reachable (why: bug — values
	// used to stringify each child, so a second pluck saw JSON text instead
	// of structure and failed with "no match")
	// Arrange
	obj := Of(map[string]any{"a": map[string]any{"x": json.Number("1")}})

	// Act
	values, err := call(t, "values", obj)
	if err != nil {
		t.Fatalf("values: unexpected error: %v", err)
	}
	got, err := call(t, "pluck", Str("[0].x"), values)

	// Assert
	if err != nil {
		t.Fatalf("pluck: unexpected error: %v", err)
	}
	if got.String() != "1" {
		t.Errorf("got %q, want %q", got.String(), "1")
	}
}

func TestFilterKeys_ErrorsOnArray(t *testing.T) {
	_, err := call(t, "keys", Of([]any{"a", "b"}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFilterJSONEscape(t *testing.T) {
	got, err := call(t, "jsonEscape", Str(`he said "hi"`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != `he said \"hi\"` {
		t.Errorf("got %q, want %q", got.String(), `he said \"hi\"`)
	}
}

func TestFilterJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    Value
		wantKind Kind
		wantErr  bool
	}{
		{"given a JSON array string, when json called, then parses to Array", Str(`[1,2,3]`), KindArray, false},
		{"given plain text, when json called, then errors", Str("not json"), KindNil, true},
		{"given an already-structured value, when json called, then identity", Of([]any{"a"}), KindArray, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := call(t, "json", test.input)
			if test.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Kind() != test.wantKind {
				t.Errorf("kind: got %v, want %v", got.Kind(), test.wantKind)
			}
		})
	}
}

func TestFilterJSON_FallbackOnParseFailure(t *testing.T) {
	// given malformed JSON text with a fallback, when json called, then
	// returns the fallback instead of erroring (why: same convention as
	// pluck's fallback — a caller can opt into graceful degradation on bad
	// source data rather than a hard failure)
	got, err := call(t, "json", Str("fallback"), Str("not-json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "fallback" {
		t.Errorf("got %q, want %q", got.String(), "fallback")
	}
}

func TestFilterString(t *testing.T) {
	// given a structured value, when string called, then returns its
	// compact JSON text as a String (why: the explicit way to opt back into
	// text after pluck/values/etc)
	got, err := call(t, "string", Of([]any{"a", "b"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Kind() != KindString {
		t.Errorf("kind: got %v, want String", got.Kind())
	}
	if got.String() != `["a","b"]` {
		t.Errorf("got %q, want %q", got.String(), `["a","b"]`)
	}
}

func TestFilterLen(t *testing.T) {
	tests := []struct {
		name  string
		input Value
		want  string
	}{
		{"given array, when len called, then element count", Of([]any{"a", "b", "c"}), "3"},
		{"given object, when len called, then key count", Of(map[string]any{"a": 1, "b": 2}), "2"},
		{"given string, when len called, then rune count", Str("hello"), "5"},
		{"given nil, when len called, then zero", Nil(), "0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := call(t, "len", test.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != test.want {
				t.Errorf("got %q, want %q", got.String(), test.want)
			}
		})
	}
}
