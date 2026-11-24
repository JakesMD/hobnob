package value

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) Value {
	t.Helper()
	v, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return v
}

func TestMissing_IsMissing(t *testing.T) {
	// given a Missing sentinel and a plain string, when checked, then only
	// the sentinel reports IsMissing and its MissingErr carries the message
	// (why: callers tell absence apart from real values by this alone)
	// Arrange
	m := Missing("boom")
	plain := Str("x")

	// Act
	missing, err, plainMissing := m.IsMissing(), m.MissingErr(), plain.IsMissing()

	// Assert
	if !missing {
		t.Fatal("expected IsMissing() to be true")
	}
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected MissingErr() to be %q, got %v", "boom", err)
	}
	if plainMissing {
		t.Fatal("expected a plain string to not be missing")
	}
}

func TestMissing_StringReturnsMarkerNotEmpty(t *testing.T) {
	// given a Missing sentinel, when rendered, then it is non-empty and
	// scannable back out (why: an empty string would be indistinguishable
	// from a legitimately empty value and silently swallow the error at
	// render position)
	// Arrange
	m := Missing("path .A.b not found")

	// Act
	s := m.String()
	msg, ok := ScanMissing(s)

	// Assert
	if s == "" {
		t.Fatal("expected String() to be non-empty for a missing value")
	}
	if !ok || msg != "path .A.b not found" {
		t.Fatalf("ScanMissing(%q) = %q, %v; want %q, true", s, msg, ok, "path .A.b not found")
	}
}

func TestScanMissing_NoMarkerInOrdinaryText(t *testing.T) {
	// given ordinary and empty text, when scanned, then no marker is found
	// (why: rendered output without a missing value must pass through)
	// Arrange
	ordinary, empty := "hello world", ""

	// Act
	_, inOrdinary := ScanMissing(ordinary)
	_, inEmpty := ScanMissing(empty)

	// Assert
	if inOrdinary {
		t.Fatal("expected no marker found in ordinary text")
	}
	if inEmpty {
		t.Fatal("expected no marker found in empty text")
	}
}

func TestScanMissing_FindsMarkerEmbeddedInSurroundingText(t *testing.T) {
	// given a missing value rendered in the middle of other text, when
	// scanned, then the message is recovered (why: a template rarely
	// renders a missing value on its own)
	// Arrange
	embedded := "prefix " + Missing("oops").String() + " suffix"

	// Act
	msg, ok := ScanMissing(embedded)

	// Assert
	if !ok || msg != "oops" {
		t.Fatalf("ScanMissing(%q) = %q, %v; want %q, true", embedded, msg, ok, "oops")
	}
}

func TestKind_Missing(t *testing.T) {
	// given a Missing sentinel, when its kind is asked, then it is
	// KindMissing, named "missing" (why: error messages name the kind)
	// Arrange
	m := Missing("x")

	// Act
	kind := m.Kind()

	// Assert
	if kind != KindMissing {
		t.Fatalf("expected Kind() == KindMissing, got %v", kind)
	}
	if kind.String() != "missing" {
		t.Fatalf("expected KindMissing.String() == missing, got %v", kind.String())
	}
}

func TestPath_ObjectKey(t *testing.T) {
	// given a nested object, when a key path is followed, then the leaf
	// value is returned with its kind (why: the basic accessor case)
	// Arrange
	root := mustParse(t, `{"profile":{"name":"Ada"}}`)

	// Act
	got, err := Path(".RESP.profile.name", root, []Value{Str("profile"), Str("name")})

	// Assert
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got.Kind() != KindString || got.String() != "Ada" {
		t.Fatalf("got %v (%v), want Ada", got.String(), got.Kind())
	}
}

func TestPath_ObjectKeyMissingIsDeferred(t *testing.T) {
	// given an object without the key, when the path is followed, then a
	// missing sentinel is returned, not a hard error (why: | default must be
	// able to catch it)
	// Arrange
	root := mustParse(t, `{"profile":{}}`)

	// Act
	got, err := Path(".RESP.profile.name", root, []Value{Str("profile"), Str("name")})

	// Assert
	if err != nil {
		t.Fatalf("Path: unexpected hard error %v", err)
	}
	if !got.IsMissing() {
		t.Fatalf("expected a missing sentinel, got %v", got)
	}
}

func TestPath_ArrayIndexPositiveNegativeOutOfRange(t *testing.T) {
	// given a 3-element array, when indexed with 0, -1 and 5, then the first
	// and last elements come back and the out-of-range index is a missing
	// sentinel (why: an absent element is absence, which | default catches)
	// Arrange
	root := mustParse(t, `["a","b","c"]`)

	// Act
	first, firstErr := Path(".A[0]", root, []Value{Num(0)})
	last, lastErr := Path(".A[-1]", root, []Value{Num(-1)})
	beyond, beyondErr := Path(".A[5]", root, []Value{Num(5)})

	// Assert
	if firstErr != nil || first.String() != "a" {
		t.Fatalf("[0] = %v, %v; want a, nil", first, firstErr)
	}
	if lastErr != nil || last.String() != "c" {
		t.Fatalf("[-1] = %v, %v; want c, nil", last, lastErr)
	}
	if beyondErr != nil {
		t.Fatalf("[5]: unexpected hard error %v", beyondErr)
	}
	if !beyond.IsMissing() {
		t.Fatalf("expected [5] out of range to be a missing sentinel, got %v", beyond)
	}
}

func TestPath_NonIntegralIndexIsHardError(t *testing.T) {
	// given an array, when indexed with 1.5, then it is a hard error (why: a
	// malformed index is a bug, not absence, so | default must not hide it)
	// Arrange
	root := mustParse(t, `["a","b"]`)

	// Act
	_, err := Path(".A[1.5]", root, []Value{Of(json.Number("1.5"))})

	// Assert
	if err == nil {
		t.Fatal("expected a hard error for a non-integral index")
	}
}

func TestPath_WrongKindIsHardErrorNamingJSON(t *testing.T) {
	// given a plain string, when a key is accessed on it, then it is a hard
	// error naming | json (why: the usual cause is JSON text that was never
	// parsed, and the error should say how to fix it)
	// Arrange
	root := Str("just a string")

	// Act
	_, err := Path(".S.field", root, []Value{Str("field")})

	// Assert
	if err == nil {
		t.Fatal("expected a hard error for key access on a string")
	}
	if !strings.Contains(err.Error(), "| json") {
		t.Fatalf("expected error to name | json, got %v", err)
	}
}

func TestPath_StarOnArray(t *testing.T) {
	// given an array of objects, when mapped with [*].name, then an Array of
	// the names comes back (why: the basic wildcard case)
	// Arrange
	root := mustParse(t, `[{"name":"a"},{"name":"b"}]`)

	// Act
	got, err := Path(".ITEMS[*].name", root, []Value{Star(), Str("name")})

	// Assert
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got.Kind() != KindArray {
		t.Fatalf("expected an Array, got %v", got.Kind())
	}
	arr := got.Any().([]any)
	if len(arr) != 2 || arr[0] != "a" || arr[1] != "b" {
		t.Fatalf("got %v, want [a b]", arr)
	}
}

func TestPath_StarDropsNoMatchElements(t *testing.T) {
	// given a heterogeneous array where only some elements have "name", when
	// mapping .name over every element, then the element missing "name" is
	// dropped, not nil (why: multiplicity is not absence)
	// Arrange
	root := mustParse(t, `[{"name":"a"},{"other":1},{"name":"c"}]`)

	// Act
	got, err := Path(".ITEMS[*].name", root, []Value{Star(), Str("name")})

	// Assert
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	arr := got.Any().([]any)
	if len(arr) != 2 || arr[0] != "a" || arr[1] != "c" {
		t.Fatalf("got %v, want [a c]", arr)
	}
}

func TestPath_StarEmptyResultIsEmptyArrayNotError(t *testing.T) {
	// given an empty array, when mapped with [*], then the result is an
	// empty Array rendering as [] (why: a wildcard result is always an
	// Array, so consumers like loop: just do nothing)
	// Arrange
	root := mustParse(t, `[]`)

	// Act
	got, err := Path(".ITEMS[*]", root, []Value{Star()})

	// Assert
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got.Kind() != KindArray || len(got.Any().([]any)) != 0 {
		t.Fatalf("got %v, want an empty array", got)
	}
	if got.String() != "[]" {
		t.Fatalf("got %q, want []", got.String())
	}
}

func TestPath_StarOnObjectYieldsValuesSortedByKey(t *testing.T) {
	// given an object, when accessed with [*], then its values come back
	// ordered by key (why: map iteration order is random, and output must be
	// deterministic)
	// Arrange
	root := mustParse(t, `{"b":2,"a":1}`)

	// Act
	got, err := Path(".CFG[*]", root, []Value{Star()})

	// Assert
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	arr := got.Any().([]any)
	if len(arr) != 2 {
		t.Fatalf("got %v, want 2 elements", arr)
	}
	if arr[0].(json.Number).String() != "1" || arr[1].(json.Number).String() != "2" {
		t.Fatalf("got %v, want sorted-key order [1 2]", arr)
	}
}

func TestPath_StarOnScalarErrors(t *testing.T) {
	// given a scalar, when accessed with [*], then it errors (why: there is
	// nothing to iterate, and returning the scalar would hide the mistake)
	// Arrange
	root := Str("x")

	// Act
	_, err := Path(".S[*]", root, []Value{Star()})

	// Assert
	if err == nil {
		t.Fatal("expected an error for [*] on a scalar")
	}
}

func TestPath_SliceClamp(t *testing.T) {
	// given a 3-element array, when sliced [0:99], then the range clamps to
	// the array's length (why: "the first N" must work on lists shorter
	// than N)
	// Arrange
	root := mustParse(t, `["a","b","c"]`)

	// Act
	got, err := Path(".A[0:99]", root, []Value{Slice(Num(0), Num(99))})

	// Assert
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	arr := got.Any().([]any)
	if len(arr) != 3 {
		t.Fatalf("got %v, want all 3 elements (clamped)", arr)
	}
}

func TestPath_SliceAbsentBounds(t *testing.T) {
	// given a 3-element array, when sliced [:2], then the absent start
	// defaults to 0 (why: an omitted bound means "from the start", as in Go)
	// Arrange
	root := mustParse(t, `["a","b","c"]`)

	// Act
	got, err := Path(".A[:2]", root, []Value{Slice(Str(""), Num(2))})

	// Assert
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	arr := got.Any().([]any)
	if len(arr) != 2 || arr[0] != "a" || arr[1] != "b" {
		t.Fatalf("got %v, want [a b]", arr)
	}
}

func TestPath_SliceOnObjectErrors(t *testing.T) {
	// given an object, when sliced, then it errors suggesting [*] (why: an
	// object has no order to slice, and [*] is what the author likely
	// wanted)
	// Arrange
	root := mustParse(t, `{"a":1}`)

	// Act
	_, err := Path(".CFG[0:1]", root, []Value{Slice(Num(0), Num(1))})

	// Assert
	if err == nil {
		t.Fatal("expected an error slicing an object")
	}
	if !strings.Contains(err.Error(), "[*]") {
		t.Fatalf("expected error to mention [*] as the alternative, got %v", err)
	}
}

func TestPath_DynamicKeyPropagatesMissing(t *testing.T) {
	// given a dynamic key that is itself a missing sentinel (e.g. .A[.B.c]
	// where .B.c doesn't exist), when used as a step, then the inner missing
	// path is what propagates, not a fresh miss (why: the error must name
	// the path that's actually absent)
	// Arrange
	root := mustParse(t, `{"x":1}`)
	dynamicKey := Missing(".B.c: \"c\" not found")

	// Act
	got, err := Path(".A[.B.c]", root, []Value{dynamicKey})

	// Assert
	if err != nil {
		t.Fatalf("Path: unexpected hard error %v", err)
	}
	if !got.IsMissing() || got.MissingErr().Error() != dynamicKey.MissingErr().Error() {
		t.Fatalf("got %v, want the inner missing sentinel to propagate", got)
	}
}

func TestPath_RootMissingPropagates(t *testing.T) {
	// given a root that is already missing, when a path is followed from it,
	// then the root's sentinel propagates (why: the error must name the
	// first thing that was absent)
	// Arrange
	root := Missing("root gone")

	// Act
	got, err := Path(".A.b", root, []Value{Str("b")})

	// Assert
	if err != nil {
		t.Fatalf("Path: unexpected hard error %v", err)
	}
	if !got.IsMissing() {
		t.Fatal("expected the root's missing sentinel to propagate")
	}
}

func TestPathCall_And_StarCall_And_SliceCall(t *testing.T) {
	// given the template-facing wrappers, when called with valid and invalid
	// arguments, then they delegate to Path and reject a wrong arity (why:
	// the accessor rewriter emits these calls, so their argument contract
	// must hold)
	// Arrange
	root := mustParse(t, `{"a":1}`)

	// Act
	got, pathErr := PathCall([]Value{Str(".X.a"), root, Str("a")})
	_, starErr := StarCall(nil)
	_, sliceErr := SliceCall([]Value{Num(0), Num(1)})
	_, arityErr := SliceCall([]Value{Num(0)})

	// Assert
	if pathErr != nil || got.String() != "1" {
		t.Fatalf("PathCall = %v, %v; want 1, nil", got, pathErr)
	}
	if starErr != nil {
		t.Fatalf("StarCall: %v", starErr)
	}
	if sliceErr != nil {
		t.Fatalf("SliceCall: %v", sliceErr)
	}
	if arityErr == nil {
		t.Fatal("expected SliceCall to error on wrong arity")
	}
}
