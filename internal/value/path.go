package value

import (
	"fmt"
	"strconv"
	"strings"
)

// missing is the sentinel a Path lookup produces for an absent key or
// index — deferred so a later "| default" pipe stage can catch it. See
// Missing.
type missing struct{ msg string }

// Missing wraps msg as a deferred "path not found" error. It is not a Go
// error: evaluating a hbpath call must SUCCEED (return a nil error) so a
// later stage in the same pipeline — specifically default — gets the
// chance to see and swallow it. Every other filter raises it as a real
// error via the adaptFilter/evalFilterCommand guard before its own body
// runs; a sentinel that reaches bare render position (no filter at all) is
// caught by the poison-marker scan in eval.EvalTemplate instead, since
// Value.String() (an fmt.Stringer) cannot itself return an error.
func Missing(msg string) Value { return Value{v: missing{msg: msg}} }

// IsMissing reports whether v is a deferred "path not found" sentinel.
func (v Value) IsMissing() bool {
	_, ok := v.v.(missing)
	return ok
}

// MissingErr returns the deferred error a sentinel carries, or nil if v
// isn't one.
func (v Value) MissingErr() error {
	if m, ok := v.v.(missing); ok {
		return fmt.Errorf("%s", m.msg)
	}
	return nil
}

const (
	missingPrefix = "\x00hobnob:missing:"
	missingSuffix = "\x00"
)

func missingMarker(msg string) string { return missingPrefix + msg + missingSuffix }

// ScanMissing finds a poison marker embedded in rendered template output —
// the only way a sentinel that reached bare render position (no filter, so
// no adaptFilter guard ever ran) can still be caught. Value.String() cannot
// return an error, so it emits this marker instead of silently returning
// "" — the exact guess the value package exists to avoid. Do not
// "simplify" that away.
func ScanMissing(s string) (string, bool) {
	start := strings.Index(s, missingPrefix)
	if start < 0 {
		return "", false
	}
	rest := s[start+len(missingPrefix):]
	end := strings.IndexByte(rest, 0)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// starStep and sliceStep are the [*] and [lo:hi] step markers, emitted by
// the accessor rewriter (internal/eval/accessor.go) as the bare identifier
// hbstar and the call (hbslice lo hi).
type starStep struct{}
type sliceStep struct{ lo, hi Value }

// Star is the [*] step: every element of an Array, or every value of an
// Object (sorted-key order).
func Star() Value { return Value{v: starStep{}} }

// Slice is the [lo:hi] step. An absent bound (IsEmpty) means unbounded on
// that side, matching Go slice syntax.
func Slice(lo, hi Value) Value { return Value{v: sliceStep{lo: lo, hi: hi}} }

// Path evaluates an accessor chain: root is the value the chain starts
// from, steps is each subsequent .key / [index] / [*] / [lo:hi] in order,
// and src is the original accessor text as the user wrote it — used only
// to name the path in an error or a deferred Missing sentinel.
//
// A star or slice step turns multiplicity on for every step after it: from
// that point on a miss or a wrong-kind element is DROPPED rather than
// failing the whole lookup ("multiplicity is not absence" — DESIGN-PATH.md),
// and the final result is always an Array, even at length 0 or 1. Before
// multiplicity begins, a miss returns a deferred Missing sentinel
// (catchable by | default) and a wrong kind returns a real error (never
// catchable) — the "two error classes" DESIGN-PATH.md describes.
//
// A malformed index (non-integral, or not a number at all) against an
// Array is always a hard error, in multi mode too: unlike a heterogeneous
// element's kind, the step itself is the same for every element, so
// dropping it silently would just as often mean "this step is a task-file
// bug" as "this element doesn't have it".
func Path(src string, root Value, steps []Value) (Value, error) {
	if root.IsMissing() {
		return root, nil
	}
	nodes := []any{root.v}
	multi := false

	for _, step := range steps {
		if step.IsMissing() {
			return step, nil
		}
		var next []any
		stepMulti := false

		switch typed := step.v.(type) {
		case starStep:
			stepMulti = true
			for _, n := range nodes {
				cur := Value{v: n}
				switch cur.Kind() {
				case KindObject:
					obj := cur.v.(map[string]any)
					for _, k := range sortedKeys(obj) {
						next = append(next, obj[k])
					}
				case KindArray:
					next = append(next, cur.v.([]any)...)
				default:
					if multi {
						continue
					}
					return Value{}, fmt.Errorf("%s: cannot use [*] on a %s; pipe through | json to parse it first", src, cur.Kind())
				}
			}

		case sliceStep:
			stepMulti = true
			for _, n := range nodes {
				cur := Value{v: n}
				switch cur.Kind() {
				case KindArray:
					arr := cur.v.([]any)
					lo, hi := clampSlice(typed.lo, typed.hi, len(arr))
					next = append(next, arr[lo:hi]...)
				case KindObject:
					if multi {
						continue
					}
					return Value{}, fmt.Errorf("%s: cannot slice an object; use [*] for every value", src)
				default:
					if multi {
						continue
					}
					return Value{}, fmt.Errorf("%s: cannot slice a %s; pipe through | json to parse it first", src, cur.Kind())
				}
			}

		default:
			for _, n := range nodes {
				cur := Value{v: n}
				switch cur.Kind() {
				case KindObject:
					key := step.String()
					obj := cur.v.(map[string]any)
					val, ok := obj[key]
					if !ok {
						if multi {
							continue
						}
						return Missing(fmt.Sprintf("%s: %q not found", src, key)), nil
					}
					next = append(next, val)
				case KindArray:
					arr := cur.v.([]any)
					idx, ok := stepIndex(step)
					if !ok {
						return Value{}, fmt.Errorf("%s: %q is not a valid array index", src, step.String())
					}
					real := idx
					if real < 0 {
						real += len(arr)
					}
					if real < 0 || real >= len(arr) {
						if multi {
							continue
						}
						return Missing(fmt.Sprintf("%s: index %d out of range", src, idx)), nil
					}
					next = append(next, arr[real])
				default:
					if multi {
						continue
					}
					return Value{}, fmt.Errorf("%s: cannot index a %s; pipe through | json to parse it first", src, cur.Kind())
				}
			}
		}

		nodes = next
		if stepMulti {
			multi = true
		}
	}

	if multi {
		result := make([]any, 0, len(nodes))
		result = append(result, nodes...)
		return Of(result), nil
	}
	if len(nodes) == 0 {
		return Missing(fmt.Sprintf("%s: not found", src)), nil
	}
	return Of(nodes[0]), nil
}

// clampSlice resolves lo/hi against an array of length n using Go/Python
// slice semantics: negative bounds count from the end, an absent bound
// takes the corresponding edge, and the whole range clamps into [0, n]
// rather than erroring — a slice is a range request, not an assertion the
// range is fully populated.
func clampSlice(lo, hi Value, n int) (int, int) {
	l := resolveBound(lo, 0, n)
	h := resolveBound(hi, n, n)
	if l < 0 {
		l = 0
	}
	if h > n {
		h = n
	}
	if l > h {
		l = h
	}
	return l, h
}

func resolveBound(b Value, def, n int) int {
	if b.IsEmpty() {
		return def
	}
	idx, ok := stepIndex(b)
	if !ok {
		return def
	}
	if idx < 0 {
		idx += n
	}
	return idx
}

// stepIndex reports the integer an array-index step or slice bound names —
// a Number, or a String holding exact integer literal text (the shape a
// template constant like the bare 0 in [0] arrives as via reflection — see
// coerce in eval/template.go and text/template's own NumberNode handling).
// Not general JSON-number parsing: "1.5" and "1e9" are deliberately
// rejected — a non-integral index is a hard error, not a valid lookup.
func stepIndex(step Value) (int, bool) {
	if step.Kind() != KindNumber && step.Kind() != KindString {
		return 0, false
	}
	s := step.String()
	if s == "" {
		return 0, false
	}
	i := 0
	if s[0] == '-' {
		i = 1
	}
	if i >= len(s) {
		return 0, false
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// PathCall adapts Path for the accessor rewriter's calling convention:
// args[0] is the source text (a String), args[1] is root, the rest are
// steps.
func PathCall(args []Value) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("hbpath: internal error: expected at least 2 arguments, got %d", len(args))
	}
	return Path(args[0].String(), args[1], args[2:])
}

// StarCall adapts Star to the same []Value-args shape as PathCall/SliceCall
// (see eval/chainvalue.go's pathFuncs) — hbstar takes no arguments.
func StarCall(_ []Value) (Value, error) { return Star(), nil }

// SliceCall adapts Slice the same way — hbslice always takes exactly the
// two bounds the rewriter emits.
func SliceCall(args []Value) (Value, error) {
	if len(args) != 2 {
		return Value{}, fmt.Errorf("hbslice: internal error: expected 2 arguments, got %d", len(args))
	}
	return Slice(args[0], args[1]), nil
}
