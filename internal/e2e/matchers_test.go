package e2e

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"hobnob/internal/app"
)

// taskLineRe matches a line of a run: step's own stdout/stderr, which
// internal/tui.LineWriter prefixes with "[task] " at column 0 — distinct from
// hobnob's own chrome ("run: [task] ...", "✓ [task] done", "⊘ [task]
// skipped"), none of which start with "[" since their own marker comes
// first.
var taskLineRe = regexp.MustCompile(`(?m)^\[[^\]\n]+\] (.*)$`)

// OK asserts the run succeeded (exit code 0).
func (r *Result) OK(t *testing.T) {
	t.Helper()
	if r.ExitCode != 0 {
		t.Fatalf("expected success, got exit %d, err=%v\nstdout:\n%s\nstderr:\n%s", r.ExitCode, r.runErr, r.Stdout, r.Stderr)
	}
}

// Fails asserts the run failed (exit code 1).
func (r *Result) Fails(t *testing.T) {
	t.Helper()
	if r.ExitCode == 0 {
		t.Fatalf("expected failure, got success\nstdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
}

// Out asserts every sub is present somewhere in stdout.
func (r *Result) Out(t *testing.T, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(r.Stdout, sub) {
			t.Errorf("expected stdout to contain %q\nstdout:\n%s\nstderr:\n%s", sub, r.Stdout, r.Stderr)
		}
	}
}

// NotOut asserts none of subs appear in stdout.
func (r *Result) NotOut(t *testing.T, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if strings.Contains(r.Stdout, sub) {
			t.Errorf("expected stdout NOT to contain %q\nstdout:\n%s", sub, r.Stdout)
		}
	}
}

// Err asserts the run failed with an error whose message contains every sub.
// app.Run returns app.ErrSilent (an empty error) for a task failure — the
// real message already went to stderr — so the comparison target switches to
// Stderr in that case; callers never need to know which happened.
func (r *Result) Err(t *testing.T, subs ...string) {
	t.Helper()
	if r.runErr == nil {
		t.Fatalf("expected an error, got success\nstdout:\n%s\nstderr:\n%s", r.Stdout, r.Stderr)
	}
	target := r.runErr.Error()
	if errors.Is(r.runErr, app.ErrSilent) {
		target = r.Stderr
	}
	for _, sub := range subs {
		if !strings.Contains(target, sub) {
			t.Errorf("expected error output to contain %q, got: %s", sub, target)
		}
	}
}

// Order asserts subs all appear in stdout as an ordered subsequence — each
// found strictly after the previous one.
func (r *Result) Order(t *testing.T, subs ...string) {
	t.Helper()
	pos := 0
	for _, sub := range subs {
		idx := strings.Index(r.Stdout[pos:], sub)
		if idx < 0 {
			t.Fatalf("expected %q after position %d, not found in order\nstdout:\n%s", sub, pos, r.Stdout)
		}
		pos += idx + len(sub)
	}
}

// Lines asserts exact equality against the run: steps' own printed output —
// each line a command wrote to stdout, in order, with hobnob's own chrome
// ("run:", "✓", "⊘", the task-name bracket itself) stripped away. This is
// the workhorse for data-shaped assertions (set/into/loop/call/typed values):
// it has golden-file precision — ordering, completeness, no extras — without
// coupling the test to hobnob's decoration. See taskLineRe.
//
// Deliberately scoped to Stdout, never Combined: stdout and stderr drain on
// separate goroutines and interleave nondeterministically, so only one
// stream at a time has a meaningful order.
func (r *Result) Lines(t *testing.T, want ...string) {
	t.Helper()
	matches := taskLineRe.FindAllStringSubmatch(r.Stdout, -1)
	got := make([]string, len(matches))
	for i, m := range matches {
		got[i] = m[1]
	}
	if len(got) != len(want) {
		t.Fatalf("task output lines: got %d, want %d\ngot:  %#v\nwant: %#v\nfull stdout:\n%s", len(got), len(want), got, want, r.Stdout)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("task output line %d: got %q, want %q\ngot:  %#v\nwant: %#v", i, got[i], want[i], got, want)
		}
	}
}

// Masked asserts a secret is redacted everywhere it would otherwise appear —
// **** is present, and the raw secret text is absent — as a single paired
// check, so a test can't accidentally pass by asserting only half.
func (r *Result) Masked(t *testing.T, secret string) {
	t.Helper()
	if !strings.Contains(r.Combined, "****") {
		t.Errorf("expected mask marker **** in output, got:\n%s", r.Combined)
	}
	if strings.Contains(r.Combined, secret) {
		t.Errorf("secret %q leaked into output unmasked:\n%s", secret, r.Combined)
	}
}

// Prompted asserts the run asked for exactly these vars, in this order —
// duplicates included, so a check:-driven re-prompt shows up as the var name
// appearing twice.
func (r *Result) Prompted(t *testing.T, vars ...string) {
	t.Helper()
	got := make([]string, len(r.Prompts))
	for i, p := range r.Prompts {
		got[i] = p.VarName
	}
	if len(got) != len(vars) {
		t.Fatalf("prompted vars: got %v, want %v", got, vars)
	}
	for i := range vars {
		if got[i] != vars[i] {
			t.Errorf("prompt %d: got %q, want %q (full sequence got=%v want=%v)", i, got[i], vars[i], got, vars)
		}
	}
}
