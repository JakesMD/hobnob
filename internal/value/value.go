// Package value holds hobnob's typed scope value: a variable is a string,
// bool, number, array, or object — never opaque JSON-in-a-string. Structure
// enters scope from exactly three places: set:/with:/vars: map/list
// literals, run: into: capture (Capture), and the explicit json filter.
// Everything else (env, CLI args, env files, prompts) is a plain string.
package value

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Kind discriminates the shape a Value holds.
type Kind int

const (
	KindNil Kind = iota
	KindString
	KindBool
	KindNumber
	KindArray
	KindObject
	// KindMissing is a deferred "path not found" sentinel produced by an
	// accessor lookup — see Missing in path.go. Added at the end of the
	// block so no existing Kind's ordinal shifts.
	KindMissing
)

func (k Kind) String() string {
	switch k {
	case KindNil:
		return "nil"
	case KindString:
		return "string"
	case KindBool:
		return "bool"
	case KindNumber:
		return "number"
	case KindArray:
		return "array"
	case KindObject:
		return "object"
	case KindMissing:
		return "missing"
	default:
		return "unknown"
	}
}

// Value is a scope variable's value: nil, a string, a bool, a json.Number
// (never float64 — avoids precision loss and unwanted ".0" suffixes), a
// []any, or a map[string]any — or, only transiently within a single
// accessor evaluation, a deferred "path not found" sentinel (see Missing in
// path.go). Nested arrays/objects use the same element types recursively,
// so Any() feeds Path (path.go) directly with no adaptation.
type Value struct {
	v   any
	raw string // original captured text; set only by Capture, else ""
}

// Nil returns the empty Value — a missing var, or an explicit null leaf.
func Nil() Value { return Value{} }

// Str wraps a plain string. Every value entering scope from env, CLI args,
// env files, or a prompt goes through Str — never sniffed for JSON shape.
func Str(s string) Value { return Value{v: s} }

// Of wraps an already-decoded JSON tree (as produced by Parse, or built up
// by a filter from Value.Any() leaves). Callers are responsible for numbers
// being json.Number, not float64.
func Of(v any) Value { return Value{v: v} }

// Num wraps an int as a Value holding a json.Number.
func Num(n int) Value { return Value{v: json.Number(strconv.Itoa(n))} }

// Canonical converts a scalar decoded by a non-JSON decoder (YAML's
// int/float, say) into the type Value expects, so Kind() reports KindNumber
// rather than falling through to its KindString default — see Kind and Of's
// "callers are responsible for numbers being json.Number" note. Non-numeric
// scalars and already-canonical values pass through unchanged.
func Canonical(v any) any {
	switch n := v.(type) {
	case int:
		return json.Number(strconv.Itoa(n))
	case int64:
		return json.Number(strconv.FormatInt(n, 10))
	case uint64:
		return json.Number(strconv.FormatUint(n, 10))
	case float64:
		return json.Number(strconv.FormatFloat(n, 'g', -1, 64))
	case float32:
		return json.Number(strconv.FormatFloat(float64(n), 'g', -1, 32))
	default:
		return v
	}
}

// Kind reports which shape v holds.
func (v Value) Kind() Kind {
	switch v.v.(type) {
	case nil:
		return KindNil
	case string:
		return KindString
	case bool:
		return KindBool
	case json.Number:
		return KindNumber
	case []any:
		return KindArray
	case map[string]any:
		return KindObject
	case missing:
		return KindMissing
	default:
		return KindString
	}
}

// Any returns the underlying decoded value — nil, string, bool, json.Number,
// []any, or map[string]any — for jsonpath queries and JSON tree assembly.
func (v Value) Any() any { return v.v }

// String renders v as hobnob has always rendered scope values: a captured
// value keeps its original text verbatim (raw), a scalar renders plain, and
// a container re-marshals to compact JSON so results stay chainable.
// Implements fmt.Stringer so text/template renders {{ .VAR }} correctly.
func (v Value) String() string {
	if v.raw != "" {
		return v.raw
	}
	s, _ := stringify(v.v)
	return s
}

// IsEmpty reports whether v renders as the empty string — the "is this var
// unset" check used throughout get: and default.
func (v Value) IsEmpty() bool {
	return v.String() == ""
}

func stringify(v any) (string, error) {
	switch typed := v.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case bool:
		return strconv.FormatBool(typed), nil
	case json.Number:
		return typed.String(), nil
	case missing:
		return missingMarker(typed.msg), nil
	default:
		jsonBytes, err := json.Marshal(typed)
		if err != nil {
			return "", err
		}
		return string(jsonBytes), nil
	}
}

// Parse strictly decodes s as a single JSON value — trailing data after the
// value is an error, unlike a bare json.Decoder.Decode call. Numbers decode
// as json.Number.
func Parse(s string) (Value, error) {
	decoder := json.NewDecoder(strings.NewReader(s))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return Value{}, err
	}
	if decoder.More() {
		return Value{}, fmt.Errorf("trailing data after JSON value")
	}
	return Of(decoded), nil
}

// Capture is the sniff-once rule for run: into: stdout/stderr text: only a
// string whose trimmed form starts with '[' or '{' and decodes cleanly as a
// single JSON value becomes structured; anything else — including
// almost-JSON like "{not json}" — stays a String. This is the only place
// hobnob guesses at a string's shape; every filter and accessor step
// downstream trusts Kind() instead of re-sniffing (see keys/values/Path
// requiring Array/Object).
func Capture(s string) Value {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 || (trimmed[0] != '[' && trimmed[0] != '{') {
		return Str(s)
	}
	parsed, err := Parse(s)
	if err != nil {
		return Str(s)
	}
	parsed.raw = s
	return parsed
}
