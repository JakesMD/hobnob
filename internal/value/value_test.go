package value

import "testing"

func TestKind(t *testing.T) {
	tests := []struct {
		name string
		v    Value
		want Kind
	}{
		{"given nil value, when kind checked, then KindNil (why: missing var or explicit null)", Nil(), KindNil},
		{"given string value, when kind checked, then KindString", Str("hello"), KindString},
		{"given bool value, when kind checked, then KindBool", Of(true), KindBool},
		{"given number value, when kind checked, then KindNumber", Num(42), KindNumber},
		{"given array value, when kind checked, then KindArray", Of([]any{"a"}), KindArray},
		{"given object value, when kind checked, then KindObject", Of(map[string]any{"a": "b"}), KindObject},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			got := test.v.Kind()

			// Assert
			if got != test.want {
				t.Errorf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name string
		v    Value
		want string
	}{
		{"given nil value, when stringified, then empty string (why: renders as \"\" in templates and shell)", Nil(), ""},
		{"given string value, when stringified, then value unchanged (why: strings pass through unquoted)", Str("hello"), "hello"},
		{"given bool value, when stringified, then plain bool text (why: chainable into shell if: conditions)", Of(true), "true"},
		{"given number value, when stringified, then plain number text (why: no float precision artifacts like 3.0)", Num(3), "3"},
		{"given array value, when stringified, then compact JSON (why: results stay chainable)", Of([]any{"a", "b"}), `["a","b"]`},
		{"given object value, when stringified, then compact JSON with sorted keys (why: deterministic output)", Of(map[string]any{"b": 1, "a": 2}), `{"a":2,"b":1}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			got := test.v.String()

			// Assert
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestString_CapturedValueKeepsRawText(t *testing.T) {
	// given a captured object whose original text is pretty-printed, when
	// stringified, then the original formatting is preserved (why: a
	// pretty-printed API response must stay byte-identical inside a run:
	// command, not silently re-marshal compact)
	// Arrange
	raw := "{\n  \"a\": 1\n}"

	// Act
	v := Capture(raw)

	// Assert
	if got := v.String(); got != raw {
		t.Errorf("got %q, want %q", got, raw)
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		v    Value
		want bool
	}{
		{"given nil value, when checked, then empty", Nil(), true},
		{"given empty string, when checked, then empty", Str(""), true},
		{"given non-empty string, when checked, then not empty", Str("x"), false},
		{"given empty array, when checked, then not empty (why: [] renders as non-empty text, matching get:'s optional-multi default)", Of([]any{}), false},
		{"given false bool, when checked, then not empty (why: \"false\" is non-empty text)", Of(false), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			got := test.v.IsEmpty()

			// Assert
			if got != test.want {
				t.Errorf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestCanonical(t *testing.T) {
	tests := []struct {
		name       string
		v          any
		wantString string
	}{
		{"given int, when canonicalized, then json.Number text", int(8080), "8080"},
		{"given float64 with fraction, when canonicalized, then decimal text (why: no float precision artifacts)", float64(1.5), "1.5"},
		{"given float64 whole number, when canonicalized, then no .0 suffix", float64(3), "3"},
		{"given string, when canonicalized, then unchanged", "hello", "hello"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			got := Of(Canonical(test.v))

			// Assert
			if got.String() != test.wantString {
				t.Errorf("got %q, want %q", got.String(), test.wantString)
			}
		})
	}

	t.Run("given int, when canonicalized, then Kind is KindNumber", func(t *testing.T) {
		if got := Of(Canonical(int(8080))).Kind(); got != KindNumber {
			t.Errorf("got %v, want %v", got, KindNumber)
		}
	})

	t.Run("given float64, when canonicalized, then Kind is KindNumber", func(t *testing.T) {
		if got := Of(Canonical(float64(1.5))).Kind(); got != KindNumber {
			t.Errorf("got %v, want %v", got, KindNumber)
		}
	})

	t.Run("given bool, when canonicalized, then unchanged and stays KindBool", func(t *testing.T) {
		if got := Of(Canonical(true)).Kind(); got != KindBool {
			t.Errorf("got %v, want %v", got, KindBool)
		}
	})

	t.Run("given nil, when canonicalized, then unchanged and stays KindNil", func(t *testing.T) {
		if got := Of(Canonical(nil)).Kind(); got != KindNil {
			t.Errorf("got %v, want %v", got, KindNil)
		}
	})

	t.Run("given array, when canonicalized, then unchanged and stays KindArray", func(t *testing.T) {
		if got := Of(Canonical([]any{"a"})).Kind(); got != KindArray {
			t.Errorf("got %v, want %v", got, KindArray)
		}
	})

	t.Run("given object, when canonicalized, then unchanged and stays KindObject", func(t *testing.T) {
		if got := Of(Canonical(map[string]any{"a": "b"})).Kind(); got != KindObject {
			t.Errorf("got %v, want %v", got, KindObject)
		}
	})
}

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantKind   Kind
		wantString string
		wantErr    bool
	}{
		{
			name:       "given a JSON array, when parsed, then decodes to Array",
			input:      `[1,2,3]`,
			wantKind:   KindArray,
			wantString: `[1,2,3]`,
		},
		{
			name:       "given a JSON object, when parsed, then decodes to Object",
			input:      `{"a":1}`,
			wantKind:   KindObject,
			wantString: `{"a":1}`,
		},
		{
			name:    "given trailing data after a JSON value, when parsed, then errors (why: Capture must not accept 42 garbage as a clean number)",
			input:   `[1,2] trailing`,
			wantErr: true,
		},
		{
			name:    "given malformed JSON, when parsed, then errors",
			input:   `not-json`,
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			got, err := Parse(test.input)

			// Assert
			if test.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (result: %q)", got.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Kind() != test.wantKind {
				t.Errorf("kind: got %v, want %v", got.Kind(), test.wantKind)
			}
			if got.String() != test.wantString {
				t.Errorf("string: got %q, want %q", got.String(), test.wantString)
			}
		})
	}
}

func TestCapture(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantKind Kind
	}{
		{
			name:     "given a JSON array, when captured, then becomes Array (why: run: into: capturing an API list is the common case)",
			input:    `[{"name":"ada"},{"name":"bob"}]`,
			wantKind: KindArray,
		},
		{
			name:     "given a JSON object, when captured, then becomes Object",
			input:    `{"a":1}`,
			wantKind: KindObject,
		},
		{
			name:     "given plain text, when captured, then stays String",
			input:    "hello\n",
			wantKind: KindString,
		},
		{
			name:     "given text starting with { that isn't valid JSON, when captured, then stays String (why: loop: over \"{not json}\" must not error)",
			input:    "{not json}",
			wantKind: KindString,
		},
		{
			name:     "given a bare number, when captured, then stays String (why: only [ and { prefixes are ever sniffed)",
			input:    "42",
			wantKind: KindString,
		},
		{
			name:     "given empty string, when captured, then stays String",
			input:    "",
			wantKind: KindString,
		},
		{
			name:     "given JSON array with trailing garbage, when captured, then stays String (why: not a clean single JSON value)",
			input:    `[1,2] and then some text`,
			wantKind: KindString,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			got := Capture(test.input)

			// Assert
			if got.Kind() != test.wantKind {
				t.Errorf("kind: got %v, want %v", got.Kind(), test.wantKind)
			}
		})
	}
}

func TestCapture_PlainStringRoundTripsExactly(t *testing.T) {
	// given plain non-JSON text, when captured, then String() returns it
	// unchanged (why: the vast majority of run: output is plain text, not
	// JSON — capture must not alter it)
	// Arrange
	input := "hello world\n"

	// Act
	v := Capture(input)

	// Assert
	if got := v.String(); got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}
