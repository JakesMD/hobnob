package e2e

import "testing"

// eq/ne/lt/le/gt/ge override text/template's builtins (see
// internal/eval/template.go) so a bare .VAR compares by its actual
// value.Value kind instead of always looking like a Struct to reflect.
// These tests drive the comparisons through real hobnob.yml runs — echoed
// into run: output, and gating an if: — rather than calling the funcs
// directly, matching this suite's end-to-end style (see values_test.go).

func TestE2E_Compare_StringVarAgainstStringLiteral(t *testing.T) {
	// given a String var, when compared eq against a matching string
	// literal, then true (why: guard — the original bug this override fixed)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - NAME: ada
		      - run: 'echo {{ eq .NAME "ada" }}'
	`, "t")
	res.OK(t)
	res.Lines(t, "true")
}

func TestE2E_Compare_NumberVarAgainstNumericLiteral(t *testing.T) {
	// given a Number var (from pluck on captured JSON), when compared eq
	// against a template numeric literal, then true (why: a literal 3
	// stringifies via coerce today, so comparing it against a real Number
	// errors "incompatible types for comparison: number, string")
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"count":3}'
		        into:
		          - RESP: stdout
		      - set:
		          - COUNT: '{{ .RESP | pluck "count" }}'
		      - run: 'echo {{ eq .COUNT 3 }}'
	`, "t")
	res.OK(t)
	res.Lines(t, `{"count":3}`, "true")
}

func TestE2E_Compare_NumericOrderingOnCapturedNumber(t *testing.T) {
	// given a Number var, when compared gt against a numeric literal, then
	// true (why: same literal-loses-its-kind bug, ordering variant)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"count":3}'
		        into:
		          - RESP: stdout
		      - set:
		          - COUNT: '{{ .RESP | pluck "count" }}'
		      - run: 'echo {{ gt .COUNT 1 }}'
	`, "t")
	res.OK(t)
	res.Lines(t, `{"count":3}`, "true")
}

func TestE2E_Compare_NumericStringsOrderNumerically(t *testing.T) {
	// given two CLI args that are numeric text (never structured — see
	// GUIDE.md Typed values), when compared lt, then ordering is numeric,
	// not lexical (why: "10" < "9" lexically, but 10 is not less than 9)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: 'echo {{ lt .A .B }}'
	`, "t", "A=9", "B=10")
	res.OK(t)
	res.Lines(t, "true")
}

func TestE2E_Compare_BoolVarAgainstBoolLiteral(t *testing.T) {
	// given a Bool var (from pluck), when compared eq against a bool
	// literal, then true (why: literal true stringifies via coerce today,
	// so comparing it against a real Bool errors)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"active":true}'
		        into:
		          - RESP: stdout
		      - set:
		          - ACTIVE: '{{ .RESP | pluck "active" }}'
		      - run: 'echo {{ eq .ACTIVE true }}'
	`, "t")
	res.OK(t)
	res.Lines(t, `{"active":true}`, "true")
}

func TestE2E_Compare_BoolVarAgainstQuotedText(t *testing.T) {
	// given the same Bool var, when compared eq against the quoted string
	// "true", then still true (why: comparison is lenient on representation
	// — a Bool's String() is "true"/"false", so it matches the text form too)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"active":true}'
		        into:
		          - RESP: stdout
		      - set:
		          - ACTIVE: '{{ .RESP | pluck "active" }}'
		      - run: 'echo {{ eq .ACTIVE "true" }}'
	`, "t")
	res.OK(t)
	res.Lines(t, `{"active":true}`, "true")
}

func TestE2E_Compare_LargeIntegersNotCollapsedByFloat(t *testing.T) {
	// given two distinct integers beyond float64's 2^53 exact-integer range,
	// when compared eq, then false (why: value.Value deliberately holds
	// json.Number, not float64, to avoid precision loss — routing comparison
	// through Float64() would silently collapse these to equal)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"a":9007199254740993,"b":9007199254740992}'
		        into:
		          - RESP: stdout
		      - set:
		          - A: '{{ .RESP | pluck "a" }}'
		          - B: '{{ .RESP | pluck "b" }}'
		      - run: 'echo {{ eq .A .B }}'
	`, "t")
	res.OK(t)
	res.Lines(t, `{"a":9007199254740993,"b":9007199254740992}`, "false")
}

func TestE2E_Compare_MissingVarEqualsEmptyString(t *testing.T) {
	// given a var never set, when compared eq against "", then true (why:
	// IsEmpty() treats a missing var and an explicit empty string alike
	// everywhere else in hobnob — comparison should too, rather than erroring
	// "incompatible types for comparison: nil, string")
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: 'echo {{ eq .MISSING "" }}'
	`, "t")
	res.OK(t)
	res.Lines(t, "true")
}

func TestE2E_Compare_LenResultAgainstNumericLiteral(t *testing.T) {
	// given len's Number result, when compared gt against a numeric literal,
	// then true (why: len is a filter that returns a real Number — the same
	// literal-coercion bug breaks any {{ gt (.X | len) 0 }} guard)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - TAGS: [a, b]
		      - run: 'echo {{ gt (.TAGS | len) 0 }}'
	`, "t")
	res.OK(t)
	res.Lines(t, "true")
}

func TestE2E_Compare_NeMirrorsEq(t *testing.T) {
	// given a String var, when compared ne against a non-matching literal,
	// then true (why: guard — ne is eq's negation, not independently broken)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - NAME: ada
		      - run: 'echo {{ ne .NAME "grace" }}'
	`, "t")
	res.OK(t)
	res.Lines(t, "true")
}

func TestE2E_Compare_BoolOrderingRejected(t *testing.T) {
	// given two real Bool vars (from pluck — a set: scalar literal like
	// "A: true" is a template, not a bare .VAR ref, so it renders as text,
	// not a Bool; see GUIDE.md's "Typed values"), when compared lt, then the
	// run fails naming the unorderable kind (why: bools are equality-only,
	// matching text/template stdlib's own rule — false < true would be an
	// arbitrary invented ordering, not something JSON/shell semantics define)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"a":true,"b":false}'
		        into:
		          - RESP: stdout
		      - set:
		          - A: '{{ .RESP | pluck "a" }}'
		          - B: '{{ .RESP | pluck "b" }}'
		      - run: 'echo {{ lt .A .B }}'
	`, "t")
	res.Fails(t)
	res.Err(t, "bool")
}

func TestE2E_Compare_StructuredValueRejected(t *testing.T) {
	// given two captured JSON objects, when compared eq, then the run fails
	// rather than silently comparing raw captured text (why: a captured
	// value's String() returns its original raw text verbatim — two
	// structurally-identical objects captured with different whitespace
	// would then compare unequal, which is worse than refusing)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"a":1}'
		        into:
		          - X: stdout
		      - run: printf '{"a":1}'
		        into:
		          - Y: stdout
		      - run: 'echo {{ eq .X .Y }}'
	`, "t")
	res.Fails(t)
	res.Err(t, "object")
}

func TestE2E_Compare_YAMLIntLiteralComparesNumerically(t *testing.T) {
	// given a YAML int nested in a set: map literal, when plucked and
	// compared eq against a numeric literal, then true (why: regression —
	// pluck surfaces a Go int, not json.Number; asNumber's raw
	// Any().(string) assertion panics on it, recovered by text/template into
	// "interface conversion: interface {} is int, not string")
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - CFG: { port: 8080 }
		      - set:
		          - PORT: '{{ .CFG | pluck "port" }}'
		      - run: 'echo {{ eq .PORT 8080 }}'
	`, "t")
	res.OK(t)
	res.Lines(t, "true")
}

func TestE2E_Compare_YAMLFloatLiteralOrdersNumerically(t *testing.T) {
	// given a YAML float nested in a set: map literal, when plucked and
	// compared gt against a numeric literal, then true (why: same
	// non-canonical-underlying-type bug as the int case, float variant)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - CFG: { ratio: 1.5 }
		      - set:
		          - RATIO: '{{ .CFG | pluck "ratio" }}'
		      - run: 'echo {{ gt .RATIO 1 }}'
	`, "t")
	res.OK(t)
	res.Lines(t, "true")
}

func TestE2E_Compare_MixedIntFloatKeepsPrecision(t *testing.T) {
	// given a large integer beyond float64's 2^53 exact-integer range
	// compared against a non-integer, when compared gt, then ordering is
	// exact (why: routing both sides through Float64 whenever either side
	// isn't a clean integer collapses 9007199254740993 to …992, giving the
	// wrong answer — see TestE2E_Compare_LargeIntegersNotCollapsedByFloat for
	// the all-integer case this one complements)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"a":9007199254740993,"b":9007199254740992.5}'
		        into:
		          - RESP: stdout
		      - set:
		          - A: '{{ .RESP | pluck "a" }}'
		          - B: '{{ .RESP | pluck "b" }}'
		      - run: 'echo {{ gt .A .B }}'
	`, "t")
	res.OK(t)
	res.Lines(t, `{"a":9007199254740993,"b":9007199254740992.5}`, "true")
}

func TestE2E_Compare_InfAndNaNTextCompareAsText(t *testing.T) {
	// given two CLI args spelling non-JSON numeric text (Inf/Infinity), when
	// compared eq, then they're compared as text, not numerically (why:
	// strconv.ParseFloat accepts "inf"/"Infinity" case-insensitively, so the
	// old gate silently treated these as equal numbers)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: 'echo {{ eq .A .B }}'
	`, "t", "A=inf", "B=Infinity")
	res.OK(t)
	res.Lines(t, "false")
}

func TestE2E_Compare_HexFloatTextComparesAsText(t *testing.T) {
	// given two CLI args, one hex-float text and one plain decimal text of
	// the same numeric value, when compared eq, then they're compared as
	// text, not numerically (why: strconv.ParseFloat accepts Go hex-float
	// syntax, which is not valid JSON number text)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: 'echo {{ eq .A .B }}'
	`, "t", "A=0x10p0", "B=16")
	res.OK(t)
	res.Lines(t, "false")
}

func TestE2E_Compare_LeadingZerosCompareAsText(t *testing.T) {
	// given two CLI args, one with a leading zero, when compared eq, then
	// they're compared as text, not numerically (why: "007" is not valid
	// JSON number text — JSON numbers forbid leading zeros — so the numeric
	// gate must reject it even though strconv/big.Rat would happily parse it)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: 'echo {{ eq .A .B }}'
	`, "t", "A=007", "B=7")
	res.OK(t)
	res.Lines(t, "false")
}

func TestE2E_Compare_LeOrdersNumerically(t *testing.T) {
	// given two numeric-text CLI args, when compared le, then numeric
	// ordering applies (why: le has no dedicated coverage today)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: 'echo {{ le .A .B }}'
	`, "t", "A=9", "B=10")
	res.OK(t)
	res.Lines(t, "true")
}

func TestE2E_Compare_GeOrdersNumerically(t *testing.T) {
	// given two numeric-text CLI args, when compared ge, then numeric
	// ordering applies (why: ge has no dedicated coverage today)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: 'echo {{ ge .A .B }}'
	`, "t", "A=9", "B=10")
	res.OK(t)
	res.Lines(t, "false")
}

func TestE2E_Compare_EqWithSingleArgumentErrors(t *testing.T) {
	// given eq called with only its first argument, when evaluated, then the
	// run fails naming the missing argument (why: stdlib's own eq errors
	// "missing argument for comparison" on this shape; templateEq's variadic
	// loop over zero args silently returned false instead)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - X: ada
		      - run: 'echo {{ eq .X }}'
	`, "t")
	res.Fails(t)
	res.Err(t, "argument")
}

func TestE2E_Compare_IfConditionGatesOnNumericComparison(t *testing.T) {
	// given a Number var, when a step's if: renders {{ gt .COUNT 1 }} into a
	// shell condition, then the step runs only when the comparison holds
	// (why: if:'s rendered true/false is executed as a shell builtin — see
	// hobnob.yml's own {{eq .OS "darwin"}} usage — so the template-level fix
	// must also hold end-to-end through a real if: gate)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"count":3}'
		        into:
		          - RESP: stdout
		      - set:
		          - COUNT: '{{ .RESP | pluck "count" }}'
		      - run: echo gated
		        if: '{{ gt .COUNT 1 }}'
		      - run: echo skipped
		        if: '{{ lt .COUNT 1 }}'
	`, "t")
	res.OK(t)
	res.Lines(t, `{"count":3}`, "gated")
}
