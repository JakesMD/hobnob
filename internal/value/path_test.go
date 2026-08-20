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
	// given a Missing sentinel
	m := Missing("boom")
	// when checked
	// then IsMissing is true and MissingErr carries the message
	if !m.IsMissing() {
		t.Fatal("expected IsMissing() to be true")
	}
	if err := m.MissingErr(); err == nil || err.Error() != "boom" {
		t.Fatalf("expected MissingErr() to be %q, got %v", "boom", err)
	}
	if Str("x").IsMissing() {
		t.Fatal("expected a plain string to not be missing")
	}
}

func TestMissing_StringReturnsMarkerNotEmpty(t *testing.T) {
	// given a Missing sentinel (why: String() must never return "" for a
	// missing value -- that would be indistinguishable from a legitimately
	// empty string and silently swallow the error at render position)
	m := Missing("path .A.b not found")
	// when rendered
	s := m.String()
	// then it is non-empty and scannable back out
	if s == "" {
		t.Fatal("expected String() to be non-empty for a missing value")
	}
	msg, ok := ScanMissing(s)
	if !ok || msg != "path .A.b not found" {
		t.Fatalf("ScanMissing(%q) = %q, %v; want %q, true", s, msg, ok, "path .A.b not found")
	}
}

func TestScanMissing_NoMarkerInOrdinaryText(t *testing.T) {
	if _, ok := ScanMissing("hello world"); ok {
		t.Fatal("expected no marker found in ordinary text")
	}
	if _, ok := ScanMissing(""); ok {
		t.Fatal("expected no marker found in empty text")
	}
}

func TestScanMissing_FindsMarkerEmbeddedInSurroundingText(t *testing.T) {
	// given a missing value rendered in the middle of other text
	embedded := "prefix " + Missing("oops").String() + " suffix"
	// when scanned
	msg, ok := ScanMissing(embedded)
	// then the message is recovered
	if !ok || msg != "oops" {
		t.Fatalf("ScanMissing(%q) = %q, %v; want %q, true", embedded, msg, ok, "oops")
	}
}

func TestKind_Missing(t *testing.T) {
	if Missing("x").Kind() != KindMissing {
		t.Fatalf("expected Kind() == KindMissing, got %v", Missing("x").Kind())
	}
	if KindMissing.String() != "missing" {
		t.Fatalf("expected KindMissing.String() == missing, got %v", KindMissing.String())
	}
}

func TestPath_ObjectKey(t *testing.T) {
	root := mustParse(t, `{"profile":{"name":"Ada"}}`)
	got, err := Path(".RESP.profile.name", root, []Value{Str("profile"), Str("name")})
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if got.Kind() != KindString || got.String() != "Ada" {
		t.Fatalf("got %v (%v), want Ada", got.String(), got.Kind())
	}
}

func TestPath_ObjectKeyMissingIsDeferred(t *testing.T) {
	root := mustParse(t, `{"profile":{}}`)
	got, err := Path(".RESP.profile.name", root, []Value{Str("profile"), Str("name")})
	if err != nil {
		t.Fatalf("Path: unexpected hard error %v", err)
	}
	if !got.IsMissing() {
		t.Fatalf("expected a missing sentinel, got %v", got)
	}
}

func TestPath_ArrayIndexPositiveNegativeOutOfRange(t *testing.T) {
	root := mustParse(t, `["a","b","c"]`)

	if got, err := Path(".A[0]", root, []Value{Num(0)}); err != nil || got.String() != "a" {
		t.Fatalf("[0] = %v, %v; want a, nil", got, err)
	}
	if got, err := Path(".A[-1]", root, []Value{Num(-1)}); err != nil || got.String() != "c" {
		t.Fatalf("[-1] = %v, %v; want c, nil", got, err)
	}
	got, err := Path(".A[5]", root, []Value{Num(5)})
	if err != nil {
		t.Fatalf("[5]: unexpected hard error %v", err)
	}
	if !got.IsMissing() {
		t.Fatalf("expected [5] out of range to be a missing sentinel, got %v", got)
	}
}

func TestPath_NonIntegralIndexIsHardError(t *testing.T) {
	root := mustParse(t, `["a","b"]`)
	_, err := Path(".A[1.5]", root, []Value{Of(json.Number("1.5"))})
	if err == nil {
		t.Fatal("expected a hard error for a non-integral index")
	}
}

func TestPath_WrongKindIsHardErrorNamingJSON(t *testing.T) {
	root := Str("just a string")
	_, err := Path(".S.field", root, []Value{Str("field")})
	if err == nil {
		t.Fatal("expected a hard error for key access on a string")
	}
	if !strings.Contains(err.Error(), "| json") {
		t.Fatalf("expected error to name | json, got %v", err)
	}
}

func TestPath_StarOnArray(t *testing.T) {
	root := mustParse(t, `[{"name":"a"},{"name":"b"}]`)
	got, err := Path(".ITEMS[*].name", root, []Value{Star(), Str("name")})
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
	// given a heterogeneous array where only some elements have "name"
	root := mustParse(t, `[{"name":"a"},{"other":1},{"name":"c"}]`)
	// when mapping .name over every element
	got, err := Path(".ITEMS[*].name", root, []Value{Star(), Str("name")})
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	// then the element missing "name" is dropped, not nil
	arr := got.Any().([]any)
	if len(arr) != 2 || arr[0] != "a" || arr[1] != "c" {
		t.Fatalf("got %v, want [a c]", arr)
	}
}

func TestPath_StarEmptyResultIsEmptyArrayNotError(t *testing.T) {
	root := mustParse(t, `[]`)
	got, err := Path(".ITEMS[*]", root, []Value{Star()})
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
	root := mustParse(t, `{"b":2,"a":1}`)
	got, err := Path(".CFG[*]", root, []Value{Star()})
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
	_, err := Path(".S[*]", Str("x"), []Value{Star()})
	if err == nil {
		t.Fatal("expected an error for [*] on a scalar")
	}
}

func TestPath_SliceClamp(t *testing.T) {
	root := mustParse(t, `["a","b","c"]`)
	got, err := Path(".A[0:99]", root, []Value{Slice(Num(0), Num(99))})
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	arr := got.Any().([]any)
	if len(arr) != 3 {
		t.Fatalf("got %v, want all 3 elements (clamped)", arr)
	}
}

func TestPath_SliceAbsentBounds(t *testing.T) {
	root := mustParse(t, `["a","b","c"]`)
	got, err := Path(".A[:2]", root, []Value{Slice(Str(""), Num(2))})
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	arr := got.Any().([]any)
	if len(arr) != 2 || arr[0] != "a" || arr[1] != "b" {
		t.Fatalf("got %v, want [a b]", arr)
	}
}

func TestPath_SliceOnObjectErrors(t *testing.T) {
	root := mustParse(t, `{"a":1}`)
	_, err := Path(".CFG[0:1]", root, []Value{Slice(Num(0), Num(1))})
	if err == nil {
		t.Fatal("expected an error slicing an object")
	}
	if !strings.Contains(err.Error(), "[*]") {
		t.Fatalf("expected error to mention [*] as the alternative, got %v", err)
	}
}

func TestPath_DynamicKeyPropagatesMissing(t *testing.T) {
	// given a dynamic key that is itself a missing sentinel (e.g. .A[.B.c]
	// where .B.c doesn't exist)
	root := mustParse(t, `{"x":1}`)
	dynamicKey := Missing(".B.c: \"c\" not found")
	// when used as a step
	got, err := Path(".A[.B.c]", root, []Value{dynamicKey})
	if err != nil {
		t.Fatalf("Path: unexpected hard error %v", err)
	}
	// then the inner missing path is what propagates, not a fresh miss
	if !got.IsMissing() || got.MissingErr().Error() != dynamicKey.MissingErr().Error() {
		t.Fatalf("got %v, want the inner missing sentinel to propagate", got)
	}
}

func TestPath_RootMissingPropagates(t *testing.T) {
	got, err := Path(".A.b", Missing("root gone"), []Value{Str("b")})
	if err != nil {
		t.Fatalf("Path: unexpected hard error %v", err)
	}
	if !got.IsMissing() {
		t.Fatal("expected the root's missing sentinel to propagate")
	}
}

func TestPathCall_And_StarCall_And_SliceCall(t *testing.T) {
	root := mustParse(t, `{"a":1}`)
	got, err := PathCall([]Value{Str(".X.a"), root, Str("a")})
	if err != nil || got.String() != "1" {
		t.Fatalf("PathCall = %v, %v; want 1, nil", got, err)
	}
	if _, err := StarCall(nil); err != nil {
		t.Fatalf("StarCall: %v", err)
	}
	if _, err := SliceCall([]Value{Num(0), Num(1)}); err != nil {
		t.Fatalf("SliceCall: %v", err)
	}
	if _, err := SliceCall([]Value{Num(0)}); err == nil {
		t.Fatal("expected SliceCall to error on wrong arity")
	}
}
