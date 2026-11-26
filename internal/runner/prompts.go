package runner

import (
	"context"

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

// SayPromptFunc matches tui.PromptSay's signature.
type SayPromptFunc func(ctx context.Context, message, task string) error

// Prompts bundles the interactive prompt implementations a run asks its
// answers through.
type Prompts struct {
	Text   TextPromptFunc
	Select SelectPromptFunc
	Path   PathPromptFunc
	Say    SayPromptFunc
}

var terminalPrompts = Prompts{Text: tui.PromptText, Select: tui.PromptSelect, Path: tui.PromptPath, Say: tui.PromptSay}

// TerminalPrompts returns the bubbletea prompts that run in the terminal.
func TerminalPrompts() Prompts { return terminalPrompts }
