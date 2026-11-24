package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func newTestPath(root, kind string) pathModel {
	return newPathModel(PathPrompt{Type: kind, VarName: "CFG", Invoke: root})
}

func press(m pathModel, keys ...tea.KeyMsg) pathModel {
	for _, key := range keys {
		next, _ := m.Update(key)
		m = next.(pathModel)
	}
	return m
}

func typed(m pathModel, text string) pathModel {
	for _, r := range text {
		m = press(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

var (
	keyUp    = tea.KeyMsg{Type: tea.KeyUp}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyLeft  = tea.KeyMsg{Type: tea.KeyLeft}
	keyRight = tea.KeyMsg{Type: tea.KeyRight}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyTab   = tea.KeyMsg{Type: tea.KeyTab}
	keyBack  = tea.KeyMsg{Type: tea.KeyBackspace}
)

func names(m pathModel) []string {
	var out []string
	for _, r := range m.rows() {
		out = append(out, r.label())
	}
	return out
}

func equal(a, b []string) bool {
	return strings.Join(a, "|") == strings.Join(b, "|")
}

func TestPathModel_FilePicker_ListsDirectoriesFirstThenByName(t *testing.T) {
	// given a directory with files and subdirectories, when a file picker opens, then directories come first and each group is sorted by name
	// (why: directories are the way to navigate, so they lead the list)

	// Arrange
	root := tree(t, "b.txt", "a.txt", "zed/", "alpha/")

	// Act
	got := names(newTestPath(root, "file"))

	// Assert
	want := []string{"alpha/", "zed/", "a.txt", "b.txt"}
	if !equal(got, want) {
		t.Errorf("rows: got %v, want %v", got, want)
	}
}

func TestPathModel_DirPicker_ListsDirectoriesOnlyWithThisDirectoryRow(t *testing.T) {
	// given a directory with files and subdirectories, when a dir picker opens with an empty segment, then only directories list under a "./ (this directory)" row
	// (why: a dir picker must be able to pick the directory it is in)

	// Arrange
	root := tree(t, "a.txt", "sub/")

	// Act
	got := names(newTestPath(root, "dir"))

	// Assert
	want := []string{"./ (this directory)", "sub/"}
	if !equal(got, want) {
		t.Errorf("rows: got %v, want %v", got, want)
	}
}

func TestPathModel_DirPicker_ThisDirectoryRowGoneOnceTyping(t *testing.T) {
	// given a dir picker, when a segment is typed, then the "./ (this directory)" row is gone
	// (why: the row only stands for an empty segment)

	// Arrange
	root := tree(t, "sub/")

	// Act
	got := names(typed(newTestPath(root, "dir"), "s"))

	// Assert
	want := []string{"sub/"}
	if !equal(got, want) {
		t.Errorf("rows: got %v, want %v", got, want)
	}
}

func TestPathModel_ListsDotfilesButNeverJunkNames(t *testing.T) {
	// given dotfiles and OS junk names, when a file picker opens, then dotfiles list and junk names do not
	// (why: hidden entries are shown, but the fixed junk list never is)

	// Arrange
	root := tree(t, ".env", ".git/", ".DS_Store", "._shadow", "Thumbs.db", "desktop.ini", "$RECYCLE.BIN/", "System Volume Information/", ".Trashes/", "real.txt")

	// Act
	got := names(newTestPath(root, "file"))

	// Assert
	want := []string{".git/", ".env", "real.txt"}
	if !equal(got, want) {
		t.Errorf("rows: got %v, want %v", got, want)
	}
}

func TestPathModel_PredicateDecidesWhatTheListingShows(t *testing.T) {
	// given a caller predicate hiding .log files, when a file picker opens, then those files are not listed but directories still are
	// (why: tui takes the filter from its caller instead of knowing about match:)

	// Arrange
	root := tree(t, "a.log", "b.txt", "sub/")
	m := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Show: func(path string, isDir bool) bool {
		return isDir || !strings.HasSuffix(path, ".log")
	}})

	// Act
	got := names(m)

	// Assert
	want := []string{"sub/", "b.txt"}
	if !equal(got, want) {
		t.Errorf("rows: got %v, want %v", got, want)
	}
}

func TestPathModel_FuzzyMatchesSegmentBestFirstAndHighlights(t *testing.T) {
	// given entries, when a segment is typed, then only fuzzy matches list, the best first
	// (why: only the segment is matched, against this directory's entries)

	// Arrange
	root := tree(t, "runner/", "xrxnxr.txt", "other.txt", "sub/runner_inner.txt")

	// Act
	got := names(typed(newTestPath(root, "file"), "rnr"))

	// Assert
	want := []string{"runner/", "xrxnxr.txt"}
	if !equal(got, want) {
		t.Errorf("rows: got %v, want %v (why: never recursive, best match first)", got, want)
	}
}

func TestPathModel_View_ShowsHeaderInfoMarkersAndHint(t *testing.T) {
	// given a file picker with info, when viewed, then it shows the menu-style header, the info, the cursor marker and the hint line
	// (why: the picker must look like the other prompts)

	// Arrange
	root := tree(t, "a.txt")
	m := newPathModel(PathPrompt{Type: "file", VarName: "CFG", Info: "Which config?", Invoke: root})

	// Act
	view := m.View()

	// Assert
	for _, want := range []string{"Pick a file for", "CFG", "Which config?", "▶ ", "a.txt", "↑↓ move", "enter pick"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(strings.ToLower(view), "folder") {
		t.Errorf("view says folder, want directory:\n%s", view)
	}
}

func TestPathModel_View_DirHeaderSaysDirectory(t *testing.T) {
	// given a dir picker, when viewed, then the header reads "Pick a directory for"
	// (why: user-facing text says directory, never folder)

	// Arrange
	m := newTestPath(tree(t), "dir")

	// Act
	view := m.View()

	// Assert
	if !strings.Contains(view, "Pick a directory for") {
		t.Errorf("view missing directory header:\n%s", view)
	}
}

func TestPathModel_SlashWithExactMatch_StepsIntoThatDirectory(t *testing.T) {
	// given a segment naming a directory exactly, when / is typed, then the picker steps into it
	// (why: an exact name wins over fuzzy ranking)

	// Arrange
	root := tree(t, "runner/inner.txt", "run/other.txt")

	// Act
	m := typed(newTestPath(root, "file"), "run/")

	// Assert
	if got := names(m); !equal(got, []string{"other.txt"}) {
		t.Errorf("rows: got %v, want [other.txt]", got)
	}
	if m.dirText() != "run/" || m.seg != "" {
		t.Errorf("input: got %q + %q, want %q + empty segment", m.dirText(), m.seg, "run/")
	}
}

func TestPathModel_SlashWithHighlightedMatch_StepsIntoHighlightedDirectory(t *testing.T) {
	// given a segment that fuzzy-matches a directory, when / is typed, then the highlighted directory opens
	// (why: rnr/ is a shorthand for runner/)

	// Arrange
	root := tree(t, "runner/inner.txt", "other.txt")

	// Act
	m := typed(newTestPath(root, "file"), "rnr/")

	// Assert
	if m.dirText() != "runner/" || m.seg != "" {
		t.Errorf("input: got %q + %q, want %q + empty segment", m.dirText(), m.seg, "runner/")
	}
}

func TestPathModel_SlashWithNoMatch_ShowsErrorAndKeepsSegment(t *testing.T) {
	// given a segment matching no directory, when / is typed, then "no directory matching" shows and the segment stays
	// (why: the user must see why nothing happened and can fix the segment)

	// Arrange
	root := tree(t, "a.txt", "sub/")

	// Act
	m := typed(newTestPath(root, "file"), "zzz/")

	// Assert
	if !strings.Contains(m.View(), `no directory matching "zzz"`) {
		t.Errorf("view missing error:\n%s", m.View())
	}
	if m.seg != "zzz" || m.dirText() != "" {
		t.Errorf("input: got %q + %q, want %q + %q", m.dirText(), m.seg, "", "zzz")
	}
}

func TestPathModel_SlashOnAFileMatch_DoesNotStepIn(t *testing.T) {
	// given a segment naming a file with no directory matching, when / is typed, then it errors
	// (why: only directories can be stepped into)

	// Arrange
	root := tree(t, "notes.txt")

	// Act
	m := typed(newTestPath(root, "file"), "notes.txt/")

	// Assert
	if m.seg != "notes.txt" || m.errMsg == "" {
		t.Errorf("got seg %q err %q, want the segment kept with an error", m.seg, m.errMsg)
	}
}

func TestPathModel_DotDotSlashAndTildeSlashAndLeadingSlashWorkAsInAShell(t *testing.T) {
	// given a picker in a subdirectory, when ../, ~/ and a leading / are typed, then each lands where a shell would and keeps that form
	// (why: the directory part keeps the form the user started in)

	// Arrange
	root := tree(t, "sub/")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	sub := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: filepath.Join(root, "sub")})

	// Act
	up := typed(sub, "../")
	tilde := typed(sub, "~/")
	abs := typed(sub, "/")

	// Assert
	if up.cwd != root || up.dirText() != "../" {
		t.Errorf("../: got %q (%q), want %q", up.cwd, up.dirText(), root)
	}
	if tilde.cwd != home || tilde.dirText() != "~/" {
		t.Errorf("~/: got %q (%q), want %q", tilde.cwd, tilde.dirText(), home)
	}
	if abs.cwd != string(filepath.Separator) || abs.dirText() != "/" {
		t.Errorf("/: got %q (%q), want root", abs.cwd, abs.dirText())
	}
}

func TestPathModel_StartDirectoryKeepsItsWrittenForm(t *testing.T) {
	// given starts written relative, ~-based and absolute, when the picker opens, then the input shows each as written
	// (why: the directory part keeps the form the user started in)

	// Arrange
	root := tree(t, "sub/deep/")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.Mkdir(filepath.Join(home, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Act
	rel := newPathModel(PathPrompt{Type: "dir", Invoke: root, Start: "sub/deep"})
	tilde := newPathModel(PathPrompt{Type: "dir", Invoke: root, Start: "~/docs"})
	abs := newPathModel(PathPrompt{Type: "dir", Invoke: root, Start: filepath.Join(root, "sub")})

	// Assert
	for _, c := range []struct{ got, want string }{
		{rel.dirText(), "sub/deep/"}, {tilde.dirText(), "~/docs/"}, {abs.dirText(), filepath.ToSlash(filepath.Join(root, "sub")) + "/"},
	} {
		if c.got != c.want {
			t.Errorf("dirText: got %q, want %q", c.got, c.want)
		}
	}
}

func TestPathModel_UpAndDownMoveTheCursor(t *testing.T) {
	// given three files, when down, down, up are pressed, then the cursor sits on the second
	// (why: arrows are the way to highlight an entry)

	// Arrange
	m := newTestPath(tree(t, "a", "b", "c"), "file")

	// Act
	m = press(m, keyDown, keyDown, keyUp)

	// Assert
	if m.cursor != 1 {
		t.Errorf("cursor: got %d, want 1", m.cursor)
	}
}

func TestPathModel_RightOpensTheHighlightedDirectory(t *testing.T) {
	// given a highlighted directory, when right is pressed, then it opens
	// (why: right is the browse-in key)

	// Arrange
	m := newTestPath(tree(t, "sub/inner.txt"), "file")

	// Act
	m = press(m, keyRight)

	// Assert
	if got := names(m); !equal(got, []string{"inner.txt"}) || m.dirText() != "sub/" {
		t.Errorf("got %v in %q, want [inner.txt] in sub/", got, m.dirText())
	}
}

func TestPathModel_RightOnAFile_DoesNothing(t *testing.T) {
	// given a highlighted file, when right is pressed, then nothing changes
	// (why: only directories open)

	// Arrange
	m := newTestPath(tree(t, "a.txt"), "file")

	// Act
	m = press(m, keyRight)

	// Assert
	if m.dirText() != "" {
		t.Errorf("dirText: got %q, want unchanged", m.dirText())
	}
}

func TestPathModel_LeftGoesUpALevelAndHighlightsWhereItCameFrom(t *testing.T) {
	// given a picker inside a subdirectory, when left is pressed, then it goes up with that directory highlighted
	// (why: going back should land you where you were)

	// Arrange
	root := tree(t, "a/", "sub/inner.txt")
	m := press(newTestPath(root, "file"), keyDown, keyRight)

	// Act
	m = press(m, keyLeft)

	// Assert
	if m.cwd != root || m.dirText() != "" {
		t.Errorf("cwd: got %q (%q), want %q", m.cwd, m.dirText(), root)
	}
	if m.cursor != 1 {
		t.Errorf("cursor: got %d, want 1 (sub/)", m.cursor)
	}
}

func TestPathModel_BackspaceOnEmptySegmentGoesUp(t *testing.T) {
	// given an empty segment inside a subdirectory, when backspace is pressed, then the picker goes up
	// (why: backspace eats the path the way it would in a shell)

	// Arrange
	root := tree(t, "sub/inner.txt")
	m := press(newTestPath(root, "file"), keyRight)

	// Act
	m = press(m, keyBack)

	// Assert
	if m.cwd != root {
		t.Errorf("cwd: got %q, want %q", m.cwd, root)
	}
}

func TestPathModel_BackspaceWithSegment_DeletesALetter(t *testing.T) {
	// given a typed segment, when backspace is pressed, then only the last letter goes
	// (why: backspace only goes up once the segment is empty)

	// Arrange
	m := typed(newTestPath(tree(t, "sub/"), "file"), "su")

	// Act
	m = press(m, keyBack)

	// Assert
	if m.seg != "s" || m.cwd == filepath.Dir(m.prompt.Invoke) {
		t.Errorf("seg: got %q, want %q", m.seg, "s")
	}
}

func TestPathModel_EnterOnAFile_PicksItsAbsolutePath(t *testing.T) {
	// given a highlighted file, when enter is pressed, then its absolute path is picked and the program quits
	// (why: enter confirms the highlighted entry)

	// Arrange
	root := tree(t, "a.txt")
	m := newTestPath(root, "file")

	// Act
	next, cmd := m.Update(keyEnter)

	// Assert
	got := next.(pathModel)
	if got.picked != filepath.Join(root, "a.txt") || !got.done || cmd == nil {
		t.Errorf("got picked %q done %v, want %q", got.picked, got.done, filepath.Join(root, "a.txt"))
	}
}

func TestPathModel_EnterOnADirectoryInAFilePicker_OpensIt(t *testing.T) {
	// given a highlighted directory in a file picker, when enter is pressed, then it opens instead of being picked
	// (why: a file picker can only pick files)

	// Arrange
	m := newTestPath(tree(t, "sub/inner.txt"), "file")

	// Act
	m = press(m, keyEnter)

	// Assert
	if m.done || m.dirText() != "sub/" {
		t.Errorf("done %v dirText %q, want an open sub/", m.done, m.dirText())
	}
}

func TestPathModel_EnterOnADirectoryInADirPicker_PicksIt(t *testing.T) {
	// given a highlighted directory in a dir picker, when enter is pressed, then the directory is picked
	// (why: a dir picker picks directories)

	// Arrange
	root := tree(t, "sub/")
	m := press(newTestPath(root, "dir"), keyDown)

	// Act
	m = press(m, keyEnter)

	// Assert
	if m.picked != filepath.Join(root, "sub") {
		t.Errorf("picked: got %q, want %q", m.picked, filepath.Join(root, "sub"))
	}
}

func TestPathModel_EnterOnThisDirectoryRow_PicksTheListedDirectory(t *testing.T) {
	// given a dir picker on its "./ (this directory)" row, when enter is pressed, then the listed directory is picked
	// (why: that row exists to pick the directory you are in)

	// Arrange
	root := tree(t, "sub/")
	m := newTestPath(root, "dir")

	// Act
	m = press(m, keyEnter)

	// Assert
	if m.picked != root {
		t.Errorf("picked: got %q, want %q", m.picked, root)
	}
}

func TestPathModel_EnterWithNothingMatching_PicksNothing(t *testing.T) {
	// given a segment matching nothing, when enter is pressed, then nothing is picked
	// (why: there is no highlighted entry to pick)

	// Arrange
	m := typed(newTestPath(tree(t, "a.txt"), "file"), "zzz")

	// Act
	m = press(m, keyEnter)

	// Assert
	if m.done || m.picked != "" {
		t.Errorf("done %v picked %q, want nothing picked", m.done, m.picked)
	}
}

func TestPathModel_TabCompletesTheHighlightedName(t *testing.T) {
	// given a segment fuzzy-matching a file, when tab is pressed, then the segment becomes the full name
	// (why: tab completes the name, as in a shell)

	// Arrange
	m := typed(newTestPath(tree(t, "config.yml"), "file"), "cfg")

	// Act
	m = press(m, keyTab)

	// Assert
	if m.seg != "config.yml" || m.done {
		t.Errorf("seg: got %q done %v, want config.yml not done", m.seg, m.done)
	}
}
