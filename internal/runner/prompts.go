package runner

import (
	"context"

	"hobnob/internal/ask"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// AskFunc puts a text or menu question to the person and returns the value
// its judge accepted, or the empty value for a skipped optional entry.
type AskFunc func(ctx context.Context, question ask.Question) (value.Value, error)

// PathPromptFunc matches tui.PromptPath's signature.
type PathPromptFunc func(ctx context.Context, prompt tui.PathPrompt, task string,
	secret bool) (val value.Value, answered bool, err error)

// SayPromptFunc matches tui.PromptSay's signature.
type SayPromptFunc func(ctx context.Context, message, task string) error

// Prompts bundles the interactive prompt implementations a run asks its
// answers through.
type Prompts struct {
	Ask  AskFunc
	Path PathPromptFunc
	Say  SayPromptFunc
}

var terminalPrompts = Prompts{Ask: tui.PromptAsk, Path: tui.PromptPath, Say: tui.PromptSay}

// TerminalPrompts returns the bubbletea prompts that run in the terminal.
func TerminalPrompts() Prompts { return terminalPrompts }
