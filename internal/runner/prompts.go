package runner

import (
	"context"

	"hobnob/internal/ask"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// AskFunc puts a text, menu or path question to the person and returns the value
// its judge accepted, or the empty value for a skipped optional entry.
type AskFunc func(ctx context.Context, question ask.Question) (value.Value, error)

// SayPromptFunc matches tui.PromptSay's signature.
type SayPromptFunc func(ctx context.Context, message, task string) error

// Prompts bundles the interactive prompt implementations a run asks its
// answers through.
type Prompts struct {
	Ask AskFunc
	Say SayPromptFunc
}

var terminalPrompts = Prompts{Ask: tui.PromptAsk, Say: tui.PromptSay}

// TerminalPrompts returns the bubbletea prompts that run in the terminal.
func TerminalPrompts() Prompts { return terminalPrompts }
