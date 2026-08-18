package cli

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/tui"

	cterm "github.com/charmbracelet/x/term"
)

//go:embed completions/hobnob.zsh
var zshCompletion string

//go:embed completions/hobnob.bash
var bashCompletion string

//go:embed completions/hobnob.fish
var fishCompletion string

type Scope struct {
	Vars    map[string]string
	Secrets map[string]bool
}

func (s *Scope) Copy() *Scope {
	vars := make(map[string]string, len(s.Vars))
	for k, v := range s.Vars {
		vars[k] = v
	}
	secrets := make(map[string]bool, len(s.Secrets))
	for k, v := range s.Secrets {
		secrets[k] = v
	}
	return &Scope{Vars: vars, Secrets: secrets}
}

func CompletionScript(shell string) (string, error) {
	switch shell {
	case "zsh":
		return zshCompletion, nil
	case "bash":
		return bashCompletion, nil
	case "fish":
		return fishCompletion, nil
	default:
		return "", fmt.Errorf("unknown shell %q: supported shells are bash, zsh, fish", shell)
	}
}

// BuildScope constructs the initial variable scope: env vars as the base,
// then system vars (HOBNOB_FILE_DIR, HOBNOB_INVOCATION_DIR), then vars sourced
// from env: files, then CLI KEY=VALUE args, then global vars evaluated on top
// (highest priority).
// CLI args win over env: files so a caller's explicit override always beats a
// sourced default. Globals win over CLI args because vars: is implementation
// detail — the public API for caller input is get: steps, which are skipped
// when a var is already set.
// Also returns a secrets map for any global vars marked secret: true, and for
// any var sourced from an env: file per its default/override (see config.LoadEnvFiles).
func BuildScope(vars []config.SetEntry, envFileEntries []config.EnvFileEntry, cliVars map[string]string, taskfileDir, invocationDir string) (*Scope, error) {
	s := &Scope{
		Vars:    make(map[string]string),
		Secrets: make(map[string]bool),
	}

	for _, e := range os.Environ() {
		idx := strings.IndexByte(e, '=')
		if idx > 0 {
			s.Vars[e[:idx]] = e[idx+1:]
		}
	}

	s.Vars["HOBNOB_FILE_DIR"] = taskfileDir
	s.Vars["HOBNOB_INVOCATION_DIR"] = invocationDir

	envFileVars, envFileSecrets, err := config.LoadEnvFiles(envFileEntries, taskfileDir, s.Vars)
	if err != nil {
		return nil, err
	}
	for k, v := range envFileVars {
		s.Vars[k] = v
		if envFileSecrets[k] {
			s.Secrets[k] = true
		}
	}

	for k, v := range cliVars {
		s.Vars[k] = v
	}

	for _, e := range vars {
		val, err := eval.EvalTemplate(e.ValTmpl, s.Vars)
		if err != nil {
			return nil, fmt.Errorf("global var %q: %w", e.Key, err)
		}
		s.Vars[e.Key] = val
		if e.Secret {
			s.Secrets[e.Key] = true
		}
	}

	return s, nil
}

func PrintUsage(w io.Writer, version string) {
	v := version
	if v == "" {
		v = "dev"
	}
	docsURL := "https://github.com/JakesMD/hobnob/blob/main/GUIDE.md"
	if version != "" {
		docsURL = "https://github.com/JakesMD/hobnob/blob/" + version + "/GUIDE.md"
	}
	fmt.Fprintf(w, `hobnob %s

Usage:
  hobnob [--file <path>] <task> [--no-input] [KEY=VALUE ...]
  hobnob [--file <path>] --list
  hobnob [--file <path>] --select
  hobnob [--file <path>] --help
  hobnob --version
  hobnob --upgrade

Flags:
  --file <path>   Hobnob file to use instead of auto-discovery
  --list          List all available tasks
  --select        Interactively select a task to run
  --help          Show this help
  --no-input      Skip interactive prompts; fail if a required variable is missing
  --version       Print version and exit
  --upgrade       Upgrade hobnob to the latest release

Docs:
  %s

`, v, docsURL)
}

func PrintHelp(cfg *config.ConfigFile, scope *Scope, w io.Writer, version string) error {
	PrintUsage(w, version)
	return ListTasks(cfg, scope, w)
}

func CollectSelectableTasks(cfg *config.ConfigFile, scope *Scope) []tui.TaskItem {
	sortedNames := make([]string, len(cfg.TaskNames))
	copy(sortedNames, cfg.TaskNames)
	sort.Strings(sortedNames)

	var tasks []tui.TaskItem
	for _, name := range sortedNames {
		if strings.HasPrefix(name, "_") {
			continue
		}
		t := cfg.Tasks[name]
		if t.Hidden {
			continue
		}
		info := listRenderInfo(t.Info, scope.Vars)
		tasks = append(tasks, tui.TaskItem{Name: name, Info: info})
	}
	return tasks
}

func ListTasks(cfg *config.ConfigFile, scope *Scope, w io.Writer) error {
	type row struct {
		name   string
		info   string
		params []config.GetEntry
	}

	sortedNames := make([]string, len(cfg.TaskNames))
	copy(sortedNames, cfg.TaskNames)
	sort.Strings(sortedNames)

	var rows []row
	maxTaskLen := 0
	for _, name := range sortedNames {
		if strings.HasPrefix(name, "_") {
			continue
		}
		t := cfg.Tasks[name]
		if t.Hidden {
			continue
		}
		info := listRenderInfo(t.Info, scope.Vars)
		params := config.CollectGetParams(t.Steps, cfg)
		rows = append(rows, row{name, info, params})
		if len(name) > maxTaskLen {
			maxTaskLen = len(name)
		}
	}

	if len(rows) == 0 {
		fmt.Fprintln(w, "No tasks found. To get started, create tasks in a hobnob.yml file.")
		fmt.Fprintln(w, "See the guide at: https://github.com/jakesmd/hobnob/blob/main/GUIDE.md")
		return nil
	}

	tw := listTermWidth()

	fmt.Fprintln(w, "Available tasks for this project:")

	identity := func(s string) string { return s }
	label := func(s string) string { return tui.SLabel.Render(s) }
	info := func(s string) string { return tui.SInfo.Render(s) }
	for _, r := range rows {
		printListRow(w, "• ", 2, r.name, maxTaskLen, r.info, tw, label, identity)

		maxParamLen := 0
		for _, p := range r.params {
			n := len(p.VarName)
			if p.DefaultTmpl != "" {
				n += 2 // parens
			}
			if n > maxParamLen {
				maxParamLen = n
			}
		}

		for _, p := range r.params {
			paramInfo := listRenderInfo(p.Info, scope.Vars)
			displayName := p.VarName
			if p.DefaultTmpl != "" {
				displayName = "(" + p.VarName + ")"
			}
			printListRow(w, "    • ", 6, displayName, maxParamLen, paramInfo, tw, info, info)
		}
	}

	return nil
}

// printListRow writes one "<bullet><label><pad>  <info>" row to w, word-wrapping
// info to fit tw and indenting continuation lines under the info column.
// bulletWidth is the bullet's visual column width (e.g. "• " is 2 columns
// even though the bullet rune itself is multiple bytes) — used for column
// math, not string length.
func printListRow(w io.Writer, bullet string, bulletWidth int, label string, labelWidth int, info string, tw int, renderLabel, renderInfo func(string) string) {
	if info == "" {
		fmt.Fprintf(w, "%s%s\n", bullet, renderLabel(label))
		return
	}
	pad := strings.Repeat(" ", labelWidth-len(label))
	infoCol := bulletWidth + labelWidth + 2
	indent := strings.Repeat(" ", infoCol)
	infoLines := listWordWrap(info, tw-infoCol)
	fmt.Fprintf(w, "%s%s%s  %s\n", bullet, renderLabel(label), pad, renderInfo(infoLines[0]))
	for _, cont := range infoLines[1:] {
		fmt.Fprintf(w, "%s%s\n", indent, renderInfo(cont))
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
		if len(line)+1+len(word) <= width {
			line += " " + word
		} else {
			lines = append(lines, line)
			line = word
		}
	}
	return append(lines, line)
}
