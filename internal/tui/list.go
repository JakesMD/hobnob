package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// menuListHeight is how many rows a menu shows before it scrolls.
const menuListHeight = 10

// listRow is one item that survived the filter: its index in the full list
// and which runes matched.
type listRow struct {
	index     int
	positions []int
}

// matchRows keeps the items query fuzzy-matches, best first. An empty query
// keeps every item in its own order.
func matchRows(items []string, query string) []listRow {
	type scored struct {
		row   listRow
		score int
	}
	var hits []scored
	for i, item := range items {
		score, positions, ok := fuzzyMatch(query, item)
		if ok {
			hits = append(hits, scored{listRow{i, positions}, score})
		}
	}
	if query != "" {
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	}
	rows := make([]listRow, len(hits))
	for i, h := range hits {
		rows[i] = h.row
	}
	return rows
}

// listState is the filter text and cursor shared by every filtered menu.
type listState struct {
	filter string
	cursor int // position in the filtered rows
	offset int // first visible filtered row
}

func (s *listState) setFilter(filter string) {
	s.filter = filter
	s.cursor, s.offset = 0, 0
}

func (s *listState) moveTo(cursor, total int) {
	s.cursor = max(0, min(cursor, total-1))
	s.offset = scrollTo(s.cursor, s.offset, menuListHeight)
}

// handleKey applies the keys every filtered menu shares: up, down, backspace,
// space and typed runes. It reports whether the key changed the list state;
// total is only read for a cursor move.
func (s *listState) handleKey(key tea.KeyMsg, total func() int) bool {
	switch key.Type {
	case tea.KeyUp:
		s.moveTo(s.cursor-1, total())
	case tea.KeyDown:
		s.moveTo(s.cursor+1, total())
	case tea.KeyBackspace:
		runes := []rune(s.filter)
		if len(runes) == 0 {
			return false
		}
		s.setFilter(string(runes[:len(runes)-1]))
	case tea.KeySpace:
		if s.filter == "" {
			return false
		}
		s.setFilter(s.filter + " ")
	case tea.KeyRunes:
		s.setFilter(s.filter + string(key.Runes))
	default:
		return false
	}
	return true
}

// body is the filter line (once there is filter text) above the scrolling
// window of total rows.
func (s listState) body(innerWidth, total int, row func(i int) string) string {
	var lines []string
	if s.filter != "" {
		lines = append(lines, filterLine(s.filter, innerWidth)...)
	}
	lines = append(lines, renderWindow(total, s.offset, menuListHeight, row)...)
	return strings.Join(lines, "\n")
}

// cursorPrefix is the "▶ " marker on the highlighted row and its blank twin
// on every other.
func cursorPrefix(highlighted bool) string {
	if highlighted {
		return SArrow.Render("▶ ")
	}
	return "  "
}

// scrollTo returns the offset that keeps cursor inside a window of height rows.
func scrollTo(cursor, offset, height int) int {
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+height {
		return cursor - height + 1
	}
	return offset
}

// renderWindow draws rows [offset, offset+height) of total, framed by dim
// "↑ n more" / "↓ n more" lines, or "nothing matches" when total is 0.
func renderWindow(total, offset, height int, row func(i int) string) []string {
	if total == 0 {
		return []string{SInfo.Render("  nothing matches")}
	}
	end := min(offset+height, total)
	var lines []string
	if offset > 0 {
		lines = append(lines, SInfo.Render(fmt.Sprintf("  ↑ %d more", offset)))
	}
	for i := offset; i < end; i++ {
		lines = append(lines, row(i))
	}
	if end < total {
		lines = append(lines, SInfo.Render(fmt.Sprintf("  ↓ %d more", total-end)))
	}
	return lines
}

// filterLine is the "› text▏" line and the rule under it.
func filterLine(text string, ruleWidth int) []string {
	return []string{
		SArrow.Render("› ") + sPathInput.Render(text) + sPathMatch.Render("▏"),
		SInfo.Render(strings.Repeat("─", ruleWidth)),
	}
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
