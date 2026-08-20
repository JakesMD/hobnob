package eval

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"text/template"

	"hobnob/internal/value"
)

// templateFuncs is computed once and reused across EvalTemplate calls —
// EvalTemplate is the hottest path in the codebase (called per var/condition/
// dir template), and rebuilding this map on every call showed up as overhead
// under loop-heavy tasks. EvalRunIntoPipe (pipe.go) reuses this same map via
// EvalTemplate rather than maintaining a second filter switch, so every
// filter here works in both {{ }} templates and run: into: pipes.
//
// Every filter is defined once, in value.Filters — this adapts each into
// text/template's FuncMap so ordinary {{ }} execution and EvalValue's
// type-preserving evaluator (chainvalue.go) share one implementation.
var templateFuncs = buildTemplateFuncs()

func buildTemplateFuncs() template.FuncMap {
	funcs := make(template.FuncMap, len(value.Filters)+6)
	for name, filter := range value.Filters {
		funcs[name] = adaptFilter(filter)
	}
	// text/template's builtin eq/ne/lt/le/gt/ge compare via each arg's raw
	// reflect.Kind, which for a value.Value is always Struct — so a bare
	// `{{eq .OS "darwin"}}` fails with "incompatible types for comparison"
	// even though .OS holds a plain string. These overrides unwrap each
	// value.Value/any argument to its native scalar before comparing.
	//
	// Assigned after the value.Filters loop above, so a future filter named
	// eq/ne/lt/le/gt/ge would be silently shadowed by these.
	funcs["eq"] = templateEq
	funcs["ne"] = templateNe
	funcs["lt"] = templateLt
	funcs["le"] = templateLe
	funcs["gt"] = templateGt
	funcs["ge"] = templateGe
	return funcs
}

// isJSONNumber reports whether s is exactly a JSON number literal. The
// first-byte check plus json.Valid is the JSON number grammar itself: it
// admits -3, 1.5, 1e9 and rejects "", "Inf", "NaN", "0x10", "007", "+3",
// "3/2" — every spelling strconv.ParseFloat or big.Rat would otherwise
// accept but that no JSON document could have produced.
func isJSONNumber(s string) bool {
	if s == "" {
		return false
	}
	if c := s[0]; c != '-' && (c < '0' || c > '9') {
		return false
	}
	return json.Valid([]byte(s))
}

// asNumber reports the exact rational value a Value represents, if any: a
// Number yields itself; a String yields one only when its text is exactly a
// JSON number literal (see isJSONNumber) — so "Inf"/"NaN"/"0x10"/"007"/"+3"
// are not numeric, even though they're valid float text. This is what makes
// comparison lenient across the coerce boundary — a template literal 3
// arrives as Str("3") (see coerce below), and needs to compare numerically
// against a real Number var without coerce itself having to guess a bare
// literal's intended type.
//
// Reads through String() rather than asserting Any()'s type directly:
// Kind()'s KindString is also its default case for any underlying type it
// doesn't recognize (see value.Value.Kind), so Any() is not guaranteed to be
// a Go string even when Kind() reports KindString. String() is total.
//
// big.Rat, not float64, because value.Value deliberately holds json.Number
// rather than float64 to avoid precision loss at large magnitudes (see
// value.go) — Rat.Cmp is exact at every magnitude a JSON number can encode.
func asNumber(v value.Value) (*big.Rat, bool) {
	switch v.Kind() {
	case value.KindNumber, value.KindString:
		s := v.String()
		if !isJSONNumber(s) {
			return nil, false
		}
		r, ok := new(big.Rat).SetString(s)
		return r, ok // ok is false only on an absurd exponent; caller falls back to lexical
	default:
		return nil, false
	}
}

// unorderable rejects kinds with no meaningful ordering: Array/Object (no
// structural comparison — see equalValues) and Bool (equality-only, matching
// text/template's own stdlib rule that bools can't be <, <=, >, >=).
func unorderable(k value.Kind) bool {
	return k == value.KindArray || k == value.KindObject || k == value.KindBool
}

// equalValues implements eq/ne: numeric equality when both sides parse as a
// number (regardless of which Kind carries it — see asNumber), else exact
// text equality via String(). A missing var (KindNil) stringifies to "" like
// everywhere else in hobnob (IsEmpty), so `eq .MISSING ""` needs no special
// case here. Arrays/Objects are refused: a captured value's String() returns
// its original raw text verbatim, so two structurally-identical objects
// captured with different whitespace would otherwise compare unequal.
func equalValues(a, b value.Value) (bool, error) {
	if a.Kind() == value.KindArray || a.Kind() == value.KindObject ||
		b.Kind() == value.KindArray || b.Kind() == value.KindObject {
		return false, fmt.Errorf("comparison not supported for %s and %s (no structural equality for array/object)", a.Kind(), b.Kind())
	}
	if an, aok := asNumber(a); aok {
		if bn, bok := asNumber(b); bok {
			return an.Cmp(bn) == 0, nil
		}
	}
	return a.String() == b.String(), nil
}

// orderValues implements lt/le/gt/ge: numeric ordering when both sides parse
// as a number, else lexical ordering via String(). Bools and Arrays/Objects
// are refused outright — see unorderable.
func orderValues(a, b value.Value) (int, error) {
	if unorderable(a.Kind()) || unorderable(b.Kind()) {
		return 0, fmt.Errorf("comparison not supported for %s and %s", a.Kind(), b.Kind())
	}
	if an, aok := asNumber(a); aok {
		if bn, bok := asNumber(b); bok {
			return an.Cmp(bn), nil
		}
	}
	return strings.Compare(a.String(), b.String()), nil
}

func templateEq(arg1 any, args ...any) (bool, error) {
	if len(args) == 0 {
		return false, fmt.Errorf("missing argument for comparison")
	}
	v1 := coerce(arg1)
	for _, a := range args {
		eq, err := equalValues(v1, coerce(a))
		if err != nil {
			return false, err
		}
		if eq {
			return true, nil
		}
	}
	return false, nil
}

func templateNe(arg1, arg2 any) (bool, error) {
	eq, err := templateEq(arg1, arg2)
	if err != nil {
		return false, err
	}
	return !eq, nil
}

func templateLt(arg1, arg2 any) (bool, error) {
	cmp, err := orderValues(coerce(arg1), coerce(arg2))
	if err != nil {
		return false, err
	}
	return cmp < 0, nil
}

func templateLe(arg1, arg2 any) (bool, error) {
	cmp, err := orderValues(coerce(arg1), coerce(arg2))
	if err != nil {
		return false, err
	}
	return cmp <= 0, nil
}

func templateGt(arg1, arg2 any) (bool, error) {
	cmp, err := orderValues(coerce(arg1), coerce(arg2))
	if err != nil {
		return false, err
	}
	return cmp > 0, nil
}

func templateGe(arg1, arg2 any) (bool, error) {
	cmp, err := orderValues(coerce(arg1), coerce(arg2))
	if err != nil {
		return false, err
	}
	return cmp >= 0, nil
}

// adaptFilter wraps a value.Filter for text/template. The piped parameter
// must be declared `any`, not value.Value: a missing var reaches here as an
// invalid reflect.Value, and template's validateType only lets that through
// for a nil-able parameter type — a struct type (value.Value) fails with
// "invalid value; expected value.Value", which would break {{ .MISSING |
// default "x" }}. coerce normalizes whatever text/template hands us — a raw
// string literal, that missing-var nil, or a prior filter's Value result —
// before the filter itself runs.
func adaptFilter(filter value.Filter) func(...any) (value.Value, error) {
	return func(args ...any) (value.Value, error) {
		coerced := make([]value.Value, len(args))
		for i, arg := range args {
			coerced[i] = coerce(arg)
		}
		return filter(coerced)
	}
}

func coerce(arg any) value.Value {
	switch typed := arg.(type) {
	case nil:
		return value.Nil()
	case value.Value:
		return typed
	case string:
		return value.Str(typed)
	default:
		return value.Str(fmt.Sprint(typed))
	}
}

// templateCache holds parsed templates keyed by source string. EvalTemplate
// is called per var/condition/dir template, often repeatedly for the same
// template text under loops — reparsing every call showed up as overhead
// under loop-heavy tasks. A parsed *template.Template is safe to Execute
// concurrently once built, so callers only ever race on the initial parse,
// which sync.Map.LoadOrStore resolves by keeping whichever copy won.
var templateCache sync.Map // string -> *template.Template

func parseTemplateCached(tmpl string) (*template.Template, error) {
	if cached, ok := templateCache.Load(tmpl); ok {
		return cached.(*template.Template), nil
	}
	parsed, err := template.New("").Funcs(templateFuncs).Parse(tmpl)
	if err != nil {
		return nil, err
	}
	actual, _ := templateCache.LoadOrStore(tmpl, parsed)
	return actual.(*template.Template), nil
}

// EvalTemplate renders tmpl to text. vars is passed as the template's dot —
// value.Value implements fmt.Stringer, so {{ .VAR }} prints exactly what
// Value.String() returns with no conversion needed. Use EvalValue instead
// when the result should keep its type rather than flatten to text.
func EvalTemplate(tmpl string, vars map[string]value.Value) (string, error) {
	parsed, err := parseTemplateCached(tmpl)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := parsed.Execute(&buf, vars); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ResolveItems renders a literal list or a template that resolves to an
// array into a []value.Value, each item still typed — a fromTmpl bare ref
// resolving to a real Array keeps its typed elements instead of stringifying
// them first. Used by execFor (loop: ITEM) and execFor's matrix form.
func ResolveItems(fromList []string, fromTmpl string, vars map[string]value.Value, ctxName string) ([]value.Value, error) {
	if len(fromList) > 0 {
		items := make([]value.Value, 0, len(fromList))
		for _, item := range fromList {
			rendered, err := EvalValue(item, vars)
			if err != nil {
				return nil, fmt.Errorf("%s from item: %w", ctxName, err)
			}
			items = append(items, rendered)
		}
		return items, nil
	}
	if fromTmpl != "" {
		rendered, err := EvalValue(fromTmpl, vars)
		if err != nil {
			return nil, fmt.Errorf("%s from template: %w", ctxName, err)
		}
		return ItemsFromValue(rendered), nil
	}
	return nil, nil
}

// ItemsFromValue turns a resolved Value into an item list for loop:/options:
// iteration: an Array's elements, typed; a Nil or empty String, zero items
// (nothing to iterate); anything else (including a plain String — even one
// that looks like JSON text but was never captured as such) is a single
// item, matching Capture's sniff-once discipline instead of re-parsing it.
func ItemsFromValue(v value.Value) []value.Value {
	if v.Kind() == value.KindArray {
		arr := v.Any().([]any)
		items := make([]value.Value, len(arr))
		for i, elem := range arr {
			items[i] = value.Of(elem)
		}
		return items
	}
	if v.IsEmpty() {
		return nil
	}
	return []value.Value{v}
}

// ResolveItemStrings is ResolveItems for callers that need plain text — get:
// options: displays and compares choices as strings, so typed elements are
// stringified after resolution rather than never being resolved typed.
func ResolveItemStrings(fromList []string, fromTmpl string, vars map[string]value.Value, ctxName string) ([]string, error) {
	items, err := ResolveItems(fromList, fromTmpl, vars, ctxName)
	if err != nil {
		return nil, err
	}
	strs := make([]string, len(items))
	for i, item := range items {
		strs[i] = item.String()
	}
	return strs, nil
}
