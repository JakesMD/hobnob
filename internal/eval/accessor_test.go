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
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"given a var name, when checked, then it is a reference (why: brace-free fields read it as a value)", ".VAR", true},
		{"given a var piped through a filter, when checked, then it is a reference (why: a pipeline still names one value)", ".VAR | first", true},
		{"given a var with accessors, when checked, then it is a reference (why: accessors refine one value)", ".VAR[0].name", true},
		{"given a var name with underscores, when checked, then it is a reference (why: names are upper snake case)", ".RELEASE_LIST", true},
		{"given a relative dir path, when checked, then it is text (why: dir: ./infra must stay a path)", "./infra", false},
		{"given a parent dir path, when checked, then it is text (why: dir: ../tests must stay a path)", "../tests", false},
		{"given a dotfile name, when checked, then it is text (why: .git is a file, not a var)", ".git", false},
		{"given a dotted dotfile name, when checked, then it is text (why: env: .env.local is a path)", ".env.local", false},
		{"given a mixed-case dotfile, when checked, then it is text (why: var names are upper case)", ".DS_Store", false},
		{"given a var name run into lower case, when checked, then it is text (why: .VARfoo names no var)", ".VARfoo", false},
		{"given a pipeline holding template braces, when checked, then it is text (why: braces make it a template)", ".VAR | first {{", false},
		{"given a dangling pipe, when checked, then it is text (why: an empty filter is not a pipeline)", ".VAR |", false},
		{"given empty text, when checked, then it is text (why: nothing to reference)", "", false},
		{"given a lone dot, when checked, then it is text (why: the root context names no var)", ".", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := IsBareRef(tc.in)

			// Assert
			if got != tc.want {
				t.Errorf("IsBareRef(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
