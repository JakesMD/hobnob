package eval

import (
	"fmt"
	"strings"

	"hobnob/internal/value"
)

const spreadMarker = "..."

// CutSpread reports whether a list element is a spread: one reference (a
// bare accessor chain with optional filters, or a parenthesised pipeline
// with optional accessors) followed by "...". ref is the element without
// the marker. Text that merely ends in "..." is not a spread, so
// "Loading..." and "{{ .A }}..." stay text.
func CutSpread(elem string) (ref string, ok bool) {
	ref, ok = cutMarker(elem)
	if !ok || !(IsBareRef(ref) || isParenRef(ref)) {
		return "", false
	}
	return ref, true
}

// CutIntoSpread is CutSpread for an into: leaf, whose grammar is a source
// head name plus accessors and filters (see EvalIntoLeaf): any leaf that
// isn't a {{ }} template and ends in "..." is a spread of that leaf.
func CutIntoSpread(leaf string) (string, bool) {
	if strings.Contains(leaf, "{{") {
		return "", false
	}
	ref, ok := cutMarker(leaf)
	if !ok || strings.TrimSpace(ref) == "" {
		return "", false
	}
	return ref, true
}

func cutMarker(s string) (string, bool) {
	return strings.CutSuffix(strings.TrimSpace(s), spreadMarker)
}

// WrapRef turns a brace-free reference into the one-action template that
// keeps its type: ".A[0]" -> "{{.A[0]}}".
func WrapRef(ref string) string {
	return "{{" + ref + "}}"
}

// isParenRef reports whether s is a parenthesised pipeline used as a value,
// optionally followed by accessor steps: (.X | json) or (.X | json)[0].name.
func isParenRef(s string) bool {
	if s == "" || s[0] != '(' {
		return false
	}
	end, err := scanBalanced(s, 0, '(', ')')
	if err != nil {
		return false
	}
	if end == len(s) {
		return true
	}
	consumed, _, err := scanTerm(s, 0)
	return err == nil && consumed == len(s)
}

// SpreadItems returns the items a spread of ref contributes to its list.
// Only a list can be spread; anything else is an error, like a wrong-kind
// accessor, and a string names | json as the way to parse it.
func SpreadItems(ref string, v value.Value) ([]any, error) {
	switch v.Kind() {
	case value.KindArray:
		return v.Any().([]any), nil
	case value.KindMissing:
		return nil, v.MissingErr()
	case value.KindNil:
		return nil, fmt.Errorf("cannot spread %s: it is null or unset", ref)
	case value.KindString:
		return nil, fmt.Errorf("cannot spread %s: it is a string; pipe through | json to parse it first", ref)
	case value.KindObject:
		return nil, fmt.Errorf("cannot spread %s: it is an object; only a list can be spread", ref)
	default:
		return nil, fmt.Errorf("cannot spread %s: it is a %s; only a list can be spread", ref, v.Kind())
	}
}
