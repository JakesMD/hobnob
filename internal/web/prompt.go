package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// Kinds of prompt a Snapshot can carry.
const (
	KindText = "text"
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
	// Options, Multi and Defaults belong to a menu: what it offers, whether
	// several may be chosen, and the options ticked to start with.
	Options  []string `json:"options,omitempty"`
	Multi    bool     `json:"multi,omitempty"`
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

// PromptText asks for free text in the page. Its signature is
// runner.TextPromptFunc's; check: and match: arrive as validate.
func (s *state) PromptText(ctx context.Context, info, varName string,
	validate func(string) (bool, error), checkDesc, defaultVal, _ string,
	secret, optional bool) (string, error) {
	prompt := Prompt{Kind: KindText, VarName: varName, Info: info, Default: defaultVal,
		HasDefault: defaultVal != "", Secret: secret, Optional: optional}
	if secret {
		prompt.Default = ""
	}
	var answer string
	skipped, err := s.ask(ctx, prompt, func(raw json.RawMessage) (RailEntry, string, bool) {
		var text string
		// null is Continue on a secret's field left as its masked default,
		// whose text the page never has.
		untouchedDefault := string(raw) == "null"
		if err := json.Unmarshal(raw, &text); err != nil {
			return RailEntry{}, "the answer must be text", false
		}
		if text == "" && optional && !(untouchedDefault && defaultVal != "") {
			return RailEntry{}, "", true
		}
		if text == "" {
			text = defaultVal
		}
		if text == "" {
			return RailEntry{}, "an answer is required", false
		}
		if validate != nil {
			ok, err := validate(text)
			if err != nil {
				return RailEntry{}, fmt.Sprintf("check failed: %v", err), false
			}
			if !ok {
				return RailEntry{}, "validation failed: " + checkDesc, false
			}
		}
		answer = text
		entry := RailEntry{Label: label(info, varName), Value: text}
		if secret {
			entry.Value = maskedAnswer
		}
		return entry, "", false
	})
	if err != nil || skipped {
		return "", err
	}
	return answer, nil
}
