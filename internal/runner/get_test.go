package runner

import (
	"context"
	"io"
	"strings"
	"testing"

	"hobnob/internal/ask"
	"hobnob/internal/config"
	"hobnob/internal/literal"
	"hobnob/internal/runevents"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// asked is what a get: entry did when a fake front end answered it.
type asked struct {
	question ask.Question
	verdicts []ask.Verdict
	vars     map[string]value.Value
	err      error
}

// getWith runs a task holding one get: entry with prompts on. The fake front
// end offers each answer to the question's judge in turn until one is
// accepted or skipped, as a real prompt keeps asking after a refusal.
func getWith(t *testing.T, entry config.GetEntry, answers ...ask.Answer) asked {
	t.Helper()
	cfg := &config.ConfigFile{Tasks: map[string]config.Task{
		"t": {Steps: []config.Step{{Kind: config.KindGet, GetEntries: []config.GetEntry{entry}}}},
	}}
	var got asked
	prompts := Prompts{Ask: func(_ context.Context, q ask.Question) (value.Value, error) {
		got.question = q
		for _, answer := range answers {
			verdict := q.Judge(answer)
			got.verdicts = append(got.verdicts, verdict)
			if verdict.Refusal == "" {
				return verdict.Value, nil
			}
		}
		t.Fatalf("no answer was accepted: %+v", got.verdicts)
		return value.Value{}, nil
	}}
	sc := scope.New()
	got.err = ExecuteTask(context.Background(), "t", sc, nil, cfg, false, t.TempDir(), runevents.New(), tui.NewRunLog(io.Discard, io.Discard), prompts)
	got.vars = sc.Vars()
	return got
}

func defaultOf(t *testing.T, raw string) literal.Expr {
	t.Helper()
	expr, err := literal.Single(raw)
	if err != nil {
		t.Fatal(err)
	}
	return expr
}

func TestGet_TextQuestion_CarriesWhatTheFrontEndPresents(t *testing.T) {
	// given a secret optional text entry with a default and info, when asked,
	// then the question holds the rendered info, the default and the flags
	// (why: front ends only present the question, so it must be complete)

	// Arrange
	entry := config.GetEntry{VarName: "NAME", Info: "Who?", Secret: true, Optional: true, DefaultTmpl: defaultOf(t, "ada")}

	// Act
	got := getWith(t, entry, ask.Answer{Text: "bob"})

	// Assert
	q := got.question
	if q.VarName != "NAME" || q.Info != "Who?" || q.Default != "ada" || !q.Secret || !q.Optional || q.IsMenu() {
		t.Errorf("unexpected question: %+v", q)
	}
}

func TestGet_TextJudge_AppliesDefaultRequiredAndSkipRules(t *testing.T) {
	// given text entries, when the judge gets an empty answer, then a
	// required one falls back to the default or is refused and an optional
	// one is skipped (why: these rules were copied per front end and drifted)

	// Arrange
	tests := []struct {
		name        string
		entry       config.GetEntry
		answer      ask.Answer
		wantRefusal string
		wantSkipped bool
		wantValue   string
	}{
		{
			name:      "given a default, when the answer is empty, then the default is taken (why: enter keeps the starting text)",
			entry:     config.GetEntry{VarName: "X", DefaultTmpl: defaultOf(t, "d")},
			answer:    ask.Answer{},
			wantValue: "d",
		},
		{
			name:        "given no default, when the answer is empty, then it is refused (why: a required answer is needed)",
			entry:       config.GetEntry{VarName: "X"},
			answer:      ask.Answer{},
			wantRefusal: "an answer is required",
		},
		{
			name:        "given optional, when the answer is empty, then it is skipped (why: optional allows no answer)",
			entry:       config.GetEntry{VarName: "X", Optional: true},
			answer:      ask.Answer{},
			wantSkipped: true,
		},
		{
			name:      "given optional with a default, when the answer is untouched, then the default is taken (why: continue on a secret's masked default keeps it)",
			entry:     config.GetEntry{VarName: "X", Optional: true, DefaultTmpl: defaultOf(t, "d")},
			answer:    ask.Answer{Untouched: true},
			wantValue: "d",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange (test fields are the arrangement)

			// Act
			got := getWith(t, test.entry, ask.Answer{Text: "x"})
			verdict := got.question.Judge(test.answer)

			// Assert
			if verdict.Refusal != test.wantRefusal || verdict.Skipped != test.wantSkipped {
				t.Fatalf("got %+v", verdict)
			}
			if test.wantValue != "" && verdict.Value.String() != test.wantValue {
				t.Errorf("value %q, want %q", verdict.Value.String(), test.wantValue)
			}
		})
	}
}

func TestGet_TextJudge_RefusesWithMatchAndCheckWording(t *testing.T) {
	// given match: and check:, when answers fail each, then the refusal says
	// "validation failed:" with the patterns or the check, and a passing
	// answer is stored (why: every front end shows the same wording)

	// Arrange
	entry := config.GetEntry{
		VarName:   "TAG",
		MatchList: []string{"v*"},
		Check:     `[ {{.TAG}} != v0 ]`,
	}

	// Act
	got := getWith(t, entry, ask.Answer{Text: "x1"}, ask.Answer{Text: "v0"}, ask.Answer{Text: "v1"})

	// Assert
	if len(got.verdicts) != 3 {
		t.Fatalf("verdicts %+v", got.verdicts)
	}
	if want := "validation failed: [ {{.TAG}} != v0 ]; must match v*"; got.verdicts[0].Refusal != want || got.verdicts[1].Refusal != want {
		t.Errorf("refusals %q, %q, want %q", got.verdicts[0].Refusal, got.verdicts[1].Refusal, want)
	}
	if got.vars["TAG"].String() != "v1" {
		t.Errorf("TAG = %q", got.vars["TAG"].String())
	}
}

func TestGet_MenuQuestion_ResolvesDefaultsIntoPicks(t *testing.T) {
	// given a multi menu whose default is a JSON list, when asked, then the
	// question's picks tick those options (why: the terminal once compared
	// each option with the whole JSON text and ticked nothing)

	// Arrange
	entry := config.GetEntry{
		VarName: "TOOLS", Multi: true, FromList: []string{"Meet", "Zoom", "Teams"},
		DefaultTmpl: defaultOf(t, `["Zoom"]`),
	}

	// Act
	got := getWith(t, entry, ask.Answer{Untouched: true})

	// Assert
	if q := got.question; !q.IsMenu() || len(q.Picks) != 1 || q.Picks[0] != "Zoom" || !q.Multi {
		t.Errorf("unexpected question: %+v", q)
	}
	if list, _ := got.vars["TOOLS"].Any().([]any); len(list) != 1 || list[0] != "Zoom" {
		t.Errorf("TOOLS = %v", got.vars["TOOLS"].Any())
	}
}

func TestGet_MenuJudge_AppliesPickRules(t *testing.T) {
	// given menus, when the judge gets picks, then it orders them like the
	// options, refuses strangers and empties, and skips an optional empty
	// (why: the page and the terminal must refuse alike)

	// Arrange
	multi := config.GetEntry{VarName: "X", Multi: true, FromList: []string{"a", "b", "c"}}
	single := config.GetEntry{VarName: "X", FromList: []string{"a", "b", "c"}}
	optional := config.GetEntry{VarName: "X", Multi: true, Optional: true, FromList: []string{"a", "b"}}
	tests := []struct {
		name        string
		entry       config.GetEntry
		answer      ask.Answer
		wantRefusal string
		wantSkipped bool
		wantPicks   string
	}{
		{"given a multi menu, when picks come unordered, then they follow the option order (why: stable lists)", multi, ask.Answer{Picks: []string{"c", "a"}}, "", false, "a,c"},
		{"given a menu, when a pick is no option, then it is refused (why: only offered options are valid)", multi, ask.Answer{Picks: []string{"z"}}, `"z" is not one of the options`, false, ""},
		{"given a required multi menu, when nothing is picked, then it is refused (why: an answer is required)", multi, ask.Answer{}, "pick at least one option", false, ""},
		{"given a required single menu, when nothing is picked, then it is refused (why: an answer is required)", single, ask.Answer{}, "an answer is required", false, ""},
		{"given an optional menu, when nothing is picked, then it is skipped with an empty list (why: optional allows no answer)", optional, ask.Answer{}, "", true, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange (test fields are the arrangement)

			// Act
			got := getWith(t, test.entry, ask.Answer{Picks: []string{"a"}})
			verdict := got.question.Judge(test.answer)

			// Assert
			if verdict.Refusal != test.wantRefusal || verdict.Skipped != test.wantSkipped {
				t.Fatalf("got %+v", verdict)
			}
			if test.wantPicks != "" {
				var picks []string
				for _, pick := range verdict.Value.Any().([]any) {
					picks = append(picks, pick.(string))
				}
				if strings.Join(picks, ",") != test.wantPicks {
					t.Errorf("picks %v, want %s", picks, test.wantPicks)
				}
			}
		})
	}
}

func TestGet_MenuJudge_CheckSeesTheChosenValue(t *testing.T) {
	// given a menu with check:, when a choice fails it, then the refusal is
	// "validation failed:" with the check and the next choice is stored
	// (why: the check judges the value the variable would hold)

	// Arrange
	entry := config.GetEntry{VarName: "ENV", FromList: []string{"dev", "prod"}, Check: `[ {{.ENV}} = dev ]`}

	// Act
	got := getWith(t, entry, ask.Answer{Picks: []string{"prod"}}, ask.Answer{Picks: []string{"dev"}})

	// Assert
	if got.verdicts[0].Refusal != "validation failed: [ {{.ENV}} = dev ]" {
		t.Errorf("refusal %q", got.verdicts[0].Refusal)
	}
	if got.vars["ENV"].String() != "dev" {
		t.Errorf("ENV = %q", got.vars["ENV"].String())
	}
}
