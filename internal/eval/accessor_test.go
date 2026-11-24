package eval

import (
	"strings"
	"testing"
)

func TestRewriteAccessors_BareVarUnchanged(t *testing.T) {
	// given a template with a bare var reference (zero steps), when
	// rewritten, then it is untouched (why: a top-level bare .VAR keeps
	// text/template's own handling)
	// Arrange
	src := "{{ .VAR }}"

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	if got != src {
		t.Fatalf("got %q, want unchanged %q", got, src)
	}
}

func TestRewriteAccessors_DottedField(t *testing.T) {
	// given a dotted field path, when rewritten, then it becomes an hbpath
	// call with one key per step (why: the original path text is kept for
	// error messages)
	// Arrange
	src := "{{ .RESP.profile.name }}"

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ (hbpath ".RESP.profile.name" .RESP "profile" "name") }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_IndexAndDynamicKey(t *testing.T) {
	// given a dynamic key and an index, when rewritten, then the key stays a
	// var reference and the index a number (why: the key is resolved at
	// render time, never parsed as a path)
	// Arrange
	src := "{{ .JSON[.MY_VAR][0] }}"

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ (hbpath ".JSON[.MY_VAR][0]" .JSON .MY_VAR 0) }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_StarAndSlice(t *testing.T) {
	// given wildcard, slice and negative-index accessors, when rewritten,
	// then each becomes its hbstar, hbslice or number step (why: Path needs
	// to tell the step kinds apart)
	// Arrange
	tests := map[string]string{
		"{{ .ITEMS[*].name }}": `{{ (hbpath ".ITEMS[*].name" .ITEMS hbstar "name") }}`,
		"{{ .ITEMS[1:3] }}":    `{{ (hbpath ".ITEMS[1:3]" .ITEMS (hbslice 1 3)) }}`,
		"{{ .ITEMS[:3] }}":     `{{ (hbpath ".ITEMS[:3]" .ITEMS (hbslice "" 3)) }}`,
		"{{ .ITEMS[-1] }}":     `{{ (hbpath ".ITEMS[-1]" .ITEMS -1) }}`,
	}
	for src, want := range tests {
		// Act
		got, err := rewriteAccessors(src)

		// Assert
		if err != nil {
			t.Fatalf("rewriteAccessors(%q): %v", src, err)
		}
		if got != want {
			t.Fatalf("rewriteAccessors(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestRewriteAccessors_QuotedKey(t *testing.T) {
	// given a quoted key containing dots and slashes, when rewritten, then it
	// becomes a single key step (why: keys like Kubernetes labels must not
	// split into a nested path)
	// Arrange
	src := `{{ .A["app.kubernetes.io/name"] }}`

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ (hbpath ".A[\"app.kubernetes.io/name\"]" .A "app.kubernetes.io/name") }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_ParenHead(t *testing.T) {
	// given an accessor on a parenthesized pipeline, when rewritten, then the
	// pipeline becomes the path root (why: (.TAGS | json)[0] indexes the
	// parsed result)
	// Arrange
	src := `{{ (.TAGS | json)[0] }}`

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ (hbpath "(.TAGS | json)[0]" (.TAGS | json) 0) }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_FilterChainAfterAccessor(t *testing.T) {
	// given an accessor followed by a filter, when rewritten, then only the
	// accessor is wrapped (why: the filter applies to the selected value)
	// Arrange
	src := "{{ .A.b | trim }}"

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ (hbpath ".A.b" .A "b") | trim }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_NoRewriteInsideStringLiteral(t *testing.T) {
	// given a string literal that looks like an accessor, when rewritten,
	// then it is untouched (why: string contents are data, not paths)
	// Arrange
	src := `{{ printf "%s" "a.b[0]" }}`

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	if got != src {
		t.Fatalf("got %q, want unchanged %q", got, src)
	}
}

func TestRewriteAccessors_NumberLiteralNotTreatedAsField(t *testing.T) {
	// given a decimal number literal, when rewritten, then it is untouched
	// (why: 1.5 must not read as a field .5 on 1)
	// Arrange
	src := "{{ 1.5 }}"

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	if got != src {
		t.Fatalf("got %q, want unchanged %q", got, src)
	}
}

func TestRewriteAccessors_RangeBody(t *testing.T) {
	// given a range over a bare var with a bare field in its body, when
	// rewritten, then it is untouched (why: both are zero-step references,
	// which text/template handles itself)
	// Arrange
	src := "{{ range .L }}{{ .name }}{{ end }}"

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	if got != src {
		t.Fatalf("got %q, want unchanged %q", got, src)
	}
}

func TestRewriteAccessors_IfBody(t *testing.T) {
	// given an accessor as an if: condition, when rewritten, then it is
	// wrapped like anywhere else (why: accessors work in every action, not
	// just output)
	// Arrange
	src := "{{ if .A.b }}yes{{ end }}"

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	want := `{{ if (hbpath ".A.b" .A "b") }}yes{{ end }}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRewriteAccessors_TextOutsideActionsUntouched(t *testing.T) {
	// given accessor-like text outside {{ }}, when rewritten, then that text
	// is untouched (why: only template actions hold paths)
	// Arrange
	src := "prefix .A.b {{ .X.y }} suffix .C.d"

	// Act
	got, err := rewriteAccessors(src)

	// Assert
	if err != nil {
		t.Fatalf("rewriteAccessors: %v", err)
	}
	if !strings.HasPrefix(got, "prefix .A.b ") || !strings.HasSuffix(got, " suffix .C.d") {
		t.Fatalf("got %q, expected text outside {{ }} untouched", got)
	}
}

func TestRewriteAccessors_UnterminatedActionErrors(t *testing.T) {
	// given an unterminated action, when rewritten, then it errors (why: a malformed
	// template must fail here, not render half-rewritten)
	// Arrange
	src := "{{ .A.b"

	// Act
	_, err := rewriteAccessors(src)

	// Assert
	if err == nil {
		t.Fatal("expected an error for an unterminated action")
	}
}

func TestRewriteAccessors_UnterminatedBracketErrors(t *testing.T) {
	// given an unterminated bracket, when rewritten, then it errors (why: a malformed
	// template must fail here, not render half-rewritten)
	// Arrange
	src := "{{ .A[0 }}"

	// Act
	_, err := rewriteAccessors(src)

	// Assert
	if err == nil {
		t.Fatal("expected an error for an unterminated bracket")
	}
}

func TestIsBareRef(t *testing.T) {
	// given var references and look-alike paths, when checked, then only real
	// upper-case var references count (why: brace-free fields must tell .VAR
	// from paths such as ./infra or .env.local)
	// Arrange
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
