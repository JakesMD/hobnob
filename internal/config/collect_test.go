package config

import (
	"testing"
	"time"
)

// TestCollectGetParams_NotObservableCases covers the two CollectGetParams
// behaviors with no e2e symptom: a templated call: target can't be
// statically resolved, so it's silently skipped rather than erroring — and
// mutually recursive call: tasks must not hang --list forever. The rest of
// CollectGetParams' behavior (which get: vars surface as params, in what
// order, with set:/with:/into: satisfaction removing them, loop iterator
// vars excluded) is covered end-to-end by internal/e2e/cli_test.go's
// TestE2E_List_* tests, since --list's param display is exactly what this
// function drives.
func TestCollectGetParams_NotObservableCases(t *testing.T) {
	t.Run("given call step with template target, when collected, then skips dynamic call (why: cannot statically resolve template dispatch)", func(t *testing.T) {
		steps := []Step{
			{Kind: KindCall, CallTarget: "{{.PROFILE}}_setup"},
		}
		got := CollectGetParams(steps, nil)
		if len(got) != 0 {
			t.Fatalf("got %d entries, want 0: %v", len(got), got)
		}
	})

	t.Run("given mutually recursive call tasks, when collected, then does not infinite loop (why: cycle guard prevents stack overflow)", func(t *testing.T) {
		steps := []Step{
			{Kind: KindCall, CallTarget: "b"},
		}
		cfg := &ConfigFile{
			Tasks: map[string]Task{
				"b": {Steps: []Step{
					{Kind: KindGet, GetEntries: []GetEntry{{VarName: "FROM_B"}}},
					{Kind: KindCall, CallTarget: "a"},
				}},
				"a": {Steps: []Step{
					{Kind: KindCall, CallTarget: "b"},
				}},
			},
		}

		done := make(chan []GetEntry, 1)
		go func() { done <- CollectGetParams(steps, cfg) }()

		select {
		case got := <-done:
			want := []string{"FROM_B"}
			if len(got) != len(want) {
				t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
			}
			if got[0].VarName != want[0] {
				t.Errorf("entry[0].VarName = %q, want %q", got[0].VarName, want[0])
			}
		case <-time.After(2 * time.Second):
			t.Fatal("CollectGetParams did not return — cycle guard is broken")
		}
	})
}
