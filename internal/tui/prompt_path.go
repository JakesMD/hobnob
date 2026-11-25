package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"hobnob/internal/value"

	tea "github.com/charmbracelet/bubbletea"
)

// PathPrompt is everything the picker takes in. Paths written in the taskfile
// (Default) resolve against WorkDir and the start directory against Invoke;
// the filter line always holds the resulting absolute path. The caller passes
// both as absolute directories. Included and Excluded are the
// caller's predicates for match: (tui never learns about match:); both are
// applied on top of the type and the junk-name list, and either may be nil.
//
//   - Included is the plain patterns: in a file picker it hides non-matching
//     files (never directories); in a dir picker non-matching directories can
//     be opened but not picked.
//   - Excluded is the "!" patterns: whatever it reports true for is hidden,
//     directories included.
type PathPrompt struct {
	Type     string // "file" or "dir"
	VarName  string
	Info     string
	Start    string   // start directory as written: relative to Invoke, ~-based or absolute; empty means WorkDir
	Invoke   string   // absolute directory that relative paths resolve against
	WorkDir  string   // absolute directory of the asking task (dir:, else the taskfile's); relative Default items resolve against it and an empty Start opens in it; empty means Invoke
	Default  []string // as written in the taskfile (relative to WorkDir, ~-based or absolute); first one decides where the picker opens
	Multi    bool     // tab toggles picks, enter confirms them all; Default is the initial picks
	Optional bool     // esc clears the segment, then gives no answer
	Included func(path string, isDir bool) bool
	Excluded func(path string, isDir bool) bool
}

const pathListHeight = 12

// junkNames are never listed, on every OS (spec §5). Compared
// case-insensitively; "._*" is handled as a prefix.
var junkNames = []string{
	".DS_Store", ".Spotlight-V100", ".Trashes", ".fseventsd", ".TemporaryItems",
	"Icon\r", "Thumbs.db", "desktop.ini", "$RECYCLE.BIN", "System Volume Information",
}

func isJunk(name string) bool {
	if strings.HasPrefix(name, "._") {
		return true
	}
	for _, junk := range junkNames {
		if strings.EqualFold(name, junk) {
			return true
		}
	}
	return false
}

type pathEntry struct {
	name     string
	isDir    bool
	pickable bool
}

type pathRow struct {
	pathEntry
	here      bool
	positions []int
}

type pathModel struct {
	prompt    PathPrompt
	task      string
	home      string
	text      string // the filter line: always an absolute path; up to the last "/" is the listed directory
	cursor    int
	offset    int
	errMsg    string
	picked    string
	picks     []string // multi: absolute paths picked so far, in pick order
	done      bool
	quit      bool
	skipped   bool // esc on an optional picker: no answer
	termWidth int
}

func newPathModel(prompt PathPrompt) pathModel {
	home, _ := os.UserHomeDir()
	m := pathModel{prompt: prompt, home: home}
	if m.prompt.WorkDir == "" {
		m.prompt.WorkDir = prompt.Invoke
	}
	if prompt.Start == "" {
		m.text = withSlash(m.resolve("", m.prompt.WorkDir))
	} else {
		m.text = withSlash(m.resolve(prompt.Start, prompt.Invoke))
	}
	if prompt.Multi {
		for _, d := range prompt.Default {
			abs := m.resolve(d, m.prompt.WorkDir)
			if info, err := os.Stat(abs); err == nil && info.IsDir() == (prompt.Type == "dir") && m.pickable(abs, info.IsDir()) {
				m.picks = append(m.picks, abs)
			}
		}
	}
	if len(prompt.Default) > 0 {
		m.open(prompt.Default[0])
	}
	return m
}

func withSlash(p string) string {
	p = filepath.ToSlash(p)
	if strings.HasSuffix(p, "/") {
		return p
	}
	return p + "/"
}

// resolve turns a path as written into an absolute one; relative paths
// resolve against base, and empty means base itself.
func (m pathModel) resolve(written, base string) string {
	switch {
	case written == "":
		return filepath.Clean(base)
	case written == "~" || strings.HasPrefix(written, "~/"):
		return filepath.Join(m.home, strings.TrimPrefix(written, "~"))
	case filepath.IsAbs(written):
		return filepath.Clean(written)
	}
	return filepath.Join(base, written)
}

// open positions the picker for a default: a default of the prompt's kind is
// highlighted in its directory, a directory given to a file picker is where
// browsing starts, and anything else opens the nearest existing parent with
// nothing selected.
func (m *pathModel) open(written string) {
	abs := m.resolve(written, m.prompt.WorkDir)
	info, err := os.Stat(abs)
	switch {
	case err == nil && info.IsDir() && m.prompt.Type == "file":
		m.text = withSlash(abs)
		return
	case err == nil && info.IsDir() == (m.prompt.Type == "dir"):
		m.text = withSlash(filepath.Dir(abs))
		m.highlight(abs)
		return
	}
	dir := filepath.Dir(abs)
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			break
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	m.text = withSlash(dir)
}

func (m *pathModel) highlight(abs string) {
	for i, row := range m.rows() {
		if !row.here && filepath.Join(m.dir(), row.name) == abs {
			m.cursor = i
		}
	}
	m.clamp()
}

// dirText is the text up to and including its last "/": the directory the
// listing shows.
func (m pathModel) dirText() string {
	return m.text[:strings.LastIndex(m.text, "/")+1]
}

// seg is the text after the last "/": the only part that is fuzzy-matched.
func (m pathModel) seg() string {
	return m.text[strings.LastIndex(m.text, "/")+1:]
}

// dir is the absolute directory being listed.
func (m pathModel) dir() string { return filepath.Clean(filepath.FromSlash(m.dirText())) }

func (m pathModel) dirExists() bool {
	info, err := os.Stat(m.dir())
	return err == nil && info.IsDir()
}

// pickable reports whether a listed entry at abs could be picked: not junk,
// not Excluded, and for a file Included, for a directory Included too.
func (m pathModel) pickable(abs string, isDir bool) bool {
	return !isJunk(filepath.Base(abs)) &&
		(m.prompt.Excluded == nil || !m.prompt.Excluded(abs, isDir)) &&
		(m.prompt.Included == nil || m.prompt.Included(abs, isDir))
}

// entries lists the directory, applying the junk list, the type and the
// caller's predicate. Directories come first, then by name ignoring case.
func (m pathModel) entries() []pathEntry {
	items, _ := os.ReadDir(m.dir())
	var dirs, files []pathEntry
	for _, item := range items {
		if isJunk(item.Name()) {
			continue
		}
		abs := filepath.Join(m.dir(), item.Name())
		isDir := item.IsDir()
		if item.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(abs); err == nil {
				isDir = info.IsDir()
			}
		}
		if m.prompt.Type == "dir" && !isDir {
			continue
		}
		if m.prompt.Excluded != nil && m.prompt.Excluded(abs, isDir) {
			continue
		}
		included := m.prompt.Included == nil || m.prompt.Included(abs, isDir)
		if !isDir && !included {
			continue
		}
		if isDir {
			dirs = append(dirs, pathEntry{item.Name(), true, m.prompt.Type == "dir" && included})
		} else {
			files = append(files, pathEntry{item.Name(), false, true})
		}
	}
	byName := func(s []pathEntry) {
		sort.SliceStable(s, func(i, j int) bool { return strings.ToLower(s[i].name) < strings.ToLower(s[j].name) })
	}
	byName(dirs)
	byName(files)
	return append(dirs, files...)
}

func (m pathModel) rows() []pathRow {
	var out []pathRow
	if !m.dirExists() {
		return nil
	}
	seg := m.seg()
	if m.prompt.Type == "dir" && seg == "" {
		here := m.pickable(m.dir(), true)
		out = append(out, pathRow{pathEntry: pathEntry{name: ".", isDir: true, pickable: here}, here: true})
	}
	entries := m.entries()
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	for _, hit := range matchRows(names, seg) {
		out = append(out, pathRow{pathEntry: entries[hit.index], positions: hit.positions})
	}
	return out
}

// renderRow draws one picker row: cursor, pick marker, then the name.
func (m pathModel) renderRow(rows []pathRow) func(i int) string {
	return func(i int) string {
		row := rows[i]
		var b strings.Builder
		b.WriteString(cursorPrefix(i == m.cursor))
		if m.prompt.Multi {
			switch {
			case m.isPicked(m.rowPath(row)):
				b.WriteString(SChecked.Render("◉ "))
			case row.pickable:
				b.WriteString(SInfo.Render("○ "))
			default:
				b.WriteString("  ")
			}
		}
		switch {
		case row.here:
			b.WriteString(sPathDir.Render("./") + SInfo.Render(" (this directory)"))
		case row.isDir:
			b.WriteString(sPathDir.Render(highlightMatch(row.name, row.positions) + "/"))
		default:
			b.WriteString(highlightMatch(row.name, row.positions))
		}
		return b.String()
	}
}

func (m pathModel) Init() tea.Cmd { return nil }

func (m pathModel) highlighted() (pathRow, bool) {
	rows := m.rows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return pathRow{}, false
	}
	return rows[m.cursor], true
}

func (m *pathModel) clamp() {
	m.cursor = max(0, min(m.cursor, len(m.rows())-1))
	m.offset = scrollTo(m.cursor, m.offset, pathListHeight)
}

func (m *pathModel) setText(text string) {
	m.text = text
	m.cursor, m.offset = 0, 0
}

func (m *pathModel) up() {
	prev := m.dir()
	m.setText(withSlash(filepath.Dir(prev)))
	for i, row := range m.rows() {
		if filepath.Join(m.dir(), row.name) == prev {
			m.cursor = i
		}
	}
	m.clamp()
}

func (m pathModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = size.Width
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	m.errMsg = ""
	row, hasRow := m.highlighted()
	switch key.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "up":
		m.cursor--
	case "down":
		m.cursor++
	case "left":
		m.up()
	case "right":
		if hasRow && row.isDir && !row.here {
			m.setText(m.dirText() + row.name + "/")
		}
	case "backspace":
		if m.text != "/" {
			runes := []rune(m.text)
			m.setText(string(runes[:len(runes)-1]))
		}
	case "ctrl+u":
		m.setText(m.dirText())
	case "esc":
		switch {
		case m.seg() != "":
			m.setText(m.dirText())
		case m.prompt.Optional:
			m.skipped, m.done = true, true
			return m, tea.Quit
		}
	case "tab":
		switch {
		case !hasRow:
		case m.prompt.Multi:
			if row.pickable {
				m.toggle(m.rowPath(row))
				m.cursor++
			} else {
				m.errMsg = "can't pick that"
			}
		case !row.here:
			m.setText(m.dirText() + row.name)
		}
	case "enter":
		switch {
		case m.prompt.Multi && len(m.picks) > 0:
			m.done = true
			return m, tea.Quit
		case !hasRow:
			m.errMsg = "nothing matches"
		case row.isDir && !row.here && m.prompt.Type == "file":
			m.errMsg = "that's a directory: → opens it"
		case !row.pickable:
			m.errMsg = "doesn't match"
		case m.prompt.Multi:
			m.picks = []string{m.rowPath(row)}
			m.done = true
			return m, tea.Quit
		default:
			return m.finish(m.rowPath(row))
		}
	default:
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
			text := string(key.Runes)
			if key.Type == tea.KeySpace {
				text = " "
			}
			m.setText(m.text + text)
		}
	}
	m.clamp()
	return m, nil
}

func (m pathModel) rowPath(row pathRow) string {
	if row.here {
		return m.dir()
	}
	return filepath.Join(m.dir(), row.name)
}

func (m *pathModel) toggle(path string) {
	for i, p := range m.picks {
		if p == path {
			m.picks = append(append([]string{}, m.picks[:i]...), m.picks[i+1:]...)
			return
		}
	}
	m.picks = append(m.picks, path)
}

func (m pathModel) isPicked(path string) bool {
	for _, p := range m.picks {
		if p == path {
			return true
		}
	}
	return false
}

func (m pathModel) finish(path string) (tea.Model, tea.Cmd) {
	m.picked = path
	m.done = true
	return m, tea.Quit
}

// pathLimit is how many characters of the path the filter line shows.
const pathLimit = 40

// shortPath cuts a long path from the left, "…" standing for the cut part.
// Display only: typing and backspace edit the full path.
func shortPath(text string) string {
	runes := []rune(text)
	if len(runes) <= pathLimit {
		return text
	}
	return "…" + string(runes[len(runes)-pathLimit+1:])
}

func (m pathModel) View() string {
	if m.done {
		return ""
	}
	frame := card{
		task:      m.task,
		varName:   m.prompt.VarName,
		optional:  m.prompt.Optional,
		info:      m.prompt.Info,
		errMsg:    m.errMsg,
		termWidth: m.termWidth,
	}
	tab := hint{"tab", "complete"}
	enter := hint{"enter", "pick"}
	if m.prompt.Multi {
		tab, enter = hint{"tab", "toggle"}, hint{"enter", "confirm"}
	}
	frame.hints = []hint{{"", "type to filter"}, {"↑↓", "move"}, {"←", "up"}, {"→", "open"}, tab, enter}
	if m.prompt.Optional {
		frame.hints = append(frame.hints, hint{"esc", "skip"})
	}

	var lines []string
	if len(m.picks) > 0 {
		lines = append(lines, SInfo.Render(fmt.Sprintf("picked (%d):", len(m.picks))))
		for _, p := range m.picks {
			lines = append(lines, "  "+SChecked.Render("◉ ")+p)
		}
		lines = append(lines, "")
	}
	lines = append(lines, filterLine(shortPath(m.text), frame.innerWidth())...)
	rows := m.rows()
	if m.dirExists() {
		lines = append(lines, renderWindow(len(rows), m.offset, pathListHeight, m.renderRow(rows))...)
	} else {
		lines = append(lines, SInfo.Render(fmt.Sprintf("  no directory %q", m.dirText())))
	}
	frame.body = strings.Join(lines, "\n")
	return frame.render()
}

// PromptPath runs the picker and returns the picked absolute path.
// With Multi the value is a list of absolute paths. answered is false when an
// optional picker was escaped: there is no answer and nothing is printed.
func PromptPath(ctx context.Context, prompt PathPrompt, task string, secret bool) (val value.Value, answered bool, err error) {
	model := newPathModel(prompt)
	model.task = task
	result, err := tea.NewProgram(model, tea.WithContext(ctx)).Run()
	if err != nil {
		return value.Value{}, false, err
	}
	final := result.(pathModel)
	if final.quit {
		return value.Value{}, false, fmt.Errorf("aborted")
	}
	if final.skipped {
		return value.Value{}, false, nil
	}
	val = value.Str(final.picked)
	if prompt.Multi {
		list := make([]any, len(final.picks))
		for i, p := range final.picks {
			list[i] = p
		}
		val = value.Of(list)
	}
	return val, true, nil
}
