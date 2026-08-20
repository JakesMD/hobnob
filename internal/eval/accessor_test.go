package eval

import (
	"strings"
	"testing"
)

func TestRewriteAccessors_BareVarUnchanged(t *testing.T) {
	// given a template with a bare var reference (zero steps)
	src := "{{ .VAR }}"
	// when rewritten
	got, err := rewriteAccessors(src)
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	// then it is untouched -- a top-level bare .VAR is never rewritten
	if got != src {
		t.Fatalf("got %q, want unchanged %q", got, src)
	}
}

func TestRewriteAccessors_DottedField(t *testing.T) {
	got, err := rewriteAccessors("{{ .RESP.profile.name }}")
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ (hbpath ".RESP.profile.name" .RESP "profile" "name") }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_IndexAndDynamicKey(t *testing.T) {
	got, err := rewriteAccessors("{{ .JSON[.MY_VAR][0] }}")
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ (hbpath ".JSON[.MY_VAR][0]" .JSON .MY_VAR 0) }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_StarAndSlice(t *testing.T) {
	tests := map[string]string{
		"{{ .ITEMS[*].name }}": `{{ (hbpath ".ITEMS[*].name" .ITEMS hbstar "name") }}`,
		"{{ .ITEMS[1:3] }}":    `{{ (hbpath ".ITEMS[1:3]" .ITEMS (hbslice 1 3)) }}`,
		"{{ .ITEMS[:3] }}":     `{{ (hbpath ".ITEMS[:3]" .ITEMS (hbslice "" 3)) }}`,
		"{{ .ITEMS[-1] }}":     `{{ (hbpath ".ITEMS[-1]" .ITEMS -1) }}`,
	}
	for src, want := range tests {
		got, err := rewriteAccessors(src)
		if err != nil {
			t.Fatalf("rewriteAccessors(%q): %v", src, err)
		}
		if got != want {
			t.Fatalf("rewriteAccessors(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestRewriteAccessors_QuotedKey(t *testing.T) {
	got, err := rewriteAccessors(`{{ .A["app.kubernetes.io/name"] }}`)
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ (hbpath ".A[\"app.kubernetes.io/name\"]" .A "app.kubernetes.io/name") }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_ParenHead(t *testing.T) {
	got, err := rewriteAccessors(`{{ (.TAGS | json)[0] }}`)
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ (hbpath "(.TAGS | json)[0]" (.TAGS | json) 0) }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_FilterChainAfterAccessor(t *testing.T) {
	got, err := rewriteAccessors("{{ .A.b | trim }}")
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ (hbpath ".A.b" .A "b") | trim }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_NoRewriteInsideStringLiteral(t *testing.T) {
	src := `{{ printf "%s" "a.b[0]" }}`
	got, err := rewriteAccessors(src)
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	if got != src {
		t.Fatalf("got %q, want unchanged %q", got, src)
	}
}

func TestRewriteAccessors_NumberLiteralNotTreatedAsField(t *testing.T) {
	src := "{{ 1.5 }}"
	got, err := rewriteAccessors(src)
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	if got != src {
		t.Fatalf("got %q, want unchanged %q", got, src)
	}
}

func TestRewriteAccessors_RangeBody(t *testing.T) {
	src := "{{ range .L }}{{ .name }}{{ end }}"
	got, err := rewriteAccessors(src)
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	// .L and .name are both bare (zero steps) -- unchanged.
	if got != src {
		t.Fatalf("got %q, want unchanged %q", got, src)
	}
}

func TestRewriteAccessors_IfBody(t *testing.T) {
	got, err := rewriteAccessors("{{ if .A.b }}yes{{ end }}")
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ if (hbpath ".A.b" .A "b") }}yes{{ end }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_TextOutsideActionsUntouched(t *testing.T) {
	src := "prefix .A.b {{ .X.y }} suffix .C.d"
	got, err := rewriteAccessors(src)
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	if !strings.HasPrefix(got, "prefix .A.b ") || !strings.HasSuffix(got, " suffix .C.d") {
		t.Fatalf("got %q, expected text outside {{ }} untouched", got)
	}
}

func TestRewriteAccessors_UnterminatedActionErrors(t *testing.T) {
	if _, err := rewriteAccessors("{{ .A.b"); err == nil {
		t.Fatal("expected an error for an unterminated action")
	}
}

func TestRewriteAccessors_UnterminatedBracketErrors(t *testing.T) {
	if _, err := rewriteAccessors("{{ .A[0 }}"); err == nil {
		t.Fatal("expected an error for an unterminated bracket")
	}
}

func TestIsBareRef(t *testing.T) {
	tests := map[string]bool{
		".VAR":                true,
		".VAR | first":        true,
		".VAR[0].name":        true,
		".RELEASE_LIST":       true,
		"./infra":             false,
		"../tests":            false,
		".git":                false,
		".env.local":          false,
		".DS_Store":           false,
		".VARfoo":             false,
		".VAR | first {{":     false,
		".VAR |":              false,
		"":                    false,
		".":                   false,
	}
	for in, want := range tests {
		if got := IsBareRef(in); got != want {
			t.Errorf("IsBareRef(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSplitSourceAccessor(t *testing.T) {
	tests := map[string][2]string{
		"stdout":          {"stdout", ""},
		"stdout[0].name":  {"stdout", "[0].name"},
		"stdout.name":     {"stdout", ".name"},
		"stderr":          {"stderr", ""},
	}
	for in, want := range tests {
		name, accessor := SplitSourceAccessor(in)
		if name != want[0] || accessor != want[1] {
			t.Errorf("SplitSourceAccessor(%q) = (%q, %q), want (%q, %q)", in, name, accessor, want[0], want[1])
		}
	}
}
