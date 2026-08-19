package runner

import (
	"context"

	"hobnob/internal/value"
)

// TextPromptFunc matches tui.PromptText's signature.
type TextPromptFunc func(ctx context.Context, info, varName string,
	validate func(string) (bool, error), checkDesc, defaultVal, task string,
	secret, optional bool) (string, error)

// SelectPromptFunc matches tui.PromptSelect's signature.
type SelectPromptFunc func(ctx context.Context, varName, info string, items []string,
	multi bool, defaultVal, task string, secret bool) (value.Value, error)

// SetPrompts substitutes the interactive prompt implementations and returns a
// func that restores the originals. Test-only seam — production code always
// calls the real tui prompts and never calls this.
func SetPrompts(text TextPromptFunc, sel SelectPromptFunc) (restore func()) {
	origText, origSel := promptTextFn, promptSelectFn
	promptTextFn, promptSelectFn = text, sel
	return func() {
		promptTextFn, promptSelectFn = origText, origSel
	}
}
