package ask

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hobnob/internal/value"
)

// tree makes a temp directory holding the given entries; a name ending in "/"
// is a directory, anything else an empty file.
func tree(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		full := filepath.Join(root, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func notTxt(abs string, isDir bool) bool { return isDir || strings.HasSuffix(abs, ".txt") }

func TestPathQuestion_JudgesEachPickedPath(t *testing.T) {
	// given a path question, when a path is picked, then it is made absolute and checked for existence, kind, junk and match:, each refusal naming the path
	// (why: the terminal picker, the page and a preset answer all share one set of path rules)

	// Arrange
	root := tree(t, "a.txt", "b.md", "sub/", "sub/c.txt", ".DS_Store", "home/docs/")
	home := filepath.Join(root, "home")
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	match := func(abs string, isDir bool) bool { return isDir || strings.HasSuffix(abs, ".txt") }
	tests := []struct {
		name    string
		rules   PathRules
		pick    string
		want    string
		refusal string
	}{
		{"absolute", PathRules{}, at("a.txt"), at("a.txt"), ""},
		{"relative to the invoking directory", PathRules{}, "sub/c.txt", at("sub", "c.txt"), ""},
		{"cleaned", PathRules{}, at("sub", "..", "a.txt"), at("a.txt"), ""},
		{"home", PathRules{Dir: true}, "~/docs", at("home", "docs"), ""},
		{"home alone", PathRules{Dir: true}, "~", home, ""},
		{"home with the OS separator", PathRules{Dir: true}, "~" + string(filepath.Separator) + "docs", at("home", "docs"), ""},
		{"missing", PathRules{}, "nope.txt", "", at("nope.txt") + " doesn't exist"},
		{"a directory for a file", PathRules{}, "sub", "", at("sub") + " is a directory, not a file"},
		{"a file for a directory", PathRules{Dir: true}, "a.txt", "", at("a.txt") + " is a file, not a directory"},
		{"junk", PathRules{}, ".DS_Store", "", at(".DS_Store") + " is a system file name"},
		{"match miss", PathRules{Included: match, Match: "*.txt"}, "b.md", "", at("b.md") + " doesn't match *.txt"},
		{"match hit", PathRules{Included: match, Match: "*.txt"}, "a.txt", at("a.txt"), ""},
		{"excluded", PathRules{Excluded: func(string, bool) bool { return true }, Match: "!*"}, "a.txt", "", at("a.txt") + " doesn't match !*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := tt.rules
			rules.Invoke, rules.WorkDir, rules.Home = root, root, home
			q := Question{Path: &rules}

			// Act
			verdict := q.Judge(Answer{Picks: []string{tt.pick}})

			// Assert
			if verdict.Refusal != tt.refusal {
				t.Fatalf("refusal = %q, want %q", verdict.Refusal, tt.refusal)
			}
			if tt.refusal == "" && verdict.Value.String() != tt.want {
				t.Errorf("value = %q, want %q", verdict.Value.String(), tt.want)
			}
		})
	}
}

func TestPathQuestion_ShapesTheAnswerLikeAMenu(t *testing.T) {
	// given path questions, when they are answered with none, one or several picks, then required refuses none, optional skips, a single refuses several and multi lists each once
	// (why: a picker answers like text and menus do, so no front end re-derives it)

	// Arrange
	root := tree(t, "a.txt", "b.txt")
	a, b := filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")
	rules := PathRules{Invoke: root, WorkDir: root}

	// Act
	required := Question{Path: &rules}.Judge(Answer{})
	optional := Question{Path: &rules, Optional: true}.Judge(Answer{})
	optionalMulti := Question{Path: &rules, Optional: true, Multi: true}.Judge(Answer{})
	several := Question{Path: &rules}.Judge(Answer{Picks: []string{a, b}})
	multi := Question{Path: &rules, Multi: true}.Judge(Answer{Picks: []string{a, b, "a.txt"}})
	multiNone := Question{Path: &rules, Multi: true}.Judge(Answer{})

	// Assert
	if required.Refusal != "an answer is required" {
		t.Errorf("required: %+v", required)
	}
	if !optional.Skipped || optional.Value.String() != "" {
		t.Errorf("optional: %+v", optional)
	}
	if !optionalMulti.Skipped || !emptyList(optionalMulti.Value) {
		t.Errorf("optional multi: %+v", optionalMulti)
	}
	if several.Refusal == "" {
		t.Errorf("several for one: %+v", several)
	}
	if multi.Refusal != "" || multi.Value.String() != value.Of([]any{a, b}).String() {
		t.Errorf("multi: %+v", multi)
	}
	if multiNone.Refusal != "an answer is required" {
		t.Errorf("multi none: %+v", multiNone)
	}
}

func emptyList(v value.Value) bool {
	items, ok := v.Any().([]any)
	return ok && len(items) == 0
}

func TestPathQuestion_RefusesAFailingCheckOnTheResolvedAnswer(t *testing.T) {
	// given a path question with a check, when the check fails or errors, then the picker is refused with its wording and sees the absolute path
	// (why: a failed check: is a refusal inside the picker, not a reopen)

	// Arrange
	root := tree(t, "a.txt")
	var seen string
	rules := PathRules{Invoke: root, WorkDir: root}
	failing := Question{Path: &rules, CheckDesc: "ok", Check: func(v value.Value) (bool, error) { seen = v.String(); return false, nil }}

	// Act
	verdict := failing.Judge(Answer{Picks: []string{"a.txt"}})

	// Assert
	if verdict.Refusal != "validation failed: ok" || seen != filepath.Join(root, "a.txt") {
		t.Errorf("verdict %+v, check saw %q", verdict, seen)
	}
}

func TestPathRules_OpeningStartsAtTheDefaultAndPicksWhatFits(t *testing.T) {
	// given defaults, when the picker opens, then it opens in the first default's folder (only a directory given to a file picker is the folder itself), picks the defaults that fit and are allowed, and otherwise opens in the nearest existing folder or the work directory
	// (why: terminal and page open the same way)

	// Arrange
	root := tree(t, "sub/a.txt", "sub/b.md", "other/", "sub/.DS_Store")
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	tests := []struct {
		name      string
		rules     PathRules
		multi     bool
		wantOpen  string
		wantPicks []string
	}{
		{"no default", PathRules{}, false, root, nil},
		{"file default", PathRules{Defaults: []string{"sub/a.txt"}}, false, at("sub"), []string{at("sub", "a.txt")}},
		{"directory default for a file picker", PathRules{Defaults: []string{"sub"}}, false, at("sub"), nil},
		{"directory default for a dir picker", PathRules{Dir: true, Defaults: []string{"sub"}}, false, root, []string{at("sub")}},
		{"missing default", PathRules{Defaults: []string{"sub/deep/x.txt"}}, false, at("sub"), nil},
		{"junk default is not picked", PathRules{Defaults: []string{"sub/.DS_Store"}}, false, at("sub"), nil},
		{"filtered default is not picked", PathRules{Defaults: []string{"sub/b.md"}, Included: notTxt}, false, at("sub"), nil},
		{"one pick without multi", PathRules{Defaults: []string{"sub/a.txt", "sub/b.md"}}, false, at("sub"), []string{at("sub", "a.txt")}},
		{"several under multi", PathRules{Defaults: []string{"sub/a.txt", "sub/b.md"}}, true, at("sub"), []string{at("sub", "a.txt"), at("sub", "b.md")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := tt.rules
			rules.Invoke, rules.WorkDir = root, root

			// Act
			open, picks := rules.Opening(tt.multi)

			// Assert
			if open != tt.wantOpen || strings.Join(picks, "|") != strings.Join(tt.wantPicks, "|") {
				t.Errorf("Opening() = %q, %q; want %q, %q", open, picks, tt.wantOpen, tt.wantPicks)
			}
		})
	}
}

func TestPathRules_ListShowsWhatCanBeBrowsed(t *testing.T) {
	// given a folder with junk, hidden, excluded and non-matching entries, when it is listed, then junk and excluded entries and (in a file picker) non-matching files are left out, directories come first, and a non-matching directory can be opened but not picked
	// (why: both front ends list the same entries)

	// Arrange
	root := tree(t, ".env", ".DS_Store", "b.md", "A.txt", "sub/", "skip/", "._x")
	rules := PathRules{Included: notTxt, Excluded: func(abs string, _ bool) bool { return filepath.Base(abs) == "skip" }}
	dirRules := PathRules{Dir: true, Included: func(abs string, _ bool) bool { return filepath.Base(abs) != "sub" }}

	// Act
	files := rules.List(root)
	dirs := dirRules.List(root)

	// Assert
	var got []string
	for _, e := range files {
		got = append(got, e.Name)
	}
	if strings.Join(got, ",") != "sub,A.txt" {
		t.Errorf("files = %v", got)
	}
	var open []string
	for _, e := range dirs {
		open = append(open, e.Name+map[bool]string{true: "+", false: "-"}[e.Pickable])
	}
	if strings.Join(open, ",") != "skip+,sub-" {
		t.Errorf("dirs = %v", open)
	}
}

func TestPathRules_DefaultTargetIsTheFirstDefaultOfTheKindWhetherOrNotMatchAllowsIt(t *testing.T) {
	// given defaults, when the picker looks for what to point at, then it is the first default if it exists and is of the picker's kind, even when match: filters it out
	// (why: the person sees where the default is)

	// Arrange
	root := tree(t, "sub/a.txt", "sub/b.md")
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	tests := []struct {
		name  string
		rules PathRules
		want  string
	}{
		{"no default", PathRules{}, ""},
		{"file default", PathRules{Defaults: []string{"sub/a.txt"}}, at("sub", "a.txt")},
		{"filtered file default", PathRules{Defaults: []string{"sub/b.md"}, Included: notTxt}, at("sub", "b.md")},
		{"filtered dir default", PathRules{Dir: true, Defaults: []string{"sub"}, Included: func(string, bool) bool { return false }}, at("sub")},
		{"wrong kind", PathRules{Dir: true, Defaults: []string{"sub/a.txt"}}, ""},
		{"missing", PathRules{Defaults: []string{"sub/none.txt"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := tt.rules
			rules.Invoke = root

			// Act
			got := rules.DefaultTarget()

			// Assert
			if got != tt.want {
				t.Errorf("DefaultTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}
