package eval

import (
	"fmt"
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
	funcs := make(template.FuncMap, len(value.Filters))
	for name, filter := range value.Filters {
		funcs[name] = adaptFilter(filter)
	}
	return funcs
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
