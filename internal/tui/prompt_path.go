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
// (Default) resolve against WorkDir; the start directory and the input line
// are relative to Invoke, so a WorkDir below Invoke shows as e.g. "sub/". The
// caller passes both as absolute directories. Included and Excluded are the
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
	Patterns []string // match: patterns as written, shown dimly in the header
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

func (r pathRow) label() string {
	switch {
	case r.here:
		return "./ (this directory)"
	case r.isDir:
		return r.name + "/"
	}
	return r.name
}

type pathModel struct {
	prompt  PathPrompt
	home    string
	form    string // how the directory is written: "rel", "home" or "abs"
	cwd     string // absolute directory being listed
	seg     string // the fuzzy-matched segment after the last "/"
	cursor  int
	offset  int
	errMsg  string
	picked  string
	picks   []string // multi: absolute paths picked so far, in pick order
	done    bool
	quit    bool
	skipped bool // esc on an optional picker: no answer
}

func newPathModel(prompt PathPrompt) pathModel {
	home, _ := os.UserHomeDir()
	m := pathModel{prompt: prompt, home: home}
	if m.prompt.WorkDir == "" {
		m.prompt.WorkDir = prompt.Invoke
	}
	if prompt.Start == "" {
		m.form, m.cwd = m.resolve("", m.prompt.WorkDir)
	} else {
		m.form, m.cwd = m.resolve(prompt.Start, prompt.Invoke)
	}
	if prompt.Multi {
		for _, d := range prompt.Default {
			_, abs := m.resolve(d, m.prompt.WorkDir)
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

// resolve turns a path as written into its form and absolute path; relative
// paths resolve against base, and empty means base itself.
func (m pathModel) resolve(written, base string) (form, abs string) {
	switch {
	case written == "":
		return "rel", filepath.Clean(base)
	case written == "~" || strings.HasPrefix(written, "~/"):
		return "home", filepath.Join(m.home, strings.TrimPrefix(written, "~"))
	case filepath.IsAbs(written):
		return "abs", filepath.Clean(written)
	}
	return "rel", filepath.Join(base, written)
}

// open positions the picker for a default: a default of the prompt's kind is
// highlighted in its directory, a directory given to a file picker is where
// browsing starts, and anything else opens the nearest existing parent with
// nothing selected.
func (m *pathModel) open(written string) {
	form, abs := m.resolve(written, m.prompt.WorkDir)
	m.form = form
	info, err := os.Stat(abs)
	switch {
	case err == nil && info.IsDir() && m.prompt.Type == "file":
		m.cwd = abs
		return
	case err == nil && info.IsDir() == (m.prompt.Type == "dir"):
		m.cwd = filepath.Dir(abs)
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
	m.cwd = dir
}

func (m *pathModel) highlight(abs string) {
	for i, row := range m.rows() {
		if !row.here && filepath.Join(m.cwd, row.name) == abs {
			m.cursor = i
		}
	}
	m.clamp()
}

// dirText writes the listed directory the way the user started writing it.
func (m pathModel) dirText() string {
	withSlash := func(p string) string {
		if p == "/" || strings.HasSuffix(p, string(filepath.Separator)) {
			return p
		}
		return p + "/"
	}
	switch m.form {
	case "home":
		if m.home != "" && (m.cwd == m.home || strings.HasPrefix(m.cwd, m.home+string(filepath.Separator))) {
			return withSlash("~" + filepath.ToSlash(strings.TrimPrefix(m.cwd, m.home)))
		}
	case "rel":
		if rel, err := filepath.Rel(m.prompt.Invoke, m.cwd); err == nil {
			if rel == "." {
				return ""
			}
			return withSlash(filepath.ToSlash(rel))
		}
	}
	return withSlash(filepath.ToSlash(m.cwd))
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
	items, _ := os.ReadDir(m.cwd)
	var dirs, files []pathEntry
	for _, item := range items {
		if isJunk(item.Name()) {
			continue
		}
		abs := filepath.Join(m.cwd, item.Name())
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
	if m.prompt.Type == "dir" && m.seg == "" {
		here := m.pickable(m.cwd, true)
		out = append(out, pathRow{pathEntry: pathEntry{name: ".", isDir: true, pickable: here}, here: true})
	}
	type scored struct {
		row   pathRow
		score int
	}
	var hits []scored
	for _, e := range m.entries() {
		score, positions, ok := fuzzyMatch(m.seg, e.name)
		if ok {
			hits = append(hits, scored{pathRow{pathEntry: e, positions: positions}, score})
		}
	}
	if m.seg != "" {
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	}
	for _, h := range hits {
		out = append(out, h.row)
	}
	return out
}

// fuzzyMatch reports whether query's letters appear in target in order,
// with a score (higher is better) and the matched rune positions.
func fuzzyMatch(query, target string) (score int, positions []int, ok bool) {
	if query == "" {
		return 0, nil, true
	}
	q := []rune(strings.ToLower(query))
	t := []rune(strings.ToLower(target))
	qi, last := 0, -2
	for ti := 0; ti < len(t) && qi < len(q); ti++ {
		if t[ti] != q[qi] {
			continue
		}
		positions = append(positions, ti)
		if ti == last+1 {
			score += 5
		}
		if ti == 0 || strings.ContainsRune("_-.", t[ti-1]) {
			score += 3
		}
		score++
		last = ti
		qi++
	}
	if qi < len(q) {
		return 0, nil, false
	}
	return score*100 - len(t), positions, true
}

func highlightMatch(text string, positions []int) string {
	if len(positions) == 0 {
		return text
	}
	set := map[int]bool{}
	for _, p := range positions {
		set[p] = true
	}
	var b strings.Builder
	for i, r := range []rune(text) {
		if set[i] {
			b.WriteString(sPathMatch.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
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
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+pathListHeight {
		m.offset = m.cursor - pathListHeight + 1
	}
}

func (m *pathModel) setSegment(seg string) {
	m.seg = seg
	m.cursor, m.offset = 0, 0
}

// enter moves the listing into dir, keeping the written form.
func (m *pathModel) enter(dir string) {
	m.cwd = filepath.Clean(dir)
	m.setSegment("")
}

func (m *pathModel) up() {
	prev := m.cwd
	m.enter(filepath.Dir(m.cwd))
	for i, row := range m.rows() {
		if filepath.Join(m.cwd, row.name) == prev {
			m.cursor = i
		}
	}
	m.clamp()
}

// slash turns the segment into a directory: an exact name if there is one,
// else the highlighted directory.
func (m *pathModel) slash() {
	switch m.seg {
	case "":
		if m.dirText() == "" {
			m.form = "abs"
			m.enter(string(filepath.Separator))
		}
		return
	case "~":
		m.form = "home"
		m.enter(m.home)
		return
	case ".":
		m.setSegment("")
		return
	case "..":
		m.up()
		return
	}
	for _, e := range m.entries() {
		if e.isDir && e.name == m.seg {
			m.enter(filepath.Join(m.cwd, e.name))
			return
		}
	}
	if row, ok := m.highlighted(); ok && row.isDir && !row.here {
		m.enter(filepath.Join(m.cwd, row.name))
		return
	}
	m.errMsg = `no directory matching "` + m.seg + `" here`
}

func (m pathModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.enter(filepath.Join(m.cwd, row.name))
		}
	case "backspace":
		if m.seg == "" {
			m.up()
		} else {
			runes := []rune(m.seg)
			m.setSegment(string(runes[:len(runes)-1]))
		}
	case "ctrl+u":
		m.setSegment("")
	case "esc":
		switch {
		case m.seg != "":
			m.setSegment("")
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
			m.setSegment(row.name)
		}
	case "enter":
		switch {
		case m.prompt.Multi && len(m.picks) > 0:
			m.done = true
			return m, tea.Quit
		case !hasRow:
			m.errMsg = "nothing matches"
		case row.isDir && !row.here && (m.prompt.Type == "file" || !row.pickable):
			m.enter(filepath.Join(m.cwd, row.name))
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
			for _, r := range text {
				if r == '/' {
					m.slash()
				} else {
					m.setSegment(m.seg + string(r))
				}
			}
		}
	}
	m.clamp()
	return m, nil
}

func (m pathModel) rowPath(row pathRow) string {
	if row.here {
		return m.cwd
	}
	return filepath.Join(m.cwd, row.name)
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

func (m pathModel) View() string {
	if m.done {
		return ""
	}
	noun := "a file"
	if m.prompt.Type == "dir" {
		noun = "a directory"
	}
	var b strings.Builder
	b.WriteString("\nPick " + noun + " for " + SLabel.Render(m.prompt.VarName) + ".")
	if m.prompt.Info != "" {
		b.WriteString("\n" + SInfo.Render(m.prompt.Info))
	}
	if len(m.prompt.Patterns) > 0 {
		b.WriteString("\n" + SInfo.Render("match: "+strings.Join(m.prompt.Patterns, ", ")))
	}
	b.WriteString("\n")
	if len(m.picks) > 0 {
		b.WriteString(SInfo.Render(fmt.Sprintf("picked (%d):", len(m.picks))) + "\n")
		for _, p := range m.picks {
			b.WriteString("  " + SChecked.Render("◉ ") + p + "\n")
		}
	}
	dir := SInfo.Render("./")
	if text := m.dirText(); text != "" {
		dir = sPathDir.Render(text)
	}
	b.WriteString(SArrow.Render("› ") + dir + m.seg + sPathMatch.Render("▏") + "\n")

	rows := m.rows()
	if len(rows) == 0 {
		b.WriteString(SInfo.Render("  nothing matches") + "\n")
	}
	end := min(m.offset+pathListHeight, len(rows))
	if m.offset > 0 {
		b.WriteString(SInfo.Render(fmt.Sprintf("  ↑ %d more", m.offset)) + "\n")
	}
	for i := m.offset; i < end; i++ {
		row := rows[i]
		if i == m.cursor {
			b.WriteString(SArrow.Render("▶ "))
		} else {
			b.WriteString("  ")
		}
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
		b.WriteString("\n")
	}
	if end < len(rows) {
		b.WriteString(SInfo.Render(fmt.Sprintf("  ↓ %d more", len(rows)-end)) + "\n")
	}
	if m.errMsg != "" {
		b.WriteString(SError.Render(m.errMsg) + "\n")
	}
	hint := "type to filter  / enter dir  ↑↓ move  ← up  → open  tab complete  enter pick"
	if m.prompt.Multi {
		hint = "type to filter  / enter dir  ↑↓ move  ← up  → open  tab toggle pick  enter confirm"
	}
	if m.prompt.Optional {
		hint += "  esc skip"
	}
	b.WriteString(SHint.Render(hint))
	return b.String()
}

// PromptPath runs the picker and returns the picked absolute path.
// With Multi the value is a list of absolute paths. answered is false when an
// optional picker was escaped: there is no answer and nothing is printed.
func PromptPath(ctx context.Context, prompt PathPrompt, task string, secret bool) (val value.Value, answered bool, err error) {
	result, err := tea.NewProgram(newPathModel(prompt), tea.WithContext(ctx)).Run()
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
	printGetLine(task, prompt.VarName, pathDisplay(final.picked, final.picks, prompt.Multi, secret))
	return val, true, nil
}

// pathDisplay is what the get: line shows for a pick: the path, the list
// joined, or the mask for a secret.
func pathDisplay(picked string, picks []string, multi, secret bool) string {
	switch {
	case secret:
		return SecretMask
	case multi:
		return strings.Join(picks, ", ")
	}
	return picked
}
