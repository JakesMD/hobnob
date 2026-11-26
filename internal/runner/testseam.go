package runner

import (
	"time"
)

// SetPrompts substitutes the terminal prompt implementations that
// TerminalPrompts returns, and returns a func that restores them. Test-only
// seam: a caller handing ExecuteTask its own Prompts is unaffected.
func SetPrompts(text TextPromptFunc, sel SelectPromptFunc, path PathPromptFunc, say SayPromptFunc) (restore func()) {
	orig := terminalPrompts
	terminalPrompts = Prompts{Text: text, Select: sel, Path: path, Say: say}
	return func() { terminalPrompts = orig }
}

// SetLeftoverGrace shortens how long a run: step's leftover processes get
// after SIGTERM before they're killed, and returns a func that restores it.
// Test-only seam.
func SetLeftoverGrace(d time.Duration) (restore func()) {
	orig := leftoverGrace
	leftoverGrace = d
	return func() { leftoverGrace = orig }
}
