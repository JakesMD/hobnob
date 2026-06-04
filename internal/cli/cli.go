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
  local cur="${words[CURRENT]}"
  local prev="${words[CURRENT-1]}"

  if [[ "$prev" == "--file" ]]; then
    _files
    return
  fi

  if [[ "$cur" == "--file="* ]]; then
    compset -P '--file='
    _files
    return
  fi

  if [[ "$cur" == --* ]]; then
    compadd -- --file --list --help --no-input --version --upgrade
    return
  fi

  local file_arg="" positional=0 i
  for (( i=2; i<CURRENT; i++ )); do
    if [[ "${words[i]}" == "--file" ]]; then
      file_arg="${words[i+1]}"
      (( i++ ))
    elif [[ "${words[i]}" != --* && "${words[i]}" != *=* ]]; then
      (( positional++ ))
    fi
  done

  if (( positional == 0 )); then
    local tasks
    if [[ -n "$file_arg" ]]; then
      tasks=(${(f)"$(hobnob --file "$file_arg" --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}')"})
    else
      tasks=(${(f)"$(hobnob --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}')"})
    fi
    compadd -a tasks
  fi
}
type compdef &>/dev/null || { autoload -Uz compinit && compinit; }
compdef _hobnob hobnob
`, nil
	case "bash":
		return `_hobnob_completion() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local prev="${COMP_WORDS[COMP_CWORD-1]}"

  if [[ "$prev" == "--file" ]]; then
    COMPREPLY=($(compgen -f -- "$cur"))
    return
  fi

  if [[ "$cur" == "--file="* ]]; then
    local val="${cur#--file=}"
    local files=($(compgen -f -- "$val"))
    COMPREPLY=("${files[@]/#/--file=}")
    return
  fi

  if [[ "$cur" == --* ]]; then
    COMPREPLY=($(compgen -W "--file --list --help --no-input --version --upgrade" -- "$cur"))
    return
  fi

  local file_arg="" positional=0 i
  for (( i=1; i<COMP_CWORD; i++ )); do
    if [[ "${COMP_WORDS[i]}" == "--file" ]]; then
      file_arg="${COMP_WORDS[i+1]}"
      (( i++ ))
    elif [[ "${COMP_WORDS[i]}" != --* && "${COMP_WORDS[i]}" != *=* ]]; then
      (( positional++ ))
    fi
  done

  if [[ "$positional" -eq 0 ]]; then
    local tasks
    if [[ -n "$file_arg" ]]; then
      tasks=$(hobnob --file "$file_arg" --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}')
    else
      tasks=$(hobnob --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}')
    fi
    COMPREPLY=($(compgen -W "${tasks}" -- "${cur}"))
  fi
}
complete -F _hobnob_completion hobnob
`, nil
	case "fish":
		return `function __fish_hobnob_file_value
    set -l cmd (commandline -opc)
    for i in (seq 2 (count $cmd))
        if test "$cmd[$i]" = "--file"; and test (math $i + 1) -le (count $cmd)
            echo $cmd[(math $i + 1)]
            return
        else if string match -qr '^--file=(.+)' "$cmd[$i]"
            string replace --regex '^--file=' '' "$cmd[$i]"
            return
        end
    end
end
function __fish_hobnob_no_task_given
    set -l cmd (commandline -opc)
    set -l positional 0
    set -l i 2
    while test $i -le (count $cmd)
        if test "$cmd[$i]" = "--file"
            set i (math $i + 2)
        else if not string match -qr '^--' "$cmd[$i]"; and not string match -qr '=' "$cmd[$i]"
            set positional (math $positional + 1)
            set i (math $i + 1)
        else
            set i (math $i + 1)
        end
    end
    test $positional -eq 0
end
function __fish_hobnob_tasks
    set -l f (__fish_hobnob_file_value)
    if test -n "$f"
        hobnob --file "$f" --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}'
    else
        hobnob --list 2>/dev/null | awk 'NR>1 && !/^[[:space:]]/{print $2}'
    end
end
complete -c hobnob -l file -r -d 'Hobnob file to use'
complete -c hobnob -f -l list -d 'List all available tasks'
complete -c hobnob -f -l help -d 'Show help'
complete -c hobnob -f -l no-input -d 'Skip interactive prompts'
complete -c hobnob -f -l version -d 'Print version and exit'
complete -c hobnob -f -l upgrade -d 'Upgrade to latest release'
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
