package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
	"hobnob/internal/value"

	"github.com/charmbracelet/lipgloss"
	cterm "github.com/charmbracelet/x/term"
)

// visibleTaskNames returns cfg's non-internal, non-hidden task names, sorted.
func visibleTaskNames(cfg *config.ConfigFile) []string {
	sortedNames := make([]string, len(cfg.TaskNames))
	copy(sortedNames, cfg.TaskNames)
	sort.Strings(sortedNames)

	var names []string
	for _, name := range sortedNames {
		if strings.HasPrefix(name, "_") {
			continue
		}
		if cfg.Tasks[name].Hidden {
			continue
		}
		names = append(names, name)
	}
	return names
}

func CollectSelectableTasks(cfg *config.ConfigFile, scope *scope.Scope) []tui.TaskItem {
	var tasks []tui.TaskItem
	for _, name := range visibleTaskNames(cfg) {
		info := listRenderInfo(cfg.Tasks[name].Info, scope.Vars())
		tasks = append(tasks, tui.TaskItem{Name: name, Info: info})
	}
	return tasks
}

// groupIndent is how far a grouped task sits right of the group heading.
const groupIndent = 2

// taskRow is one --list/--help row: a task plus its rendered info line.
type taskRow struct {
	name string
	info string
}

// buildTaskRows returns the visible (non-internal, non-hidden) tasks in
// sorted order, alongside the longest task name — callers need that width to
// align the info column before any rendering happens.
func buildTaskRows(cfg *config.ConfigFile, scope *scope.Scope) (rows []taskRow, maxNameLen int) {
	for _, name := range visibleTaskNames(cfg) {
		info := listRenderInfo(cfg.Tasks[name].Info, scope.Vars())
		rows = append(rows, taskRow{name, info})
		if width := lipgloss.Width(name); width > maxNameLen {
			maxNameLen = width
		}
	}
	return rows, maxNameLen
}

// ListTasks prints the visible tasks grouped by the part of their name before
// the first colon; tasks without a colon come last, ungrouped.
func ListTasks(cfg *config.ConfigFile, scope *scope.Scope, out io.Writer) error {
	return listTasks(cfg, scope, out, "")
}

// listTasks is ListTasks with every line indented, for --help's Tasks section.
func listTasks(cfg *config.ConfigFile, scope *scope.Scope, out io.Writer, indent string) error {
	rows, maxTaskLen := buildTaskRows(cfg, scope)

	if len(rows) == 0 {
		fmt.Fprintln(out, "No tasks found. To get started, create tasks in a hobnob.yml file.")
		fmt.Fprintln(out, "See the guide at: "+GuideURL)
		return nil
	}

	groups, groupNames, ungrouped := groupTaskRows(rows)
	termWidth := listTermWidth()
	heading := tui.SBold

	wroteBlock := false
	for _, group := range groupNames {
		if wroteBlock {
			fmt.Fprintln(out)
		}
		wroteBlock = true
		fmt.Fprintln(out, indent+heading.Render(group))
		for _, row := range groups[group] {
			printListRow(out, indent+strings.Repeat(" ", groupIndent), row, maxTaskLen, termWidth, tui.TaskStyle(group))
		}
	}
	if len(ungrouped) > 0 && wroteBlock {
		fmt.Fprintln(out)
	}
	ungroupedWidth := maxTaskLen
	if len(groupNames) > 0 {
		ungroupedWidth += groupIndent
	}
	for _, row := range ungrouped {
		printListRow(out, indent, row, ungroupedWidth, termWidth, tui.TaskStyle(row.name))
	}
	return nil
}

// groupTaskRows splits rows (already sorted) by the part of the name before
// the first colon. Names without a colon are returned as ungrouped.
func groupTaskRows(rows []taskRow) (groups map[string][]taskRow, groupNames []string, ungrouped []taskRow) {
	groups = map[string][]taskRow{}
	for _, row := range rows {
		group, _, grouped := strings.Cut(row.name, ":")
		if !grouped {
			ungrouped = append(ungrouped, row)
			continue
		}
		if _, seen := groups[group]; !seen {
			groupNames = append(groupNames, group)
		}
		groups[group] = append(groups[group], row)
	}
	return groups, groupNames, ungrouped
}

// printListRow writes one "<indent><name><pad>  <info>" row to out in nameStyle,
// word-wrapping the grey info to fit termWidth and indenting continuation
// lines under the info column.
func printListRow(out io.Writer, indent string, row taskRow, nameWidth, termWidth int, nameStyle lipgloss.Style) {
	name := nameStyle.Render(row.name)
	if row.info == "" {
		fmt.Fprintf(out, "%s%s\n", indent, name)
		return
	}
	pad := strings.Repeat(" ", nameWidth-lipgloss.Width(row.name))
	infoCol := lipgloss.Width(indent) + nameWidth + 2
	continuation := strings.Repeat(" ", infoCol)
	infoLines := listWordWrap(row.info, termWidth-infoCol)
	fmt.Fprintf(out, "%s%s%s  %s\n", indent, name, pad, tui.SInfo.Render(infoLines[0]))
	for _, cont := range infoLines[1:] {
		fmt.Fprintf(out, "%s%s\n", continuation, tui.SInfo.Render(cont))
	}
}

func listRenderInfo(tmpl string, scope map[string]value.Value) string {
	if tmpl == "" {
		return ""
	}
	rendered, err := eval.EvalTemplate(tmpl, scope)
	// At --list time, runtime vars aren't in scope yet, so any {{.VAR}} ref
	// produces <no value>. Hiding the info line is better than showing that.
	if err != nil || strings.Contains(rendered, "<no value>") {
		return ""
	}
	return rendered
}

func listTermWidth() int {
	if w, _, err := cterm.GetSize(os.Stdout.Fd()); err == nil && w > 0 {
		return w
	}
	return 80
}

func listWordWrap(text string, width int) []string {
	if width <= 0 {
		width = 40
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(line)+1+lipgloss.Width(word) <= width {
			line += " " + word
		} else {
			lines = append(lines, line)
			line = word
		}
	}
	return append(lines, line)
}
