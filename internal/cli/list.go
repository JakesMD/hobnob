package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/tui"

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

func CollectSelectableTasks(cfg *config.ConfigFile, scope *Scope) []tui.TaskItem {
	var tasks []tui.TaskItem
	for _, name := range visibleTaskNames(cfg) {
		info := listRenderInfo(cfg.Tasks[name].Info, scope.Vars)
		tasks = append(tasks, tui.TaskItem{Name: name, Info: info})
	}
	return tasks
}

// taskRow is one --list/--help row: a task plus its rendered info line and
// the get: params it prompts for.
type taskRow struct {
	name   string
	info   string
	params []config.GetEntry
}

// buildTaskRows returns the visible (non-internal, non-hidden) tasks in
// sorted order, alongside the longest task name — callers need that width to
// align the info column before any rendering happens.
func buildTaskRows(cfg *config.ConfigFile, scope *Scope) (rows []taskRow, maxNameLen int) {
	for _, name := range visibleTaskNames(cfg) {
		task := cfg.Tasks[name]
		info := listRenderInfo(task.Info, scope.Vars)
		params := config.CollectGetParams(task.Steps, cfg)
		rows = append(rows, taskRow{name, info, params})
		if width := lipgloss.Width(name); width > maxNameLen {
			maxNameLen = width
		}
	}
	return rows, maxNameLen
}

func ListTasks(cfg *config.ConfigFile, scope *Scope, out io.Writer) error {
	rows, maxTaskLen := buildTaskRows(cfg, scope)

	if len(rows) == 0 {
		fmt.Fprintln(out, "No tasks found. To get started, create tasks in a hobnob.yml file.")
		fmt.Fprintln(out, "See the guide at: https://github.com/jakesmd/hobnob/blob/main/GUIDE.md")
		return nil
	}

	termWidth := listTermWidth()

	fmt.Fprintln(out, "Available tasks for this project:")

	identity := func(value string) string { return value }
	label := func(value string) string { return tui.SLabel.Render(value) }
	info := func(value string) string { return tui.SInfo.Render(value) }
	for _, row := range rows {
		printListRow(out, "• ", 2, row.name, maxTaskLen, row.info, termWidth, label, identity)
		printTaskParams(out, row.params, scope, termWidth, info)
	}

	return nil
}

// printTaskParams renders one task's get: params, indented under its row.
func printTaskParams(out io.Writer, params []config.GetEntry, scope *Scope, termWidth int, renderInfo func(string) string) {
	maxParamLen := 0
	for _, param := range params {
		width := lipgloss.Width(param.VarName)
		if param.DefaultTmpl != "" {
			width += 2 // parens
		}
		if width > maxParamLen {
			maxParamLen = width
		}
	}

	for _, param := range params {
		paramInfo := listRenderInfo(param.Info, scope.Vars)
		displayName := param.VarName
		if param.DefaultTmpl != "" {
			displayName = "(" + param.VarName + ")"
		}
		printListRow(out, "    • ", 6, displayName, maxParamLen, paramInfo, termWidth, renderInfo, renderInfo)
	}
}

// printListRow writes one "<bullet><label><pad>  <info>" row to out, word-wrapping
// info to fit termWidth and indenting continuation lines under the info column.
// bulletWidth is the bullet's visual column width (e.g. "• " is 2 columns
// even though the bullet rune itself is multiple bytes) — used for column
// math, not string length.
func printListRow(out io.Writer, bullet string, bulletWidth int, label string, labelWidth int, info string, termWidth int, renderLabel, renderInfo func(string) string) {
	if info == "" {
		fmt.Fprintf(out, "%s%s\n", bullet, renderLabel(label))
		return
	}
	pad := strings.Repeat(" ", labelWidth-lipgloss.Width(label))
	infoCol := bulletWidth + labelWidth + 2
	indent := strings.Repeat(" ", infoCol)
	infoLines := listWordWrap(info, termWidth-infoCol)
	fmt.Fprintf(out, "%s%s%s  %s\n", bullet, renderLabel(label), pad, renderInfo(infoLines[0]))
	for _, cont := range infoLines[1:] {
		fmt.Fprintf(out, "%s%s\n", indent, renderInfo(cont))
	}
}

func listRenderInfo(tmpl string, scope map[string]string) string {
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
