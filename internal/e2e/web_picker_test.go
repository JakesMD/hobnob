package e2e

import (
	"net/http"
	"path/filepath"
	"testing"

	"hobnob/internal/web"
)

// lastColumn is the open folder's listing, the picker's final column.
func lastColumn(p *web.Prompt) web.Column {
	return p.Picker.Columns[len(p.Picker.Columns)-1]
}

func entryNames(column web.Column) []string {
	names := make([]string, len(column.Entries))
	for i, entry := range column.Entries {
		names[i] = entry.Name
	}
	return names
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestE2E_Web_PickerStartsAtTheDefaultFolderAndHidesWhatMatchExcludes(t *testing.T) {
	// given a file get: whose default: is a folder and whose match: allows
	// only .txt and excludes secret.txt, when the page opens it, then the
	// snapshot opens that folder and lists the folders and the allowed files
	// only (why: the same Included/Excluded predicates as the terminal picker,
	// which hides, not greys out)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              default: sub
		              match: ["*.txt", "!secret.txt"]
		      - run: echo conf={{.CONF}}
	`
	files := Files{"hobnob.yml": yml, "sub/f.txt": "x", "sub/g.md": "x", "sub/secret.txt": "x", "sub/inner/h.txt": "x"}
	var asked web.Snapshot
	c := Case{Files: files, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked = tab.Screen(web.ScreenPrompt)
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": filepath.Join(b.dir, "sub", "f.txt")})
		tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	p := asked.Prompt
	if p.Kind != web.KindPath || p.Picker.Type != "file" || p.Picker.Open != filepath.Join(res.Dir, "sub") {
		t.Errorf("kind = %s, type = %s, open = %s", p.Kind, p.Picker.Type, p.Picker.Open)
	}
	if got := entryNames(lastColumn(p)); !equalStrings(got, []string{"inner", "f.txt"}) {
		t.Errorf("listing = %v", got)
	}
	res.Out(t, "conf="+filepath.Join(res.Dir, "sub", "f.txt"))
}

func TestE2E_Web_OpeningAFolderByPostChangesTheListing(t *testing.T) {
	// given a file picker open at the working folder, when the page opens a
	// subfolder, then the next snapshot has a column more, listing that
	// folder, and a path that isn't a folder is refused (why: the page
	// browses through the server, which filters)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		      - run: echo conf={{.CONF}}
	`
	files := Files{"hobnob.yml": yml, "sub/f.txt": "x", "sub/deep/g.txt": "x"}
	var before, after web.Snapshot
	var refused int
	c := Case{Files: files, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		before = tab.Screen(web.ScreenPrompt)
		id := before.Prompt.ID
		refused = b.Post("open", map[string]any{"id": id, "path": filepath.Join(b.dir, "sub", "f.txt")})
		if status := b.Post("open", map[string]any{"id": id, "path": filepath.Join(b.dir, "sub")}); status != http.StatusNoContent {
			b.Fail("open status = %d", status)
		}
		after = tab.Until(func(s web.Snapshot) bool { return s.Prompt != nil && s.Prompt.Picker.Open == filepath.Join(b.dir, "sub") })
		b.Post("answer", map[string]any{"id": id, "value": filepath.Join(b.dir, "sub", "f.txt")})
		tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if refused != http.StatusBadRequest {
		t.Errorf("opening a file: status = %d", refused)
	}
	if len(after.Prompt.Picker.Columns) != len(before.Prompt.Picker.Columns)+1 {
		t.Errorf("columns: %d before, %d after", len(before.Prompt.Picker.Columns), len(after.Prompt.Picker.Columns))
	}
	if got := entryNames(lastColumn(after.Prompt)); !equalStrings(got, []string{"deep", "f.txt"}) {
		t.Errorf("listing = %v", got)
	}
}

func TestE2E_Web_FolderPickerStartsWithItsDefaultSelectedAndAnswersAtOnce(t *testing.T) {
	// given a dir get: with a folder default:, when the page answers with
	// the folder the snapshot started with, then the run gets it (why: a
	// folder picker's answer starts as its default)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - OUT:
		              type: dir
		              default: build
		      - run: echo out={{.OUT}}
	`
	files := Files{"hobnob.yml": yml, "build/a.txt": "x", "other/b.txt": "x"}
	var asked web.Snapshot
	c := Case{Files: files, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked = tab.Screen(web.ScreenPrompt)
		if len(asked.Prompt.Picker.Selected) != 1 {
			b.Fail("selected = %v", asked.Prompt.Picker.Selected)
		}
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": asked.Prompt.Picker.Selected[0]})
		tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	want := filepath.Join(res.Dir, "build")
	if got := asked.Prompt.Picker.Selected; !equalStrings(got, []string{want}) || asked.Prompt.Picker.Open != want {
		t.Errorf("selected = %v, open = %s", got, asked.Prompt.Picker.Open)
	}
	res.Out(t, "out="+want)
}

func TestE2E_Web_MultiPickerAnswersWithSeveralPathsAndRefusesBadOnes(t *testing.T) {
	// given a multi: file get: with match:, when the page answers with a file
	// match: excludes and then with two good files, then the first is refused
	// with a reason and the second becomes the list (why: the same rules as
	// the terminal, several picks)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - FILES:
		              type: file
		              multi: true
		              default: sub
		              match: "*.txt"
		      - run: echo files={{.FILES | join ","}}
	`
	files := Files{"hobnob.yml": yml, "sub/a.txt": "x", "sub/b.txt": "x", "sub/c.md": "x"}
	var refusal string
	var done web.Snapshot
	c := Case{Files: files, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked := tab.Screen(web.ScreenPrompt)
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": []string{filepath.Join(b.dir, "sub", "c.md")}})
		refusal = tab.Until(func(s web.Snapshot) bool { return s.Prompt != nil && s.Prompt.Refusal != "" }).Prompt.Refusal
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": []string{filepath.Join(b.dir, "sub", "a.txt"), filepath.Join(b.dir, "sub", "b.txt")}})
		done = tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "files="+filepath.Join(res.Dir, "sub", "a.txt")+","+filepath.Join(res.Dir, "sub", "b.txt"))
	if refusal == "" {
		t.Error("the excluded file was accepted")
	}
	if len(done.Rail.Answers) != 1 {
		t.Errorf("answers = %+v", done.Rail.Answers)
	}
}

func TestE2E_Web_OptionalPickerCanBeSkippedAndARequiredOneRefusesAnEmptyAnswer(t *testing.T) {
	// given an optional file get: and a required one, when the page skips the
	// first and answers the second with nothing, then the run goes on without
	// the first and the second is refused (why: required and optional work as
	// they do in the text prompt)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - A:
		              type: file
		              optional: true
		          - B:
		              type: file
		      - run: echo a=[{{.A}}] b={{.B}}
	`
	files := Files{"hobnob.yml": yml, "f.txt": "x"}
	var refusal string
	c := Case{Files: files, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		first := tab.Screen(web.ScreenPrompt)
		b.Post("skip", map[string]any{"id": first.Prompt.ID})
		second := tab.Until(func(s web.Snapshot) bool { return s.Prompt != nil && s.Prompt.VarName == "B" })
		b.Post("answer", map[string]any{"id": second.Prompt.ID, "value": ""})
		refusal = tab.Until(func(s web.Snapshot) bool { return s.Prompt != nil && s.Prompt.Refusal != "" }).Prompt.Refusal
		b.Post("answer", map[string]any{"id": second.Prompt.ID, "value": filepath.Join(b.dir, "f.txt")})
		tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "a=[] b="+filepath.Join(res.Dir, "f.txt"))
	if refusal == "" {
		t.Error("an empty answer to a required picker was accepted")
	}
}
