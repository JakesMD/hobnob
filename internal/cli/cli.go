package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/tui"

	cterm "github.com/charmbracelet/x/term"
)

type LineWriter struct {
	w      io.Writer
	prefix string
	buf    bytes.Buffer
}

func NewLineWriter(w io.Writer, prefix string) *LineWriter {
	return &LineWriter{w: w, prefix: prefix}
}

func (lw *LineWriter) Write(p []byte) (int, error) {
	n := len(p)
	for _, b := range p {
		if b == '\n' {
			lw.w.Write([]byte(lw.prefix))
			lw.w.Write(lw.buf.Bytes())
			lw.w.Write([]byte{'\n'})
			lw.buf.Reset()
		} else {
			lw.buf.WriteByte(b)
		}
	}
	return n, nil
}

func (lw *LineWriter) Flush() {
	if lw.buf.Len() > 0 {
		lw.w.Write([]byte(lw.prefix))
		lw.w.Write(lw.buf.Bytes())
		lw.w.Write([]byte{'\n'})
		lw.buf.Reset()
	}
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
// then system vars (HOBNOB_FILE_DIR, HOBNOB_INVOCATION_DIR), then global vars
// evaluated on top, then CLI KEY=VALUE args (highest priority).
// Also returns a secrets map for any global vars marked secret: true.
func BuildScope(vars []config.SetEntry, cliVars map[string]string, taskfileDir, invocationDir string) (map[string]string, map[string]bool, error) {
	scope := make(map[string]string)
	secrets := make(map[string]bool)

	for _, e := range os.Environ() {
		idx := strings.IndexByte(e, '=')
		if idx > 0 {
			scope[e[:idx]] = e[idx+1:]
		}
	}

	scope["HOBNOB_FILE_DIR"] = taskfileDir
	scope["HOBNOB_INVOCATION_DIR"] = invocationDir

	for _, e := range vars {
		val, err := eval.EvalTemplate(e.ValTmpl, scope)
		if err != nil {
			return nil, nil, fmt.Errorf("global var %q: %w", e.Key, err)
		}
		scope[e.Key] = val
		if e.Secret {
			secrets[e.Key] = true
		}
	}

	for k, v := range cliVars {
		scope[k] = v
	}

	return scope, secrets, nil
}

func RunDisplayLines(cmd, task string) []string {
	prefix := tui.TaskPrefix(task)
	lines := strings.Split(cmd, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		if i == 0 {
			result[i] = tui.SStep.Render("run:") + " " + prefix + tui.SStep.Render(line)
		} else {
			result[i] = tui.SStep.Render(line)
		}
	}
	return result
}

func CollectGetParams(steps []config.Step, cfg *config.ConfigFile) []config.GetEntry {
	return collectGetParams(steps, cfg, make(map[string]bool), make(map[string]bool))
}

func collectGetParams(steps []config.Step, cfg *config.ConfigFile, visited map[string]bool, preset map[string]bool) []config.GetEntry {
	var entries []config.GetEntry
	alreadySet := make(map[string]bool, len(preset))
	for k := range preset {
		alreadySet[k] = true
	}
	for _, s := range steps {
		switch s.Kind {
		case config.KindSet:
			for _, e := range s.SetEntries {
				alreadySet[e.Key] = true
			}
		case config.KindGet:
			for _, e := range s.GetEntries {
				if !alreadySet[e.VarName] {
					entries = append(entries, e)
				}
				alreadySet[e.VarName] = true
			}
		case config.KindRun:
			for _, e := range s.IntoEntries {
				alreadySet[e.ParentKey] = true
			}
		case config.KindFor:
			loopPreset := make(map[string]bool, len(alreadySet))
			for k := range alreadySet {
				loopPreset[k] = true
			}
			if len(s.ForMatrix) > 0 {
				for _, m := range s.ForMatrix {
					loopPreset[m.VarName] = true
				}
			} else if s.ForVar != "" {
				loopPreset[s.ForVar] = true
			} else {
				loopPreset["ITEM"] = true
			}
			entries = append(entries, collectGetParams(s.ForSteps, cfg, visited, loopPreset)...)
		case config.KindCall:
			if cfg != nil && !strings.Contains(s.CallTarget, "{{") {
				if !visited[s.CallTarget] {
					childPreset := make(map[string]bool, len(alreadySet)+len(s.CallVars))
					for k := range alreadySet {
						childPreset[k] = true
					}
					for _, w := range s.CallVars {
						childPreset[w.Key] = true
					}
					visited[s.CallTarget] = true
					if t, ok := cfg.Tasks[s.CallTarget]; ok {
						for _, e := range collectGetParams(t.Steps, cfg, visited, childPreset) {
							if !alreadySet[e.VarName] {
								entries = append(entries, e)
							}
							alreadySet[e.VarName] = true
						}
					}
					delete(visited, s.CallTarget)
				}
				for _, e := range s.IntoEntries {
					alreadySet[e.ParentKey] = true
				}
			}
		}
	}
	return entries
}

func ListTasks(cfg *config.ConfigFile, scope map[string]string, w io.Writer) error {
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
		info := listRenderInfo(t.Info, scope)
		params := CollectGetParams(t.Steps, cfg)
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
			info := listRenderInfo(p.Info, scope)
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
