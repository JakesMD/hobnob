// Package e2e drives hobnob end-to-end: a hobnob.yml (or a small set of
// files) goes in, app.Run executes it for real, and assertions read what the
// user would actually see — printed output, exit code, prompts asked. See
// GUIDE.md for the language this suite is a spec of.
//
// Mutation checklist — each line is a one-statement break that some e2e test
// in this package must catch. Run through this list after any change to the
// harness itself, and in full before a release; it's a stronger signal than
// a coverage percentage because it tests the assertions, not just execution.
//
//  1. reverse matrix nesting order in internal/runner/loop.go        -> loop_test.go (.Lines)
//  2. drop the raw-value ReplaceAll in maskSecrets                   -> secrets_test.go (.Masked)
//  3. drop the JSON-escaped ReplaceAll in maskSecrets                -> secrets_test.go (into: leaf)
//  4. apply cliVars after globals in internal/cli/scope.go           -> scope_test.go precedence
//  5. remove the "_" guard in internal/app/app.go Run                -> cli_test.go
//  6. make Task.Hidden not suppress from --list                      -> cli_test.go (.NotOut)
//  7. make module flatten override a native task                    -> modules_test.go
//  8. skip Scope.Copy() before execCall                              -> call_test.go isolation
//  9. evaluate set: entries bottom-to-top                            -> set_test.go
//
// 10. make a later env: entry lose to an earlier one                 -> env_test.go
// 11. resolve relative dir: against cwd instead of TaskfileDir       -> dir_test.go
// 12. make value.Capture always return a String                     -> values_test.go
// 13. swallow the check: re-prompt loop in get.go                    -> get_test.go
// 14. make step-level dir: lose to task-level                        -> dir_test.go
// 15. drop the missing-env-file warning in envfiles.go               -> env_test.go stderr
package e2e

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"hobnob/internal/app"
	"hobnob/internal/runner"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// envMu serializes every e2e test's use of process-global state: os.Environ,
// os.Stdout/Stderr, and the working directory. Tests in this package must
// never call t.Parallel() — see the note on Run below.
var envMu sync.Mutex

// Files maps a relative path to file content. "hobnob.yml" is the entry
// point app.Run is pointed at.
type Files map[string]string

// Case describes one end-to-end scenario.
type Case struct {
	Yml      string              // shorthand for Files{"hobnob.yml": Yml}
	Files    Files               // multi-file, inline; wins over Yml if both set
	Dir      string              // OR: copy this internal/e2e/testdata subdirectory in
	Args     []string            // as the user types, without --file
	Answers  map[string][]string // fake prompt answers by var name; consumed in order, re-tried against check:
	TaskPick string              // answer for the interactive task-select picker
	Env      map[string]string   // the ONLY env vars visible to the run (plus PATH/HOME/TMPDIR)
	TTY      bool                // App.IsTerminal (default false). Set true to reach get:'s interactive
	// path at all — like the real CLI, a non-terminal stdin skips straight to
	// getFromNoPrompts regardless of Answers, so any test asserting on a
	// prompt (Answers, TaskPick, .Prompted) needs TTY: true.
	Cwd      string // subdir of the temp root to chdir into (default: the root itself)
	Discover bool   // omit --file, exercise findTaskfile's upward walk
}

// PromptCall records one prompt the run actually made.
type PromptCall struct {
	VarName  string
	Info     string
	Default  string
	Items    []string
	Multi    bool
	Secret   bool
	Optional bool
	Task     string
}

// Result is what the user would have seen.
type Result struct {
	Stdout, Stderr, Combined string
	ExitCode                 int
	Prompts                  []PromptCall
	Dir                      string

	// runErr is unexported so it can't be compared directly (app.ErrSilent's
	// empty message makes that a footgun) — go through .OK/.Fails/.Err.
	runErr error
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// dedent strips the common leading whitespace from a raw-string YAML
// fixture, and its leading/trailing blank lines, so fixtures can be indented
// to match the surrounding Go code.
func dedent(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return strings.Join(lines, "\n") + "\n"
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if len(line) >= minIndent {
			out[i] = line[minIndent:]
		} else {
			out[i] = strings.TrimLeft(line, " \t")
		}
	}
	return strings.Join(out, "\n") + "\n"
}

// Yml is the 80% shorthand: a single-file fixture and the args to run it
// with.
func Yml(t *testing.T, yml string, args ...string) *Result {
	t.Helper()
	return Run(t, Case{Yml: yml, Args: args})
}

// Run writes c's files into a fresh temp dir, points a real app.Run at them
// with every seam (terminal, prompts, task picker, environment, cwd)
// substituted, and captures exactly what got printed.
//
// Every seam Run touches — os.Stdout/os.Stderr, os.Environ, the process cwd,
// runner's prompt funcs — is process-global, so tests in this package must
// never run in parallel (no t.Parallel()). envMu enforces serialization
// against other e2e tests; it does nothing for non-e2e tests sharing the
// process, which is why this package owns its own dedicated test binary.
func Run(t *testing.T, c Case) *Result {
	t.Helper()
	envMu.Lock()
	defer envMu.Unlock()

	files := c.Files
	if files == nil {
		files = Files{"hobnob.yml": c.Yml}
	}
	for _, content := range files {
		if strings.Contains(content, "sleep ") {
			t.Fatal("fixture contains 'sleep ' — e2e tests must not sleep; see harness_test.go runtime rules")
		}
	}

	dir := t.TempDir()
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}

	if c.Dir != "" {
		copyTestdataDir(t, c.Dir, dir)
	}
	for relPath, content := range files {
		full := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(dedent(content)), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", full, err)
		}
	}

	cwd := dir
	if c.Cwd != "" {
		cwd = filepath.Join(dir, c.Cwd)
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", cwd, err)
		}
	}
	t.Chdir(cwd)

	restoreEnv := isolateEnv(t, c.Env)
	defer restoreEnv()

	args := c.Args
	if !c.Discover {
		tfPath := filepath.Join(dir, "hobnob.yml")
		args = append([]string{"--file", tfPath}, args...)
	}

	state := newPromptState(t, c.Answers)
	restorePrompts := runner.SetPrompts(state.text, state.sel)
	defer restorePrompts()

	a := app.New("v0.0.0-test")
	a.IsTerminal = func() bool { return c.TTY }
	a.SelectTask = func(ctx context.Context, tasks []tui.TaskItem) (string, error) {
		if c.TaskPick == "" {
			t.Fatalf("unexpected task-select prompt (%d tasks offered) — Case.TaskPick not set", len(tasks))
		}
		state.record(PromptCall{VarName: "(task select)", Task: c.TaskPick})
		return c.TaskPick, nil
	}

	res := &Result{Dir: dir}
	stdout, stderr, combined := captureOutput(t, func() {
		res.runErr = a.Run(t.Context(), args)
	})
	res.Stdout = ansiRe.ReplaceAllString(stdout, "")
	res.Stderr = ansiRe.ReplaceAllString(stderr, "")
	res.Combined = ansiRe.ReplaceAllString(combined, "")
	res.Prompts = state.calls
	if res.runErr != nil {
		res.ExitCode = 1
	}
	return res
}

// isolateEnv clears the process environment down to PATH/HOME/TMPDIR plus
// exactly what env specifies, and returns a func restoring the original
// environment. cli.BuildScope reads os.Environ() wholesale, so anything left
// over from the host (CI, USER, a stray VERSION) would otherwise leak into
// scope and silently change a test's result — the existing subprocess-based
// tests each hand-scrubbed CI= slightly differently, which this replaces.
// CI is deliberately not restored: defaultNoPrompts() must be driven solely
// by Case.TTY.
func isolateEnv(t *testing.T, env map[string]string) (restore func()) {
	t.Helper()
	for _, forbidden := range []string{"PATH", "HOME"} {
		if _, ok := env[forbidden]; ok {
			t.Fatalf("Case.Env must not set %s — every scope var is exported into the child shell (see envWithScopeOverrides), so overriding it breaks sh -c itself, not hobnob", forbidden)
		}
	}

	orig := os.Environ()
	origPath, origHome, origTmp := os.Getenv("PATH"), os.Getenv("HOME"), os.Getenv("TMPDIR")

	os.Clearenv()
	os.Setenv("PATH", origPath)
	os.Setenv("HOME", origHome)
	if origTmp != "" {
		os.Setenv("TMPDIR", origTmp)
	}
	for k, v := range env {
		os.Setenv(k, v)
	}

	return func() {
		os.Clearenv()
		for _, kv := range orig {
			if key, val, ok := strings.Cut(kv, "="); ok {
				os.Setenv(key, val)
			}
		}
	}
}

// captureOutput swaps os.Stdout/os.Stderr for the duration of f, draining
// both concurrently so output past the ~64KB pipe buffer can't deadlock (the
// bug in the harness this replaces, internal/runner/runner_test.go's old
// captureStdout, closed the pipe before draining it).
func captureOutput(t *testing.T, f func()) (stdout, stderr, combined string) {
	t.Helper()
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr

	var mu sync.Mutex
	var outBuf, errBuf, combinedBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	drain := func(r *os.File, dst *bytes.Buffer) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				mu.Lock()
				dst.Write(buf[:n])
				combinedBuf.Write(buf[:n])
				mu.Unlock()
			}
			if readErr != nil {
				return
			}
		}
	}
	go drain(rOut, &outBuf)
	go drain(rErr, &errBuf)

	f()

	os.Stdout, os.Stderr = origOut, origErr
	wOut.Close()
	wErr.Close()
	wg.Wait()
	rOut.Close()
	rErr.Close()

	return outBuf.String(), errBuf.String(), combinedBuf.String()
}

func copyTestdataDir(t *testing.T, name, destRoot string) {
	t.Helper()
	src := filepath.Join("testdata", name)
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read testdata dir %q: %v", src, err)
	}
	var walk func(rel string)
	walk = func(rel string) {
		full := filepath.Join(src, rel)
		info, err := os.Stat(full)
		if err != nil {
			t.Fatalf("stat %q: %v", full, err)
		}
		if info.IsDir() {
			children, err := os.ReadDir(full)
			if err != nil {
				t.Fatalf("read dir %q: %v", full, err)
			}
			for _, child := range children {
				walk(filepath.Join(rel, child.Name()))
			}
			return
		}
		content, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("read %q: %v", full, err)
		}
		destPath := filepath.Join(destRoot, rel)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(destPath), err)
		}
		if err := os.WriteFile(destPath, content, 0o644); err != nil {
			t.Fatalf("write %q: %v", destPath, err)
		}
	}
	for _, entry := range entries {
		walk(entry.Name())
	}
}

// promptState backs the fake prompt implementations installed via
// runner.SetPrompts, plus the fake task-select. It is not itself concurrency
// -safe against Run() calls in other tests — Run's envMu lock already
// prevents that — but the drain goroutines in captureOutput never touch it,
// so no locking is needed for those.
type promptState struct {
	t       *testing.T
	answers map[string][]string
	textIdx map[string]int
	selIdx  map[string]int
	calls   []PromptCall
}

func newPromptState(t *testing.T, answers map[string][]string) *promptState {
	return &promptState{
		t:       t,
		answers: answers,
		textIdx: map[string]int{},
		selIdx:  map[string]int{},
	}
}

func (s *promptState) record(c PromptCall) {
	s.calls = append(s.calls, c)
}

// text implements runner.TextPromptFunc. It replays successive answers for
// varName until one satisfies validate — the same check:/re-prompt contract
// tui.PromptText's real bubbletea loop implements — and fails loudly rather
// than hanging if every answer is exhausted without success.
func (s *promptState) text(ctx context.Context, info, varName string, validate func(string) (bool, error), checkDesc, defaultVal, task string, secret, optional bool) (string, error) {
	s.t.Helper()
	answers := s.answers[varName]
	start := s.textIdx[varName]
	for i := start; i < len(answers); i++ {
		candidate := answers[i]
		if validate != nil {
			ok, _ := validate(candidate)
			if !ok {
				continue
			}
		}
		s.textIdx[varName] = i + 1
		s.record(PromptCall{VarName: varName, Info: info, Default: defaultVal, Secret: secret, Optional: optional, Task: task})
		return candidate, nil
	}
	s.t.Fatalf("unexpected text prompt for %q (task %q) — got %d candidate answer(s), none satisfied check: %q, or none were provided", varName, task, len(answers)-start, checkDesc)
	return "", errors.New("unreachable")
}

// sel implements runner.SelectPromptFunc. Each call consumes the next
// not-yet-used answer for varName; internal/runner's own check:/re-prompt
// loop (promptSelectUntilValid) is what drives repeat calls, not this func.
func (s *promptState) sel(ctx context.Context, varName, info string, items []string, multi bool, defaultVal, task string, secret bool) (value.Value, error) {
	s.t.Helper()
	answers := s.answers[varName]
	idx := s.selIdx[varName]
	if idx >= len(answers) {
		s.t.Fatalf("unexpected select prompt for %q (task %q, visit %d) — only %d answer(s) provided", varName, task, idx+1, len(answers))
	}
	raw := answers[idx]
	s.selIdx[varName] = idx + 1
	s.record(PromptCall{VarName: varName, Info: info, Default: defaultVal, Items: items, Multi: multi, Secret: secret, Task: task})
	if multi {
		parts := []any{}
		if raw != "" {
			for _, p := range strings.Split(raw, ",") {
				parts = append(parts, p)
			}
		}
		return value.Of(parts), nil
	}
	return value.Str(raw), nil
}
