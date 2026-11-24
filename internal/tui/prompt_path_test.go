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
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
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
	m := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Included: func(path string, isDir bool) bool {
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

func hasPrefixLabel(m pathModel, label string) bool {
	for _, n := range names(m) {
		if n == label {
			return true
		}
	}
	return false
}

func TestPathModel_Multi_TabTogglesPicksAndEnterConfirmsAll(t *testing.T) {
	// given a multi file picker, when tab toggles two files and enter confirms, then both are returned in pick order
	// (why: multi: collects several answers before confirming)

	// Arrange
	root := tree(t, "a.txt", "b.txt", "c.txt")
	m := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Multi: true})

	// Act
	m = press(m, keyDown, keyTab, keyUp, keyTab)
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
	m := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Multi: true})

	// Act
	m = press(m, keyTab, keyTab)

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
	m := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Multi: true, Default: []string{"a.txt", "b.txt"}})

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
	m := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Optional: true})
	m = typed(m, "ab")

	// Act
	m = press(m, keyEsc)
	afterFirst := m

	m = press(m, keyEsc)

	// Assert
	if afterFirst.seg != "" || afterFirst.done {
		t.Errorf("first esc: seg=%q done=%v, want cleared and not done", afterFirst.seg, afterFirst.done)
	}
	if !m.done || !m.skipped {
		t.Errorf("second esc: done=%v skipped=%v, want both true", m.done, m.skipped)
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

func TestPathModel_FilePicker_PlainPatternNeverHidesADirectoryButBangPatternDoes(t *testing.T) {
	// given plain and ! predicates, when a file picker lists, then the plain one hides only files and the ! one hides a directory too
	// (why: dirs must stay reachable for plain patterns, while !.* must hide .git/; mutation checklist line 74)

	// Arrange
	root := tree(t, "a.yml", "b.txt", "sub/", ".git/")
	plain := func(path string, isDir bool) bool { return strings.HasSuffix(path, ".yml") }
	bang := func(path string, isDir bool) bool { return strings.HasPrefix(filepath.Base(path), ".") }

	// Act
	withPlain := names(newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Included: plain}))
	withBang := names(newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Excluded: bang}))

	// Assert
	if want := []string{".git/", "sub/", "a.yml"}; !equal(withPlain, want) {
		t.Errorf("plain: got %v, want %v", withPlain, want)
	}
	if want := []string{"sub/", "a.yml", "b.txt"}; !equal(withBang, want) {
		t.Errorf("bang: got %v, want %v", withBang, want)
	}
}

func TestPathModel_DirPicker_NonMatchingDirCanBeOpenedButNotPicked(t *testing.T) {
	// given a dir picker whose predicate matches only "ok", when enter is pressed on "other", then it opens; on "ok" it is picked
	// (why: you may need to walk through non-matching dirs to reach a matching one)

	// Arrange
	root := tree(t, "other/ok/", "ok/")
	only := func(path string, isDir bool) bool { return filepath.Base(path) == "ok" }
	m := newPathModel(PathPrompt{Type: "dir", VarName: "X", Invoke: root, Included: only})

	// Act
	m = typed(m, "oth")
	opened := press(m, keyEnter)
	picked := press(typed(newPathModel(PathPrompt{Type: "dir", VarName: "X", Invoke: root, Included: only}), "ok"), keyEnter)

	// Assert
	if opened.done || opened.cwd != filepath.Join(root, "other") {
		t.Errorf("enter on non-matching dir: cwd=%s done=%v, want it opened", opened.cwd, opened.done)
	}
	if !picked.done || picked.picked != filepath.Join(root, "ok") {
		t.Errorf("enter on matching dir: picked=%q done=%v", picked.picked, picked.done)
	}
	if hasPrefixLabel(opened, "./ (this directory)") && opened.cwd == root {
		t.Error("unexpected here row")
	}
}

func TestPathModel_DirPicker_ThisDirectoryNotPickableUnlessItMatches(t *testing.T) {
	// given a dir picker whose predicate rejects the listed directory, when enter is pressed on the here row, then nothing is picked
	// (why: only matching directories can be picked)

	// Arrange
	root := tree(t, "sub/")
	none := func(path string, isDir bool) bool { return false }
	m := newPathModel(PathPrompt{Type: "dir", VarName: "X", Invoke: root, Included: none})

	// Act
	m = press(m, keyEnter)

	// Assert
	if m.done || m.errMsg == "" {
		t.Errorf("done=%v err=%q, want refused with a message", m.done, m.errMsg)
	}
}

func TestPathModel_View_ShowsMatchPatternsDimlyInHeader(t *testing.T) {
	// given match patterns, when the picker renders, then the header shows them
	// (why: the user should know what is being filtered)

	// Arrange
	m := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: tree(t), Patterns: []string{"*.yml", "!.*"}})

	// Act
	view := m.View()

	// Assert
	if !strings.Contains(view, "*.yml") || !strings.Contains(view, "!.*") {
		t.Errorf("view missing patterns:\n%s", view)
	}
}

func TestPathModel_Open_FileDefaultIsPreselectedInItsDirectory(t *testing.T) {
	// given a file default in a subdirectory, when the picker opens, then it lists that directory with the file highlighted
	// (why: accepting the default should be one keypress)

	// Arrange
	root := tree(t, "sub/a.txt", "sub/b.txt")

	// Act
	m := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Default: []string{"sub/b.txt"}})

	// Assert
	row, ok := m.highlighted()
	if m.cwd != filepath.Join(root, "sub") || !ok || row.name != "b.txt" {
		t.Errorf("cwd=%s row=%v, want sub with b.txt highlighted", m.cwd, row.name)
	}
}

func TestPathModel_Open_DirDefaultIsPreselectedInItsParent(t *testing.T) {
	// given a directory default in a dir picker, when it opens, then the parent is listed with that directory highlighted
	// (why: same one-keypress accept as for files)

	// Arrange
	root := tree(t, "x/", "y/")

	// Act
	m := newPathModel(PathPrompt{Type: "dir", VarName: "X", Invoke: root, Default: []string{"y"}})

	// Assert
	row, ok := m.highlighted()
	if m.cwd != root || !ok || row.name != "y" {
		t.Errorf("cwd=%s row=%v, want root with y highlighted", m.cwd, row.name)
	}
}

func TestPathModel_Open_DirectoryDefaultInFilePickerIsWhereBrowsingStarts(t *testing.T) {
	// given a directory default in a file picker, when it opens, then browsing starts inside that directory
	// (why: a file picker cannot pick it, but it hints where to look)

	// Arrange
	root := tree(t, "sub/a.txt", "other.txt")

	// Act
	m := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Default: []string{"sub"}})

	// Assert
	if m.cwd != filepath.Join(root, "sub") || !equal(names(m), []string{"a.txt"}) {
		t.Errorf("cwd=%s rows=%v", m.cwd, names(m))
	}
}

func TestPathModel_Open_MissingOrWrongKindDefaultOpensNearestExistingParent(t *testing.T) {
	// given a missing default and a file default in a dir picker, when it opens, then the nearest existing parent is listed with nothing selected
	// (why: the default cannot be used, but its neighbourhood is the best guess)

	// Arrange
	root := tree(t, "sub/a.txt")

	// Act
	missing := newPathModel(PathPrompt{Type: "file", VarName: "X", Invoke: root, Default: []string{"sub/gone/deeper/f.txt"}})
	wrong := newPathModel(PathPrompt{Type: "dir", VarName: "X", Invoke: root, Default: []string{"sub/a.txt"}})

	// Assert
	if missing.cwd != filepath.Join(root, "sub") || missing.cursor != 0 {
		t.Errorf("missing: cwd=%s cursor=%d", missing.cwd, missing.cursor)
	}
	if wrong.cwd != filepath.Join(root, "sub") || wrong.cursor != 0 {
		t.Errorf("wrong kind: cwd=%s cursor=%d", wrong.cwd, wrong.cursor)
	}
}

func TestPathModel_Open_NoDefaultStartsInInvokeDirectory(t *testing.T) {
	// given no default, when it opens, then it lists the working directory
	// (why: the task's working directory is the natural start)

	// Arrange
	root := tree(t, "a.txt")

	// Act
	m := newTestPath(root, "file")

	// Assert
	if m.cwd != root {
		t.Errorf("cwd=%s, want %s", m.cwd, root)
	}
}
