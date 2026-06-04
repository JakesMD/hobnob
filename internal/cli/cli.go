package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/tui"

	cterm "github.com/charmbracelet/x/term"
)

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
		return `_hobnob() {
  if (( CURRENT == 2 )); then
    local tasks
    tasks=(${(f)"$(hobnob --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}')"})
    compadd -a tasks
  fi
}
type compdef &>/dev/null || { autoload -Uz compinit && compinit; }
compdef _hobnob hobnob
`, nil
	case "bash":
		return `_hobnob_completion() {
  local cur tasks
  cur="${COMP_WORDS[COMP_CWORD]}"
  if [[ "${COMP_CWORD}" -eq 1 ]]; then
    tasks=$(hobnob --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}')
    COMPREPLY=($(compgen -W "${tasks}" -- "${cur}"))
  fi
}
complete -F _hobnob_completion hobnob
`, nil
	case "fish":
		return `function __fish_hobnob_no_task_given
    test (count (commandline -opc)) -eq 1
end
function __fish_hobnob_tasks
    hobnob --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}'
end
complete -c hobnob -f -n "__fish_hobnob_no_task_given" -a "(__fish_hobnob_tasks)"
`, nil
	default:
		return "", fmt.Errorf("unknown shell %q: supported shells are bash, zsh, fish", shell)
	}
}

// BuildScope constructs the initial variable scope: env vars as the base,
// then system vars (HOBNOB_FILE_DIR, HOBNOB_INVOCATION_DIR), then CLI KEY=VALUE
// args, then global vars evaluated on top (highest priority).
// Globals win over CLI args because vars: is implementation detail — the public
// API for caller input is get: steps, which are skipped when a var is already set.
// Also returns a secrets map for any global vars marked secret: true.
func BuildScope(vars []config.SetEntry, cliVars map[string]string, taskfileDir, invocationDir string) (*Scope, error) {
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
  hobnob [--file <path>] --help
  hobnob --version
  hobnob --upgrade

Flags:
  --file <path>   Hobnob file to use instead of auto-discovery
  --list          List all available tasks
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

func ListTasks(cfg *config.ConfigFile, scope *Scope, w io.Writer) error {
	type row struct {
		name   string
		info   string
		params []config.GetEntry
	}

	var rows []row
	maxTaskLen := 0
	for _, name := range cfg.TaskNames {
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

	tw := listTermWidth()
	// "• " (2) + name (maxTaskLen) + "  " (2)
	taskInfoCol := 2 + maxTaskLen + 2
	taskInfoIndent := strings.Repeat(" ", taskInfoCol)

	fmt.Fprintln(w, "Available tasks for this project:")

	for _, r := range rows {
		namePad := strings.Repeat(" ", maxTaskLen-len(r.name))
		if r.info != "" {
			infoLines := listWordWrap(r.info, tw-taskInfoCol)
			fmt.Fprintf(w, "• %s%s  %s\n", tui.SLabel.Render(r.name), namePad, infoLines[0])
			for _, cont := range infoLines[1:] {
				fmt.Fprintf(w, "%s%s\n", taskInfoIndent, cont)
			}
		} else {
			fmt.Fprintf(w, "• %s\n", tui.SLabel.Render(r.name))
		}

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
		// "    • " (6) + name (maxParamLen) + "  " (2)
		paramInfoCol := 6 + maxParamLen + 2
		paramInfoIndent := strings.Repeat(" ", paramInfoCol)

		for _, p := range r.params {
			info := listRenderInfo(p.Info, scope.Vars)
			displayName := p.VarName
			if p.DefaultTmpl != "" {
				displayName = "(" + p.VarName + ")"
			}
			paramPad := strings.Repeat(" ", maxParamLen-len(displayName))
			if info != "" {
				infoLines := listWordWrap(info, tw-paramInfoCol)
				fmt.Fprintf(w, "    • %s%s  %s\n",
					tui.SInfo.Render(displayName), paramPad,
					tui.SInfo.Render(infoLines[0]))
				for _, cont := range infoLines[1:] {
					fmt.Fprintf(w, "%s%s\n", paramInfoIndent, tui.SInfo.Render(cont))
				}
			} else {
				fmt.Fprintf(w, "    • %s\n", tui.SInfo.Render(displayName))
			}
		}
	}

	return nil
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
