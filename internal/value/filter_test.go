package value

import (
	"strings"
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
	// given an empty, nil or set piped value, when default called, then the
	// fallback replaces only the empty and nil ones (why: | default fills
	// gaps without overriding a real value)
	// Arrange
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
			// Act
			got, err := call(t, "default", test.def, test.piped)

			// Assert
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
	// (why: command output usually ends in a newline that values shouldn't
	// carry)
	// Arrange
	input := Str("  hello  ")

	// Act
	got, err := call(t, "trim", input)

	// Assert
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
	// a captured API response before the next accessor)
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
	// given text, when upper and lower called, then the case changes (why:
	// the basic case transforms authors reach for in names and tags)
	// Arrange
	lowerText, upperText := Str("hello"), Str("HELLO")

	// Act
	upper, upperErr := call(t, "upper", lowerText)
	lower, lowerErr := call(t, "lower", upperText)

	// Assert
	if upperErr != nil || upper.String() != "HELLO" {
		t.Errorf("upper: got (%q, %v), want (%q, nil)", upper.String(), upperErr, "HELLO")
	}
	if lowerErr != nil || lower.String() != "hello" {
		t.Errorf("lower: got (%q, %v), want (%q, nil)", lower.String(), lowerErr, "hello")
	}
}

func TestFilterUpper_ErrorsOnStructuredValue(t *testing.T) {
	// given an array, when upper called, then errors naming | string (why:
	// upper on structure is never meaningful — must not silently stringify)
	// Arrange
	input := Of([]any{"a"})

	// Act
	_, err := call(t, "upper", input)

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFilterSplit(t *testing.T) {
	// given comma-separated text with an empty field, when split called, then
	// it becomes an Array without the empty element (why: a doubled separator
	// shouldn't produce blank items to loop over)
	// Arrange
	input := Str("a,,b")

	// Act
	got, err := call(t, "split", Str(","), input)

	// Assert
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

func TestFilterJoin(t *testing.T) {
	// given a list of strings, when join called, then elements joined by the
	// separator (why: the inverse of split, for comma-separated query params)

	// Arrange
	list := Of([]any{"history", "science"})

	// Act
	got, err := call(t, "join", Str(","), list)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Kind() != KindString {
		t.Fatalf("kind: got %v, want String", got.Kind())
	}
	if got.String() != "history,science" {
		t.Errorf("got %q, want %q", got.String(), "history,science")
	}
}

func TestFilterJoin_ScalarsJoinAsText(t *testing.T) {
	// given numbers and bools in the list, when join called, then each joins
	// as its text (why: JSON-captured lists mix scalars, e.g. IDs and flags)

	// Arrange
	list := Of([]any{float64(42), 1.5, true, "x"})

	// Act
	got, err := call(t, "join", Str(" "), list)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != "42 1.5 true x" {
		t.Errorf("got %q, want %q", got.String(), "42 1.5 true x")
	}
}

func TestFilterJoin_ErrorsOnNonTextElement(t *testing.T) {
	// given a list with a non-text element, when join called, then it errors
	// naming the element and the accessor to use (why: joining structure
	// produces text nobody meant)
	tests := []struct {
		name     string
		elem     any
		wantHint string
	}{
		{"given an object element, when join called, then errors pointing at a field accessor (why: compact JSON mid-string is never what was meant)", map[string]any{"k": "v"}, ".LIST[*].name"},
		{"given a nested list element, when join called, then errors pointing at an index accessor (why: a,[\"b\"] is never what was meant)", []any{"b"}, ".LIST[*][0]"},
		{"given a null element, when join called, then errors (why: a silent empty part, a,,b, hides missing data)", nil, "null"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			list := Of([]any{"a", test.elem})

			// Act
			_, err := call(t, "join", Str(","), list)

			// Assert
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			for _, want := range []string{"element [1]", test.wantHint} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err.Error(), want)
				}
			}
		})
	}
}

func TestFilterJoin_NilErrorsWithoutJSONHint(t *testing.T) {
	// given nil (an unset variable), when join called, then errors without
	// suggesting | json (why: there is no text to parse, so the hint misleads)

	// Arrange
	nothing := Nil()

	// Act
	_, err := call(t, "join", Str(","), nothing)

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "| json") {
		t.Errorf("error %q suggests | json for nil", err.Error())
	}
}

func TestFilterJoin_ErrorsWithoutSeparator(t *testing.T) {
	// given no separator, when join called, then errors (why: {{ .A | join }}
	// is a typo, not a request for a default separator)

	// Arrange
	list := Of([]any{"a", "b"})

	// Act
	_, err := call(t, "join", list)

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFilterJoin_ErrorsOnNonListNamingJSON(t *testing.T) {
	// given JSON-shaped text rather than a list, when join called, then errors
	// naming | json (why: env vars and CLI args arrive as text and must be
	// parsed explicitly; joining a string's characters is never meant)

	// Arrange
	text := Str(`["a","b"]`)

	// Act
	_, err := call(t, "join", Str(","), text)

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "| json") {
		t.Errorf("error %q does not name | json", err.Error())
	}
}

func TestFilterJoin_EmptyListGivesEmptyString(t *testing.T) {
	// given an empty list, when join called, then "" (why: a list that ends up
	// empty, e.g. split of "", must round-trip rather than error)

	// Arrange
	fromSplit, err := call(t, "split", Str(","), Str(""))
	if err != nil {
		t.Fatalf("split: unexpected error: %v", err)
	}
	tests := []struct {
		name string
		list Value
	}{
		{"literal empty list", Of([]any{})},
		{"empty split result", fromSplit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			got, err := call(t, "join", Str(","), test.list)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != "" {
				t.Errorf("got %q, want empty", got.String())
			}
		})
	}
}

func TestFilterLines(t *testing.T) {
	// given text with padding and blank lines, when lines called, then each
	// non-blank line is trimmed into an Array (why: turns command output into
	// something loop: can iterate)
	// Arrange
	input := Str("  foo  \n\n  bar  \n")

	// Act
	got, err := call(t, "lines", input)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != `["foo","bar"]` {
		t.Errorf("got %q, want %q", got.String(), `["foo","bar"]`)
	}
}

func TestFilterKeys(t *testing.T) {
	// given an object, when keys called, then its keys come back sorted (why:
	// map order is random, and output must be deterministic)
	// Arrange
	obj := Of(map[string]any{"us": "us-east-1", "eu": "eu-west-1"})

	// Act
	keys, err := call(t, "keys", obj)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keys.String() != `["eu","us"]` {
		t.Errorf("keys: got %q, want %q", keys.String(), `["eu","us"]`)
	}
}

func TestFilterKeys_ErrorsOnArray(t *testing.T) {
	// given an array, when keys called, then it errors (why: an array has no
	// keys, and returning indexes would hide the mistake)
	// Arrange
	input := Of([]any{"a", "b"})

	// Act
	_, err := call(t, "keys", input)

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFilterJSONEscape(t *testing.T) {
	// given text with double quotes, when jsonEscape called, then the quotes
	// are escaped (why: the text must be safe inside a hand-written JSON
	// string)
	// Arrange
	input := Str(`he said "hi"`)

	// Act
	got, err := call(t, "jsonEscape", input)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != `he said \"hi\"` {
		t.Errorf("got %q, want %q", got.String(), `he said \"hi\"`)
	}
}

func TestFilterJSON(t *testing.T) {
	// given JSON text, plain text or structure, when json called, then text
	// is parsed, non-JSON errors and structure passes through (why: | json is
	// the explicit way to turn text into structure)
	// Arrange
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
			// Act
			got, err := call(t, "json", test.input)

			// Assert
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

func TestFilterString(t *testing.T) {
	// given a structured value, when string called, then returns its
	// compact JSON text as a String (why: the explicit way to opt back into
	// text after values/keys/an accessor/etc)
	// Arrange
	input := Of([]any{"a", "b"})

	// Act
	got, err := call(t, "string", input)

	// Assert
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

func TestFilterQuote(t *testing.T) {
	// given text or structure, when quote called, then it becomes one
	// single-quoted shell word (why: a value spliced into a shell command
	// must stay one argument)
	// Arrange
	tests := []struct {
		name  string
		input Value
		want  string
	}{
		{"given plain string, when quote called, then wrapped in single quotes", Str("hello"), "'hello'"},
		{"given string with embedded single quote, when quote called, then quote escaped", Str("it's"), `'it'\''s'`},
		{"given empty string, when quote called, then empty quoted pair", Str(""), "''"},
		{"given array, when quote called, then compact JSON quoted (why: matches | string's stringify-first behaviour)", Of([]any{"a", "b"}), `'["a","b"]'`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			got, err := call(t, "quote", test.input)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Kind() != KindString {
				t.Errorf("kind: got %v, want String", got.Kind())
			}
			if got.String() != test.want {
				t.Errorf("got %q, want %q", got.String(), test.want)
			}
		})
	}
}

func TestFilterLen(t *testing.T) {
	// given an array, object, string or nil, when len called, then it counts
	// elements, keys, runes or zero (why: one filter answers "how many" for
	// every kind)
	// Arrange
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
			// Act
			got, err := call(t, "len", test.input)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != test.want {
				t.Errorf("got %q, want %q", got.String(), test.want)
			}
		})
	}
}
