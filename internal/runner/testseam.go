package runner

import (
	"context"
	"time"

	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// TextPromptFunc matches tui.PromptText's signature.
type TextPromptFunc func(ctx context.Context, info, varName string,
	validate func(string) (bool, error), checkDesc, defaultVal, task string,
	secret, optional bool) (string, error)

// SelectPromptFunc matches tui.PromptSelect's signature.
type SelectPromptFunc func(ctx context.Context, varName, info string, items []string,
	multi bool, defaultVal, task string, secret, optional bool,
	validate func(value.Value) (bool, error), checkDesc string) (value.Value, error)

// PathPromptFunc matches tui.PromptPath's signature.
type PathPromptFunc func(ctx context.Context, prompt tui.PathPrompt, task string,
	secret bool) (val value.Value, answered bool, err error)

// SetPrompts substitutes the interactive prompt implementations and returns a
// func that restores the originals. Test-only seam — production code always
// calls the real tui prompts and never calls this.
func SetPrompts(text TextPromptFunc, sel SelectPromptFunc, path PathPromptFunc) (restore func()) {
	origText, origSel, origPath := promptTextFn, promptSelectFn, promptPathFn
	promptTextFn, promptSelectFn, promptPathFn = text, sel, path
	return func() {
		promptTextFn, promptSelectFn, promptPathFn = origText, origSel, origPath
	}
}

// SetLeftoverGrace shortens how long a run: step's leftover processes get
// after SIGTERM before they're killed, and returns a func that restores it.
// Test-only seam.
func SetLeftoverGrace(d time.Duration) (restore func()) {
	orig := leftoverGrace
	leftoverGrace = d
	return func() { leftoverGrace = orig }
}
