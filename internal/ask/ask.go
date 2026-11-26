// Package ask is the question a get: entry puts to a front end (the
// terminal, the page) and the judge that decides what the person's answer
// means. A front end presents the question and hands back what the person
// did; defaults, required refusal, check: and the wording of every refusal
// live in Judge, so no front end repeats them.
package ask

import (
	"fmt"
	"slices"

	"hobnob/internal/value"
)

// Question is one get: entry's prompt. It is a text prompt unless it has
// Options.
type Question struct {
	VarName string
	// Info is rendered already.
	Info string
	Task string
	// Options makes the question a menu: what it offers, after match:.
	Options  []string
	Multi    bool
	Secret   bool
	Optional bool
	// Default is the text a text prompt starts filled with.
	Default string
	// Picks are the options a menu starts chosen or ticked, in option
	// order, with the default already resolved.
	Picks []string
	// Check, when set, judges a candidate (match: and check:); CheckDesc is
	// what a refusal says it was.
	Check     func(candidate value.Value) (bool, error)
	CheckDesc string
}

// Answer is what the person did at the prompt.
type Answer struct {
	// Text is a text prompt's answer.
	Text string
	// Picks are a menu's chosen options.
	Picks []string
	// Untouched says the person continued without changing the starting
	// answer, which a front end that can't see it (a secret's default)
	// can't send.
	Untouched bool
}

// Verdict is the judge's ruling: Refusal keeps the prompt open and says why,
// Skipped says there is no answer (Value is then the empty one), otherwise
// Value is the accepted answer.
type Verdict struct {
	Value   value.Value
	Refusal string
	Skipped bool
}

// IsMenu reports whether q offers options.
func (q Question) IsMenu() bool { return len(q.Options) > 0 }

// Judge rules on an answer.
func (q Question) Judge(answer Answer) Verdict {
	if q.IsMenu() {
		return q.judgeMenu(answer)
	}
	return q.judgeText(answer)
}

func (q Question) judgeText(answer Answer) Verdict {
	text := answer.Text
	if answer.Untouched {
		text = q.Default
	}
	if text == "" && q.Optional {
		return q.skipped()
	}
	if text == "" {
		text = q.Default
	}
	if text == "" {
		return Verdict{Refusal: "an answer is required"}
	}
	return q.checked(value.Str(text))
}

func (q Question) judgeMenu(answer Answer) Verdict {
	chosen := answer.Picks
	if answer.Untouched {
		chosen = q.Picks
	}
	for _, pick := range chosen {
		if !slices.Contains(q.Options, pick) {
			return Verdict{Refusal: fmt.Sprintf("%q is not one of the options", pick)}
		}
	}
	var picks []string
	for _, option := range q.Options {
		if slices.Contains(chosen, option) {
			picks = append(picks, option)
		}
	}
	switch {
	case len(picks) == 0 && q.Optional:
		return q.skipped()
	case len(picks) == 0 && q.Multi:
		return Verdict{Refusal: "pick at least one option"}
	case len(picks) == 0:
		return Verdict{Refusal: "an answer is required"}
	case len(picks) > 1 && !q.Multi:
		return Verdict{Refusal: "the answer must be one of the options"}
	}
	if !q.Multi {
		return q.checked(value.Str(picks[0]))
	}
	list := make([]any, len(picks))
	for i, pick := range picks {
		list[i] = pick
	}
	return q.checked(value.Of(list))
}

func (q Question) checked(candidate value.Value) Verdict {
	if q.Check != nil {
		ok, err := q.Check(candidate)
		if err != nil {
			return Verdict{Refusal: fmt.Sprintf("check failed: %v", err)}
		}
		if !ok {
			return Verdict{Refusal: "validation failed: " + q.CheckDesc}
		}
	}
	return Verdict{Value: candidate}
}

func (q Question) skipped() Verdict {
	if q.Multi {
		return Verdict{Value: value.Of([]any{}), Skipped: true}
	}
	return Verdict{Value: value.Str(""), Skipped: true}
}
