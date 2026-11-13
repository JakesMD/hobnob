package literal

import (
	"errors"
	"fmt"
	"strings"

	"hobnob/internal/eval"
	"hobnob/internal/value"
)

// ErrSpread is wrapped by every error for a spread written where no list can
// take its items: a single-value field, an object field, or an into: leaf
// outside a list. Callers that parse entry lists match it to give their own
// entry-level wording.
var ErrSpread = errors.New("spread outside a list")

const spreadMarker = "..."

// spreadError is a spread found where only one value fits.
type spreadError struct{ raw string }

func (e spreadError) Error() string {
	return fmt.Sprintf("%q spreads a list, so it only works as a list element; drop the ... or wrap it in [ ]", strings.TrimSpace(e.raw))
}

func (e spreadError) Is(target error) bool { return target == ErrSpread }

// leafGrammar turns a raw string leaf into the text stored for it, and
// returns the reference when the leaf is a spread (REF...).
type leafGrammar func(raw string) (text, spread string)

// templateLeaf is the grammar of every templated field: a reference (a bare
// accessor chain with optional filters) drops its braces and is wrapped back
// into the one-action template that keeps its type; anything else is stored
// as written. A spread is one reference, bare or a parenthesised pipeline,
// followed by "...".
func templateLeaf(raw string) (string, string) {
	if ref, ok := cutSpread(raw); ok {
		return wrapRef(ref), ref
	}
	if eval.IsBareRef(raw) {
		return wrapRef(raw), ""
	}
	return raw, ""
}

// intoLeaf is the grammar of into: leaves: a source head name plus accessors
// and filters, never a template to wrap, so the text is stored as written.
// Any leaf that isn't a {{ }} template and ends in "..." spreads that leaf.
func intoLeaf(raw string) (string, string) {
	if strings.Contains(raw, "{{") {
		return raw, ""
	}
	ref, ok := cutMarker(raw)
	if !ok || strings.TrimSpace(ref) == "" {
		return raw, ""
	}
	return ref, ref
}

// cutSpread reports whether elem is a spread of one reference. Text that
// merely ends in "..." is not, so "Loading..." and "{{ .A }}..." stay text.
func cutSpread(elem string) (string, bool) {
	ref, ok := cutMarker(elem)
	if !ok || !(eval.IsBareRef(ref) || eval.IsParenRef(ref)) {
		return "", false
	}
	return ref, true
}

func cutMarker(s string) (string, bool) {
	return strings.CutSuffix(strings.TrimSpace(s), spreadMarker)
}

// wrapRef turns a brace-free reference into the one-action template that
// keeps its type: ".A[0]" -> "{{.A[0]}}".
func wrapRef(ref string) string {
	return "{{" + ref + "}}"
}

// singleLeaf applies grammar to a field that holds one value, where a spread
// has nothing to splice into.
func singleLeaf(grammar leafGrammar, raw string) (string, error) {
	text, spread := grammar(raw)
	if spread != "" {
		return "", spreadError{raw: raw}
	}
	return text, nil
}

// spreadItems returns the items a spread of ref contributes to its list.
// Only a list can be spread; anything else is an error, like a wrong-kind
// accessor, and a string names | json as the way to parse it.
func spreadItems(ref string, v value.Value) ([]any, error) {
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
