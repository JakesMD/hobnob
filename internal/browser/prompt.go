package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"hobnob/internal/ask"
	"hobnob/internal/value"
)

// Kinds of prompt a Snapshot can carry.
const (
	KindText = "text"
	// KindMenu offers options, one choice or several.
	KindMenu = "menu"
	// KindSay is a say: step's message: Info, with only Continue to answer it.
	KindSay = "say"
)

// maskedAnswer stands for a secret answer on the rail.
const maskedAnswer = "••••"

// Prompt is the question the page is asking. It is the part of the snapshot
// every prompt kind shares; a kind adds its own fields beside these.
type Prompt struct {
	// ID tells one asking from the next, so an answer to a prompt that has
	// since gone is refused rather than given to its successor.
	ID      int    `json:"id"`
	Kind    string `json:"kind"`
	VarName string `json:"varName"`
	Info    string `json:"info"`
	// Default is empty for a secret, which never reaches the page; HasDefault
	// says there is one.
	Default    string `json:"default,omitempty"`
	HasDefault bool   `json:"hasDefault,omitempty"`
	Secret     bool   `json:"secret,omitempty"`
	Optional   bool   `json:"optional,omitempty"`
	// Refusal is why the last answer was refused, empty when none was.
	Refusal string `json:"refusal,omitempty"`
	// Picker is the folder listing of a picker prompt.
	Picker *Picker `json:"picker,omitempty"`
	// Multi says several may be chosen, in a menu or a picker.
	Multi bool `json:"multi,omitempty"`
	// Options and Defaults belong to a menu: what it offers and the options
	// ticked to start with.
	Options  []string `json:"options,omitempty"`
	Defaults []string `json:"defaults,omitempty"`
}

// judge decides an answer's JSON value: a non-empty refusal keeps the prompt
// open and says why, otherwise the answer is taken and entry is its rail line.
// skip says the answer is no answer, for an optional prompt.
type judge func(raw json.RawMessage) (entry RailEntry, refusal string, skip bool)

// openPrompt is a prompt waiting for the person.
type openPrompt struct {
	Prompt
	judge judge
	// browse opens a folder in a picker prompt; nil for other kinds.
	browse func(path string) error
	busy   sync.Mutex // one answer is judged at a time
	done   chan promptOutcome
}

type promptOutcome struct{ skipped bool }

var errNoSuchPrompt = errors.New("no such prompt is open")

// ask shows p and blocks until it is answered, skipped or ctx ends. The
// answer itself reaches the caller through what judge captures. Menus and
// pickers call it with their own Kind, fields and judge.
func (s *state) ask(ctx context.Context, p Prompt, judge judge, configure ...func(*openPrompt)) (skipped bool, err error) {
	open := &openPrompt{Prompt: p, judge: judge, done: make(chan promptOutcome, 1)}
	for _, apply := range configure {
		apply(open)
	}
	s.update(func() {
		s.nextPrompt++
		open.ID = s.nextPrompt
		s.prompt = open
	})
	select {
	case outcome := <-open.done:
		return outcome.skipped, nil
	case <-ctx.Done():
		s.update(func() {
			if s.prompt == open {
				s.prompt = nil
			}
		})
		return false, ctx.Err()
	}
}

type answerRequest struct {
	ID    int             `json:"id"`
	Value json.RawMessage `json:"value"`
}

// answer is the POST /actions/answer handler.
func (s *state) answer(body json.RawMessage) error {
	var req answerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	open, err := s.openPrompt(req.ID)
	if err != nil {
		return err
	}
	open.busy.Lock()
	defer open.busy.Unlock()
	if !s.isOpen(open) {
		return errNoSuchPrompt
	}
	entry, refusal, skip := open.judge(req.Value)
	return s.settle(open, func() {
		if refusal != "" {
			open.Refusal = refusal
			return
		}
		if !skip {
			s.answers = append(s.answers, entry)
		}
		s.prompt = nil
		open.done <- promptOutcome{skipped: skip}
	})
}

// skip is the POST /actions/skip handler: an optional prompt goes on without
// an answer.
func (s *state) skip(body json.RawMessage) error {
	var req answerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return err
	}
	open, err := s.openPrompt(req.ID)
	if err != nil {
		return err
	}
	if !open.Optional {
		return fmt.Errorf("%s is required", open.VarName)
	}
	open.busy.Lock()
	defer open.busy.Unlock()
	return s.settle(open, func() {
		s.prompt = nil
		open.done <- promptOutcome{skipped: true}
	})
}

func (s *state) openPrompt(id int) (*openPrompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prompt == nil || s.prompt.ID != id {
		return nil, errNoSuchPrompt
	}
	return s.prompt, nil
}

// isOpen says open is still the prompt waiting for the person. A POST that
// waited for open.busy checks it again, since the answer ahead of it may have
// settled the prompt.
func (s *state) isOpen(open *openPrompt) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prompt == open
}

// settle applies change if open is still the open prompt.
func (s *state) settle(open *openPrompt, change func()) error {
	var err error
	s.update(func() {
		if s.prompt != open {
			err = errNoSuchPrompt
			return
		}
		change()
	})
	return err
}

// PromptAsk puts q to the person in the page: a menu when it has options,
// else a text field. Its signature is runner.AskFunc's. The page only reads
// what the person did into an ask.Answer; q's judge rules on it. A secret
// is masked on the rail, and its default never reaches the page.
func (s *state) PromptAsk(ctx context.Context, q ask.Question) (value.Value, error) {
	if q.Path != nil {
		return s.promptPath(ctx, q)
	}
	prompt := Prompt{VarName: q.VarName, Info: q.Info, Secret: q.Secret, Optional: q.Optional, Multi: q.Multi}
	read := readText
	if q.IsMenu() {
		prompt.Kind, prompt.Options = KindMenu, q.Options
		if q.Multi {
			prompt.Defaults = q.Picks
		} else if len(q.Picks) > 0 {
			prompt.Default, prompt.HasDefault = q.Picks[0], true
		}
		read = readPicks(q.Multi)
	} else {
		prompt.Kind, prompt.HasDefault = KindText, q.Default != ""
		if !q.Secret {
			prompt.Default = q.Default
		}
	}
	var answer value.Value
	skipped, err := s.ask(ctx, prompt, func(raw json.RawMessage) (RailEntry, string, bool) {
		given, refusal := read(raw)
		if refusal != "" {
			return RailEntry{}, refusal, false
		}
		verdict := q.Judge(given)
		if verdict.Refusal != "" {
			return RailEntry{}, verdict.Refusal, false
		}
		if verdict.Skipped {
			return RailEntry{}, "", true
		}
		answer = verdict.Value
		entry := RailEntry{Label: label(q.Info, q.VarName), Value: shown(answer)}
		if q.Secret {
			entry.Value = maskedAnswer
		}
		return entry, "", false
	})
	if err != nil {
		return value.Value{}, err
	}
	if skipped {
		return q.Judge(ask.Answer{}).Value, nil
	}
	return answer, nil
}

// readText reads a text answer. null is Continue on a field left as its
// starting text, which a secret's masked default is the page never has.
func readText(raw json.RawMessage) (ask.Answer, string) {
	if string(raw) == "null" {
		return ask.Answer{Untouched: true}, ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return ask.Answer{}, "the answer must be text"
	}
	return ask.Answer{Text: text}, ""
}

// readPicks reads a menu answer: null is Continue on an untouched menu.
func readPicks(multi bool) func(json.RawMessage) (ask.Answer, string) {
	return func(raw json.RawMessage) (ask.Answer, string) {
		if string(raw) == "null" {
			return ask.Answer{Untouched: true}, ""
		}
		if multi {
			var picks []string
			if err := json.Unmarshal(raw, &picks); err != nil {
				return ask.Answer{}, "the answer must be a list of options"
			}
			return ask.Answer{Picks: picks}, ""
		}
		var one string
		if err := json.Unmarshal(raw, &one); err != nil {
			return ask.Answer{}, "the answer must be one of the options"
		}
		if one == "" {
			return ask.Answer{}, ""
		}
		return ask.Answer{Picks: []string{one}}, ""
	}
}

// shown is an answer as the rail reads it.
func shown(answer value.Value) string {
	list, ok := answer.Any().([]any)
	if !ok {
		return answer.String()
	}
	parts := make([]string, len(list))
	for i, item := range list {
		parts[i] = value.Of(item).String()
	}
	return strings.Join(parts, ", ")
}

// PromptSay shows message in the page until the person continues. Its
// signature is runner.SayPromptFunc's.
func (s *state) PromptSay(ctx context.Context, message, _ string) error {
	_, err := s.ask(ctx, Prompt{Kind: KindSay, Info: message}, func(json.RawMessage) (RailEntry, string, bool) {
		return RailEntry{}, "", true
	})
	return err
}
