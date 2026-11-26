package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hobnob/internal/ask"
	"hobnob/internal/value"

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

// pathCase is the question a test opens a picker with.
type pathCase struct {
	Type     string
	VarName  string
	Info     string
	Invoke   string
	WorkDir  string
	Default  []string
	Multi    bool
	Optional bool
	Included func(path string, isDir bool) bool
	Excluded func(path string, isDir bool) bool
}

func newPickerModel(c pathCase) pathModel {
	return newPathModel(ask.Question{
		VarName: c.VarName, Info: c.Info, Multi: c.Multi, Optional: c.Optional,
		Path: &ask.PathRules{Dir: c.Type == "dir", Invoke: c.Invoke, WorkDir: c.WorkDir, Defaults: c.Default, Included: c.Included, Excluded: c.Excluded},
	})
}

func newTestPath(root, kind string) pathModel {
	return newPickerModel(pathCase{Type: kind, VarName: "CFG", Invoke: root})
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
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
)

func names(m pathModel) []string {
	var out []string
	listing := false
	for _, line := range strings.Split(m.View(), "\n") {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
		switch {
		case strings.HasPrefix(line, "──"):
			listing = true
		case strings.HasPrefix(line, "╰"):
			listing = false
		case !listing, line == "":
		default:
			line = strings.TrimSpace(strings.TrimPrefix(line, "▶"))
			line = strings.TrimSpace(strings.TrimLeft(line, "◉○"))
			out = append(out, line)
		}
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

func TestPathModel_View_ShowsCardWithTitleInfoFilterLineMarkersAndHints(t *testing.T) {
	// given a file picker with info, when viewed, then it shows a card titled with the task and var, the info, the absolute path on the filter line, the cursor marker and the hints
	// (why: the picker must look like the other prompts)

	// Arrange
	root := tree(t, "a.txt")
	m := newPickerModel(pathCase{Type: "file", VarName: "CFG", Info: "Which config?", Invoke: root})
	m.question.Task = "deploy"

	// Act
	view := m.View()

	// Assert
	for _, want := range []string{"╭─ ", "deploy", "» ", "CFG", "Which config?", "› ", "▶ ", "a.txt", "↑↓", "enter"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Pick a file") || strings.Contains(strings.ToLower(view), "folder") {
		t.Errorf("view has the old header or says folder:\n%s", view)
	}
}

func TestPathModel_View_FilterLineShowsOnlyTheLast40CharactersAfterAnEllipsis(t *testing.T) {
	// given a path longer than 40 characters, when viewed, then the filter line shows its tail after "…" while the text stays whole
	// (why: display only; typing still edits the full path)

	// Arrange
	root := tree(t, "aaaaaaaaaa/bbbbbbbbbb/cccccccccc/dddddddddd/eeeeeeeeee/")
	m := newPickerModel(pathCase{Type: "dir", Invoke: filepath.Join(root, "aaaaaaaaaa/bbbbbbbbbb/cccccccccc/dddddddddd/eeeeeeeeee")})
	full := m.text

	// Act
	view := m.View()

	// Assert
	tail := "…" + full[len(full)-39:]
	if !strings.Contains(view, tail) || strings.Contains(view, full) || m.text != full {
		t.Errorf("want only %q on the filter line:\n%s", tail, view)
	}
}

func TestPathModel_View_NonexistentDirectoryShowsNoDirectoryMessage(t *testing.T) {
	// given text naming a directory that does not exist, when viewed, then the listing says no directory and names it
	// (why: the text alone decides where the picker is, so a typo must be explained)

	// Arrange
	root := tree(t, "a.txt")
	m := newTestPath(root, "file")
	m.text = "/nope/"

	// Act
	view := m.View()

	// Assert
	if !strings.Contains(view, `no directory "/nope/"`) {
		t.Errorf("view missing message:\n%s", view)
	}
	if got := names(m); len(got) != 1 || got[0] != `no directory "/nope/"` {
		t.Errorf("rows: got %v, want only the message", got)
	}
}

func TestPathModel_TypingSlash_ListsTheDirectoryTheTextNames(t *testing.T) {
	// given a typed name, when / is typed after it, then the directory the text names is listed and the segment is empty
	// (why: / is plain text; the text alone decides the listing)

	// Arrange
	root := tree(t, "runner/inner.txt", "run/other.txt")

	// Act
	m := typed(newTestPath(root, "file"), "run/")

	// Assert
	if got := names(m); !equal(got, []string{"other.txt"}) {
		t.Errorf("rows: got %v, want [other.txt]", got)
	}
	if m.seg() != "" || m.text != root+"/run/" {
		t.Errorf("text: got %q, want %q", m.text, root+"/run/")
	}
}

func TestPathModel_TypingFuzzyNameThenSlash_DoesNotOpenTheHighlightedMatch(t *testing.T) {
	// given a segment that only fuzzy-matches a directory, when / is typed, then the text names a directory that does not exist
	// (why: no exact-or-highlighted guessing; only the text decides)

	// Arrange
	root := tree(t, "runner/inner.txt")

	// Act
	m := typed(newTestPath(root, "file"), "rnr/")

	// Assert
	if m.dirExists() || m.text != root+"/rnr/" {
		t.Errorf("text %q exists=%v, want the typed text and a missing directory", m.text, m.dirExists())
	}
}

func TestPathModel_TypedDotDotSegmentsListWhateverDirectoryTheyName(t *testing.T) {
	// given a picker in a subdirectory, when ../ is typed, then the parent is listed
	// (why: the text is the only source of where the picker is)

	// Arrange
	root := tree(t, "sub/", "top.txt")
	sub := newPickerModel(pathCase{Type: "file", VarName: "X", Invoke: filepath.Join(root, "sub")})

	// Act
	got := names(typed(sub, "../"))

	// Assert
	if want := []string{"sub/", "top.txt"}; !equal(got, want) {
		t.Errorf("rows: got %v, want %v", got, want)
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
	if got := names(m); !equal(got, []string{"inner.txt"}) || !strings.HasSuffix(m.text, "/sub/") {
		t.Errorf("got %v in %q, want [inner.txt] in .../sub/", got, m.text)
	}
}

func TestPathModel_RightOnAFile_DoesNothing(t *testing.T) {
	// given a highlighted file, when right is pressed, then nothing changes
	// (why: only directories open)

	// Arrange
	root := tree(t, "a.txt")
	m := newTestPath(root, "file")

	// Act
	m = press(m, keyRight)

	// Assert
	if m.text != root+"/" {
		t.Errorf("text: got %q, want unchanged %q", m.text, root+"/")
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
	if m.dir() != root || m.text != root+"/" {
		t.Errorf("dir: got %q (%q), want %q", m.dir(), m.text, root)
	}
	if m.cursor != 1 {
		t.Errorf("cursor: got %d, want 1 (sub/)", m.cursor)
	}
}

func TestPathModel_BackspaceOnTrailingSlash_ListsTheDirectoryTheTextNowNames(t *testing.T) {
	// given text ending in the slash of a subdirectory, when backspace is pressed, then the slash is gone and the parent is listed
	// (why: backspace only edits text)

	// Arrange
	root := tree(t, "sub/inner.txt")
	m := press(newTestPath(root, "file"), keyRight)

	// Act
	m = press(m, keyBack)

	// Assert
	if m.text != root+"/sub" || m.dir() != root || !equal(names(m), []string{"sub/"}) {
		t.Errorf("text %q dir %q rows %v, want the parent listing filtered by sub", m.text, m.dir(), names(m))
	}
}

func TestPathModel_BackspaceCannotDeleteTheLeadingSlash(t *testing.T) {
	// given text that is just "/", when backspace is pressed, then the text is unchanged
	// (why: the filter line is never empty)

	// Arrange
	m := newTestPath(tree(t), "dir")
	m.text = "/"

	// Act
	m = press(m, keyBack, keyBack)

	// Assert
	if m.text != "/" {
		t.Errorf("text: got %q, want %q", m.text, "/")
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
	if m.seg() != "s" {
		t.Errorf("seg: got %q, want %q", m.seg(), "s")
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
	if got.answer.String() != filepath.Join(root, "a.txt") || !got.done || cmd == nil {
		t.Errorf("got picked %q done %v, want %q", got.answer.String(), got.done, filepath.Join(root, "a.txt"))
	}
}

func TestPathModel_EnterOnADirectoryInAFilePicker_ShowsThatRightOpensIt(t *testing.T) {
	// given a highlighted directory in a file picker, when enter is pressed, then nothing opens or is picked and "that's a directory: → opens it" shows
	// (why: enter only picks)

	// Arrange
	root := tree(t, "sub/inner.txt")
	m := newTestPath(root, "file")

	// Act
	m = press(m, keyEnter)

	// Assert
	if m.done || m.text != root+"/" || !strings.Contains(m.View(), "that's a directory: → opens it") {
		t.Errorf("done %v text %q, want nothing opened and the hint:\n%s", m.done, m.text, m.View())
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
	if m.answer.String() != filepath.Join(root, "sub") {
		t.Errorf("picked: got %q, want %q", m.answer.String(), filepath.Join(root, "sub"))
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
	if m.answer.String() != root {
		t.Errorf("picked: got %q, want %q", m.answer.String(), root)
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
	if m.done || m.answer.String() != "" {
		t.Errorf("done %v picked %q, want nothing picked", m.done, m.answer.String())
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
	if m.seg() != "config.yml" || m.done {
		t.Errorf("seg: got %q done %v, want config.yml not done", m.seg(), m.done)
	}
}

func TestPathModel_Multi_TabTogglesPicksAndEnterConfirmsAll(t *testing.T) {
	// given a multi file picker, when tab toggles two files and enter confirms, then both are returned in pick order
	// (why: multi: collects several answers before confirming)

	// Arrange
	root := tree(t, "a.txt", "b.txt", "c.txt")
	m := newPickerModel(pathCase{Type: "file", VarName: "X", Invoke: root, Multi: true})

	// Act
	m = press(m, keyDown, keyTab, keyUp, keyUp, keyTab)
	view := m.View()
	m = press(m, keyEnter)

	// Assert
	want := []string{filepath.Join(root, "b.txt"), filepath.Join(root, "a.txt")}
	if !m.done || !equal(m.picks, want) {
		t.Errorf("picks: got %v done=%v, want %v", m.picks, m.done, want)
	}
	if !strings.Contains(view, "b.txt") || !strings.Contains(view, "picked") {
		t.Errorf("view should list picks above the input, got:\n%s", view)
	}
}

func TestPathModel_Multi_TabAgainUnpicks(t *testing.T) {
	// given a picked file, when tab is pressed on it again, then it is no longer picked
	// (why: tab is a toggle)

	// Arrange
	root := tree(t, "a.txt", "b.txt")
	m := newPickerModel(pathCase{Type: "file", VarName: "X", Invoke: root, Multi: true})

	// Act
	m = press(m, keyTab, keyUp, keyTab)

	// Assert
	if len(m.picks) != 0 {
		t.Errorf("picks: got %v, want none", m.picks)
	}
}

func TestPathModel_Multi_DefaultArrivesAlreadyMarked(t *testing.T) {
	// given a multi default list, when the picker opens, then those paths are already picked and enter confirms them
	// (why: a default is a pre-made answer the user can accept)

	// Arrange
	root := tree(t, "a.txt", "b.txt")
	m := newPickerModel(pathCase{Type: "file", VarName: "X", Invoke: root, Multi: true, Default: []string{"a.txt", "b.txt"}})

	// Act
	m = press(m, keyEnter)

	// Assert
	want := []string{filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")}
	if !m.done || !equal(m.picks, want) {
		t.Errorf("picks: got %v, want %v", m.picks, want)
	}
}

func TestPathModel_Optional_EscClearsSegmentThenGivesNoAnswer(t *testing.T) {
	// given an optional picker with text typed, when esc is pressed twice, then the first clears the segment and the second skips
	// (why: esc backs out one step at a time)

	// Arrange
	root := tree(t, "a.txt")
	m := newPickerModel(pathCase{Type: "file", VarName: "X", Invoke: root, Optional: true})
	m = typed(m, "ab")

	// Act
	m = press(m, keyEsc)
	afterFirst := m

	m = press(m, keyEsc)

	// Assert
	if afterFirst.seg() != "" || afterFirst.done {
		t.Errorf("first esc: seg=%q done=%v, want cleared and not done", afterFirst.seg(), afterFirst.done)
	}
	if !m.done {
		t.Errorf("second esc: done=%v, want true", m.done)
	}
}

func TestPathModel_Required_EscDoesNothing(t *testing.T) {
	// given a picker that is not optional, when esc is pressed on an empty segment, then it stays open
	// (why: an answer is required)

	// Arrange
	m := newTestPath(tree(t, "a.txt"), "file")

	// Act
	m = press(m, keyEsc)

	// Assert
	if m.done {
		t.Error("esc finished a required picker")
	}
}

func TestPathModel_DirPicker_NonMatchingDirCanBeOpenedByRightButNotPicked(t *testing.T) {
	// given a dir picker whose predicate matches only "ok", when enter is pressed on "other", then it is refused and right opens it; on "ok" enter picks
	// (why: you may need to walk through non-matching dirs to reach a matching one)

	// Arrange
	root := tree(t, "other/ok/", "ok/")
	only := func(path string, isDir bool) bool { return filepath.Base(path) == "ok" }
	m := typed(newPickerModel(pathCase{Type: "dir", VarName: "X", Invoke: root, Included: only}), "oth")

	// Act
	refused := press(m, keyEnter)
	opened := press(m, keyRight)
	picked := press(typed(newPickerModel(pathCase{Type: "dir", VarName: "X", Invoke: root, Included: only}), "ok"), keyEnter)

	// Assert
	if refused.done || refused.errMsg == "" {
		t.Errorf("enter on non-matching dir: done=%v err=%q, want a refusal", refused.done, refused.errMsg)
	}
	if opened.done || opened.dir() != filepath.Join(root, "other") {
		t.Errorf("right on non-matching dir: dir=%s done=%v, want it opened", opened.dir(), opened.done)
	}
	if !picked.done || picked.answer.String() != filepath.Join(root, "ok") {
		t.Errorf("enter on matching dir: picked=%q done=%v", picked.answer.String(), picked.done)
	}
}

func TestPathModel_DirPicker_ThisDirectoryNotPickableUnlessItMatches(t *testing.T) {
	// given a dir picker whose predicate rejects the listed directory, when enter is pressed on the here row, then nothing is picked
	// (why: only matching directories can be picked)

	// Arrange
	root := tree(t, "sub/")
	none := func(path string, isDir bool) bool { return false }
	m := newPickerModel(pathCase{Type: "dir", VarName: "X", Invoke: root, Included: none})

	// Act
	m = press(m, keyEnter)

	// Assert
	if m.done || m.errMsg == "" {
		t.Errorf("done=%v err=%q, want refused with a message", m.done, m.errMsg)
	}
}

func TestPathModel_Open_FileDefaultIsPreselectedInItsDirectory(t *testing.T) {
	// given a file default in a subdirectory, when the picker opens, then it lists that directory with the file highlighted
	// (why: accepting the default should be one keypress)

	// Arrange
	root := tree(t, "sub/a.txt", "sub/b.txt")

	// Act
	m := newPickerModel(pathCase{Type: "file", VarName: "X", Invoke: root, Default: []string{"sub/b.txt"}})

	// Assert
	row, ok := m.highlighted()
	if m.dir() != filepath.Join(root, "sub") || !ok || row.name != "b.txt" {
		t.Errorf("cwd=%s row=%v, want sub with b.txt highlighted", m.dir(), row.name)
	}
}

func TestPathModel_Open_DirDefaultIsPreselectedInItsParent(t *testing.T) {
	// given a directory default in a dir picker, when it opens, then the parent is listed with that directory highlighted
	// (why: same one-keypress accept as for files)

	// Arrange
	root := tree(t, "x/", "y/")

	// Act
	m := newPickerModel(pathCase{Type: "dir", VarName: "X", Invoke: root, Default: []string{"y"}})

	// Assert
	row, ok := m.highlighted()
	if m.dir() != root || !ok || row.name != "y" {
		t.Errorf("cwd=%s row=%v, want root with y highlighted", m.dir(), row.name)
	}
}

func TestPathModel_Multi_ViewUsesMenuMarkers(t *testing.T) {
	// given a multi picker with one pick, when it renders, then the pick has a filled marker and others an empty one, with no check mark
	// (why: multi pickers match the select menu's markers)

	// Arrange
	root := tree(t, "a.txt", "b.txt")
	m := newPickerModel(pathCase{Type: "file", VarName: "X", Invoke: root, Multi: true})

	// Act
	m = press(m, keyTab)
	view := m.View()

	// Assert
	if !strings.Contains(view, "◉ ") || !strings.Contains(view, "○ ") || strings.Contains(view, "✓") {
		t.Errorf("view should use ◉/○ and no ✓, got:\n%s", view)
	}
}

func TestPathModel_Multi_TabMovesCursorDown(t *testing.T) {
	// given a multi picker, when tab toggles a row, then the cursor moves to the next row
	// (why: lets the user pick a run of rows quickly)

	// Arrange
	root := tree(t, "a.txt", "b.txt")
	m := newPickerModel(pathCase{Type: "file", VarName: "X", Invoke: root, Multi: true})

	// Act
	m = press(m, keyTab)

	// Assert
	if m.cursor != 1 {
		t.Errorf("cursor: got %d, want 1", m.cursor)
	}
}

func TestPathModel_DirPicker_ThisDirectoryRowNotPickableInExcludedDirectory(t *testing.T) {
	// given a dir picker inside .git with !.* excluded, when it lists, then ./ can't be picked
	// (why: an excluded directory is never an answer)

	// Arrange
	root := tree(t, ".git/")
	excluded := func(p string, isDir bool) bool { return strings.HasPrefix(filepath.Base(p), ".") }
	m := newPickerModel(pathCase{Type: "dir", Invoke: root, WorkDir: filepath.Join(root, ".git"), Excluded: excluded})

	// Act
	row, ok := m.highlighted()
	m = press(m, keyEnter)

	// Assert
	if !ok || !row.here || row.pickable || m.done {
		t.Errorf("here row: %+v ok=%v done=%v, want unpickable and not done", row, ok, m.done)
	}
}

func TestPathModel_DirPicker_ThisDirectoryRowNotPickableInJunkDirectory(t *testing.T) {
	// given a dir picker inside a junk-named directory, when it lists, then ./ can't be picked
	// (why: junk names are never offered)

	// Arrange
	root := tree(t, "Thumbs.db/")
	m := newPickerModel(pathCase{Type: "dir", Invoke: root, WorkDir: filepath.Join(root, "Thumbs.db")})

	// Act
	row, _ := m.highlighted()

	// Assert
	if !row.here || row.pickable {
		t.Errorf("here row: %+v, want unpickable", row)
	}
}

func TestPathModel_RefusalFromTheJudgeShowsOnTheErrorLineAndKeepsThePickerOpen(t *testing.T) {
	// given a picker whose question's check rejects every path, when enter is pressed on a file, then the refusal shows on the error line and the picker stays open
	// (why: a failed check: is a refusal inside the picker, not a reopen)

	// Arrange
	root := tree(t, "a.txt")
	m := newTestPath(root, "file")
	m.question.CheckDesc = "never"
	m.question.Check = func(value.Value) (bool, error) { return false, nil }

	// Act
	m = press(m, keyEnter)

	// Assert
	if m.done || m.errMsg != "validation failed: never" {
		t.Errorf("done=%v err=%q, want an open picker showing the refusal", m.done, m.errMsg)
	}
}

func TestPathModel_Open_DirDefaultFailingMatchIsStillHighlighted(t *testing.T) {
	// given a directory default that exists but fails match:, when the picker opens, then it lists the parent with that directory highlighted though it can't be picked
	// (why: the person sees where the default is)

	// Arrange
	root := tree(t, "x/", "y/")
	onlyX := func(abs string, _ bool) bool { return filepath.Base(abs) == "x" }

	// Act
	m := newPickerModel(pathCase{Type: "dir", VarName: "X", Invoke: root, Default: []string{"y"}, Included: onlyX})

	// Assert
	row, ok := m.highlighted()
	if m.dir() != root || !ok || row.name != "y" {
		t.Errorf("cwd=%s row=%v, want root with y highlighted", m.dir(), row.name)
	}
}
