package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hobnob/internal/config"
)

func makeRunCfg(command string, into []config.IntoEntry) *config.ConfigFile {
	return &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{{Kind: config.KindRun, Command: command, IntoEntries: into}}},
		},
	}
}

// realPath resolves symlinks so macOS /var/folders paths compare correctly.
func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", p, err)
	}
	return r
}

// captureRunDir runs a single run: step and returns the working directory
// the command executed in, captured via `pwd > outfile`.
func captureRunDir(t *testing.T, step config.Step, taskfileDir, parentDir string) string {
	t.Helper()
	outFile := filepath.Join(t.TempDir(), "out.txt")
	step.Command = fmt.Sprintf("pwd > '%s'", outFile)
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{step}},
		},
		TaskfileDir: taskfileDir,
	}
	if err := ExecuteTask(context.Background(), "t", makeScope(map[string]string{}), cfg, true, parentDir); err != nil {
		t.Fatalf("ExecuteTask: %v", err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read out file: %v", err)
	}
	return strings.TrimRight(string(data), "\n")
}

func TestDir_RunStep(t *testing.T) {
	taskfileDir := t.TempDir()
	parentDir := t.TempDir()
	stepDir := t.TempDir()

	subName := "sub"
	subDir := filepath.Join(taskfileDir, subName)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	tests := []struct {
		name        string
		stepDirTmpl string
		wantDir     string
	}{
		{
			name:        "given no step dir, when run, then uses parentDir (why: inherited dir is the default)",
			stepDirTmpl: "",
			wantDir:     realPath(t, parentDir),
		},
		{
			name:        "given absolute step dir, when run, then uses step dir (why: absolute path overrides context)",
			stepDirTmpl: stepDir,
			wantDir:     realPath(t, stepDir),
		},
		{
			name:        "given relative step dir, when run, then resolved from taskfileDir (why: relative paths anchor to the taskfile)",
			stepDirTmpl: subName,
			wantDir:     realPath(t, subDir),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			step := config.Step{Kind: config.KindRun, DirTmpl: test.stepDirTmpl}

			// Act
			got := captureRunDir(t, step, taskfileDir, parentDir)

			// Assert
			if got != test.wantDir {
				t.Errorf("run dir: got %q, want %q", got, test.wantDir)
			}
		})
	}
}

func TestRunInto(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		into     []config.IntoEntry
		wantVars map[string]string
		wantErr  string
	}{
		{
			name:     "given stdout into, when command prints to stdout, then captures raw stdout (why: basic capture without any pipe transform)",
			command:  "printf 'hello'",
			into:     []config.IntoEntry{{ParentKey: "OUT", ValueTmpl: "stdout"}},
			wantVars: map[string]string{"OUT": "hello"},
		},
		{
			name:     "given stderr into, when command prints to stderr, then captures raw stderr (why: error output captured separately from stdout)",
			command:  "printf 'err msg' >&2",
			into:     []config.IntoEntry{{ParentKey: "ERR", ValueTmpl: "stderr"}},
			wantVars: map[string]string{"ERR": "err msg"},
		},
		{
			name:     "given stdout | trim into, when command echoes with newline, then trims whitespace (why: common single-value capture pattern)",
			command:  "printf 'hello\n'",
			into:     []config.IntoEntry{{ParentKey: "OUT", ValueTmpl: "stdout | trim"}},
			wantVars: map[string]string{"OUT": "hello"},
		},
		{
			name:     "given stdout | lines into, when command outputs multiple lines, then returns JSON array (why: captures list output into loop-ready var)",
			command:  "printf 'alpha\nbeta\ngamma'",
			into:     []config.IntoEntry{{ParentKey: "ITEMS", ValueTmpl: "stdout | lines"}},
			wantVars: map[string]string{"ITEMS": `["alpha","beta","gamma"]`},
		},
		{
			name:     "given stdout | upper into, when command outputs lowercase, then stores uppercase (why: normalise captured output)",
			command:  "printf 'hello'",
			into:     []config.IntoEntry{{ParentKey: "OUT", ValueTmpl: "stdout | upper"}},
			wantVars: map[string]string{"OUT": "HELLO"},
		},
		{
			name:     "given stdout | lower into, when command outputs uppercase, then stores lowercase (why: normalise captured output)",
			command:  "printf 'HELLO'",
			into:     []config.IntoEntry{{ParentKey: "OUT", ValueTmpl: "stdout | lower"}},
			wantVars: map[string]string{"OUT": "hello"},
		},
		{
			name:    "given two into entries, when command writes to both streams, then captures each independently (why: stdout and stderr are distinct capture targets)",
			command: "printf 'out' && printf 'err' >&2",
			into: []config.IntoEntry{
				{ParentKey: "OUT", ValueTmpl: "stdout"},
				{ParentKey: "ERR", ValueTmpl: "stderr"},
			},
			wantVars: map[string]string{"OUT": "out", "ERR": "err"},
		},
		{
			name:    "given unknown pipe function in into, when executed, then returns error (why: typo guard surfaces early)",
			command: "printf 'hello'",
			into:    []config.IntoEntry{{ParentKey: "OUT", ValueTmpl: "stdout | nope"}},
			wantErr: "run into",
		},
		{
			name:    "given a nested object into entry with pluck leaves, when command outputs JSON, then assembles one JSON var from several plucked fields (why: into: leaves keep their normal stdout|filter grammar, just nested under keys instead of flattened to separate vars)",
			command: `printf '{"id":42,"profile":{"name":"Ada"}}'`,
			into: []config.IntoEntry{
				{ParentKey: "CUSTOM", ValNode: &config.JSONNode{
					Kind: config.JSONObject,
					Fields: []config.JSONField{
						{Key: "id", Node: config.JSONNode{Kind: config.JSONString, Tmpl: `stdout | pluck "id"`}},
						{Key: "name", Node: config.JSONNode{Kind: config.JSONString, Tmpl: `stdout | pluck "profile.name"`}},
					},
				}},
			},
			wantVars: map[string]string{"CUSTOM": `{"id":"42","name":"Ada"}`},
		},
		{
			name:    "given a two-level-deep nested into entry, when command outputs JSON, then arbitrary nesting is preserved (why: json is json — the same object/array recursion applies at any depth)",
			command: `printf '{"a":"x","b":"y"}'`,
			into: []config.IntoEntry{
				{ParentKey: "SHAPE", ValNode: &config.JSONNode{
					Kind: config.JSONObject,
					Fields: []config.JSONField{
						{Key: "outer", Node: config.JSONNode{
							Kind: config.JSONObject,
							Fields: []config.JSONField{
								{Key: "a", Node: config.JSONNode{Kind: config.JSONString, Tmpl: `stdout | pluck "a"`}},
							},
						}},
					},
				}},
			},
			wantVars: map[string]string{"SHAPE": `{"outer":{"a":"x"}}`},
		},
		{
			name:    "given a nested into entry whose plucked value contains a double quote, when executed, then the quote is escaped by json.Marshal rather than corrupting the JSON (why: regression for the same injection bug fixed on the set:/with: side — marshal happens once, after every leaf is evaluated)",
			command: `printf '%s' '{"msg":"he said \"hi\""}'`,
			into: []config.IntoEntry{
				{ParentKey: "OUT", ValNode: &config.JSONNode{
					Kind: config.JSONObject,
					Fields: []config.JSONField{
						{Key: "msg", Node: config.JSONNode{Kind: config.JSONString, Tmpl: `stdout | pluck "msg"`}},
					},
				}},
			},
			wantVars: map[string]string{"OUT": `{"msg":"he said \"hi\""}`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg := makeRunCfg(test.command, test.into)
			vars := map[string]string{}

			// Act
			err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, "")

			// Assert
			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", test.wantErr)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Errorf("error: got %q, want it to contain %q", err.Error(), test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, want := range test.wantVars {
				if got := vars[k]; got != want {
					t.Errorf("vars[%s]: got %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestRunStep_ScopeVarOverridesInheritedEnv(t *testing.T) {
	// given env var FOO set in ambient environment, when scope sets FOO to a
	// different value, then the run step sees the scope value (why: scope vars
	// must win over inherited env — exec first-occurrence semantics means the
	// old append-only approach silently lost the override)
	t.Setenv("FOO", "from-env")

	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{Kind: config.KindRun, Command: "printf '%s' \"$FOO\"", IntoEntries: []config.IntoEntry{
					{ParentKey: "RESULT", ValueTmpl: "stdout"},
				}},
			}},
		},
	}
	vars := map[string]string{"FOO": "from-scope"}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := vars["RESULT"]; got != "from-scope" {
		t.Errorf("RESULT: got %q, want %q (why: scope var must override inherited env)", got, "from-scope")
	}
}

func TestRunStep_EnvVarNotInScope_StillVisible(t *testing.T) {
	// given env var BAR not in scope, when run step reads BAR, then it sees the
	// ambient env value (why: env vars not shadowed by scope must still pass through)
	t.Setenv("BAR", "from-env")

	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{Kind: config.KindRun, Command: "printf '%s' \"$BAR\"", IntoEntries: []config.IntoEntry{
					{ParentKey: "RESULT", ValueTmpl: "stdout"},
				}},
			}},
		},
	}
	vars := map[string]string{}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := vars["RESULT"]; got != "from-env" {
		t.Errorf("RESULT: got %q, want %q (why: env vars not in scope must still reach the subprocess)", got, "from-env")
	}
}

func TestStepIf_RunConditionFalse_PrintsSkipLine(t *testing.T) {
	// given run: step with if: that evaluates false, when executed,
	// then prints a skip line and does not run the command
	// (why: user should see why the command didn't execute, not silence)

	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {
				Steps: []config.Step{
					{
						Kind:    config.KindRun,
						Command: "echo should-not-run",
						IfExpr:  `[ "{{.ENABLED}}" = "true" ]`,
					},
				},
			},
		},
	}
	vars := map[string]string{"ENABLED": "false"}

	// Act
	var err error
	out := captureStdout(t, func() {
		err = ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())
	})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "run:") || !strings.Contains(out, "skipped") {
		t.Errorf("expected skip line mentioning run: and skipped, got: %q", out)
	}
	if !strings.Contains(out, "[t]") {
		t.Errorf("expected skip line to include task prefix [t], got: %q", out)
	}
}

func TestStepIf_EvaluatedInInheritedDir_NotStepDir(t *testing.T) {
	// given run: step with its own dir: and an if: that checks a file
	// relative to the inherited task dir, when executed, then if: runs in
	// the inherited dir, not the step's own dir: override (why: if: gates
	// whether the step's dir: override even applies, so it must not depend
	// on that override itself)

	// Arrange
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "marker"), nil, 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	cfg := &config.ConfigFile{
		TaskfileDir: root,
		Tasks: map[string]config.Task{
			"t": {
				Steps: []config.Step{
					{
						Kind:    config.KindRun,
						Command: "true",
						DirTmpl: "sub",
						IfExpr:  `[ -f marker ]`,
					},
				},
			},
		},
	}
	vars := map[string]string{}

	// Act
	var err error
	out := captureStdout(t, func() {
		err = ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, root)
	})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "skipped") {
		t.Errorf("step should have run because marker exists in the inherited dir, but was skipped: %q", out)
	}
}

func TestStepIf_NonRunConditionFalse_PrintsNothing(t *testing.T) {
	// given set: step with if: that evaluates false, when executed,
	// then nothing is printed (why: skip printing is scoped to run: steps only)

	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {
				Steps: []config.Step{
					{
						Kind:       config.KindSet,
						SetEntries: []config.SetEntry{{Key: "MARKER", ValTmpl: "ran"}},
						IfExpr:     `[ "{{.ENABLED}}" = "true" ]`,
					},
				},
			},
		},
	}
	vars := map[string]string{"ENABLED": "false"}

	// Act
	var err error
	out := captureStdout(t, func() {
		err = ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())
	})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected no output for skipped non-run step, got: %q", out)
	}
	if vars["MARKER"] == "ran" {
		t.Error("set step should have been skipped")
	}
}

func TestExecRun_CtxCancelled_ReturnsErrInterrupted(t *testing.T) {
	// given a run: step whose command is still executing, when ctx is
	// cancelled, then the returned error wraps ErrInterrupted (why: callers
	// need to distinguish a graceful shutdown from an ordinary command failure)

	// Arrange
	cfg := makeRunCfg("sleep 5", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Act
	err := ExecuteTask(ctx, "t", makeScope(map[string]string{}), cfg, true, t.TempDir())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
}

func TestExecRun_OrdinaryFailure_NotWrappedAsInterrupted(t *testing.T) {
	// given a run: step that fails on its own (ctx never cancelled), when
	// executed, then the error is returned as-is, not wrapped as
	// ErrInterrupted (why: regression guard so ordinary command failures keep
	// their original exit-status error instead of being misreported as a
	// graceful shutdown)

	// Arrange
	cfg := makeRunCfg("exit 3", nil)

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(map[string]string{}), cfg, true, t.TempDir())

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, ErrInterrupted) {
		t.Fatalf("ordinary failure should not be wrapped as ErrInterrupted, got: %v", err)
	}
}

func TestExecRun_CtxCancelled_KillsWholeProcessGroup(t *testing.T) {
	// given a run: step whose shell forks a background child (not just the
	// tail-exec'd sh itself), when ctx is cancelled, then the child is killed
	// too (why: shellCmd.Cancel signals the whole process group so multi-
	// command scripts/pipelines/background jobs don't outlive the graceful
	// shutdown)

	// Arrange
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-finished")
	cfg := makeRunCfg(fmt.Sprintf("(sleep 5; touch %s) & wait", marker), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Act
	err := ExecuteTask(ctx, "t", makeScope(map[string]string{}), cfg, true, dir)

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // give a leaked child a chance to finish
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("background child kept running after ctx cancellation — process group was not killed")
	}
}

func TestKillRunningStep_ForceKillsGroupThatIgnoredSIGTERM(t *testing.T) {
	// given a run: step whose shell (and its background child) ignore
	// SIGTERM, when ctx is cancelled (graceful shutdown) and KillRunningStep
	// is then called (2nd CTRL+C), then the whole still-running process group
	// is force-killed (why: regression test for the bug where the 2nd
	// CTRL+C targeted hobnob's own process group instead of the step's —
	// see cmd/hobnob/main.go)

	// Arrange
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-finished")
	cfg := makeRunCfg(fmt.Sprintf("trap '' TERM; (sleep 5; touch %s) & wait", marker), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- ExecuteTask(ctx, "t", makeScope(map[string]string{}), cfg, true, dir)
	}()
	time.Sleep(150 * time.Millisecond) // let ctx cancel and the ignored SIGTERM land

	// Act
	KillRunningStep()

	// Assert
	select {
	case err := <-done:
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("expected ErrInterrupted, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ExecuteTask did not return after KillRunningStep")
	}
	time.Sleep(200 * time.Millisecond) // give a leaked child a chance to finish
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("background child kept running after KillRunningStep — force-kill did not reach the step's process group")
	}
}

func TestKillRunningStep_NoStepRunning_NoOp(t *testing.T) {
	// given no run: step is currently executing, when KillRunningStep is
	// called, then it does nothing and does not panic (why: the 2nd-CTRL+C
	// handler must be able to call it unconditionally, including when
	// hobnob is idle e.g. blocked on a get: prompt)

	// Act / Assert
	KillRunningStep()
}
