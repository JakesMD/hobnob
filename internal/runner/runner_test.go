package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/value"
)

func copyVars(src map[string]value.Value) map[string]value.Value {
	out := make(map[string]value.Value, len(src))
	for key, val := range src {
		out[key] = val
	}
	return out
}

func makeScope(vars map[string]value.Value) *cli.Scope {
	return &cli.Scope{Vars: vars, Secrets: make(map[string]bool)}
}

// sv wraps a plain string map as typed scope vars — most fixtures in this
// package only care about plain strings; the type distinction is incidental.
func sv(m map[string]string) map[string]value.Value {
	out := make(map[string]value.Value, len(m))
	for k, v := range m {
		out[k] = value.Str(v)
	}
	return out
}

func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	f()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	return buf.String()
}

func TestMaskSecrets(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		vars    map[string]value.Value
		secrets map[string]bool
		want    string
	}{
		{
			name:    "given no secrets, when masking, then string unchanged (why: nothing to mask)",
			input:   "deploy --user=alice --pass=hunter2",
			vars:    sv(map[string]string{"PASS": "hunter2"}),
			secrets: map[string]bool{},
			want:    "deploy --user=alice --pass=hunter2",
		},
		{
			name:    "given secret var in command, when masking, then value replaced with **** (why: secret must not appear in logs)",
			input:   "deploy --user=alice --pass=hunter2",
			vars:    sv(map[string]string{"PASS": "hunter2"}),
			secrets: map[string]bool{"PASS": true},
			want:    "deploy --user=alice --pass=****",
		},
		{
			name:    "given secret value appears multiple times, when masking, then all replaced (why: full redaction required)",
			input:   "echo hunter2 && login --pass=hunter2",
			vars:    sv(map[string]string{"PASS": "hunter2"}),
			secrets: map[string]bool{"PASS": true},
			want:    "echo **** && login --pass=****",
		},
		{
			name:    "given secret var with empty value, when masking, then string unchanged (why: empty string replacement would corrupt output)",
			input:   "deploy --pass=",
			vars:    sv(map[string]string{"PASS": ""}),
			secrets: map[string]bool{"PASS": true},
			want:    "deploy --pass=",
		},
		{
			name:    "given multiple secret vars, when masking, then all replaced (why: each secret must be redacted)",
			input:   "connect --user=root --pass=s3cr3t --token=abc123",
			vars:    sv(map[string]string{"PASS": "s3cr3t", "TOKEN": "abc123"}),
			secrets: map[string]bool{"PASS": true, "TOKEN": true},
			want:    "connect --user=root --pass=**** --token=****",
		},
		{
			name:    `given secret containing a quote embedded in a JSON literal (json.Marshal-escaped), when masking, then the escaped form is also replaced (why: a set:/into: JSON literal leaf marshals its value, so the escaped form can differ from the raw secret and must be matched too)`,
			input:   `echo 'literal={"token":"ab\"cd"} raw=ab"cd'`,
			vars:    sv(map[string]string{"TOK": `ab"cd`}),
			secrets: map[string]bool{"TOK": true},
			want:    `echo 'literal={"token":"****"} raw=****'`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange (test fields are the arrangement)

			// Act
			got := maskSecrets(test.input, &cli.Scope{Vars: test.vars, Secrets: test.secrets})

			// Assert
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestDisplayDirPath(t *testing.T) {
	tests := []struct {
		name          string
		dir           string
		invocationDir string
		want          string
	}{
		{
			name:          "given dir is invocation dir, when formatting, then returns ./ (why: dir is the cwd itself, mirror the dir: path style)",
			dir:           "/home/user/project",
			invocationDir: "/home/user/project",
			want:          "./",
		},
		{
			name:          "given dir is subdir of invocation dir, when formatting, then returns relative path with ./ prefix (why: mirror the dir: path style)",
			dir:           "/home/user/project/infra",
			invocationDir: "/home/user/project",
			want:          "./infra",
		},
		{
			name:          "given dir is nested subdir, when formatting, then returns relative path with ./ prefix (why: still inside cwd)",
			dir:           "/home/user/project/infra/staging",
			invocationDir: "/home/user/project",
			want:          "./infra/staging",
		},
		{
			name:          "given dir is outside invocation dir, when formatting, then returns full path (why: relative path with .. is harder to read than absolute)",
			dir:           "/home/user/other",
			invocationDir: "/home/user/project",
			want:          "/home/user/other",
		},
		{
			name:          "given dir is parent of invocation dir, when formatting, then returns full path (why: not within cwd or its subdirs)",
			dir:           "/home/user",
			invocationDir: "/home/user/project",
			want:          "/home/user",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange (test fields are the arrangement)

			// Act
			got := displayDirPath(test.dir, test.invocationDir)

			// Assert
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func boolPtr(value bool) *bool { return &value }

func TestTaskInput_False_SkipsPrompts(t *testing.T) {
	// given task with input: false and get step with default,
	// when executed with noPrompts=false,
	// then get step uses default without prompting (why: task-level input overrides CLI flag)

	// Arrange
	dir := t.TempDir()
	cfg := &config.ConfigFile{
		TaskfileDir: dir,
		Tasks: map[string]config.Task{
			"t": {
				Interactive: boolPtr(false),
				Steps: []config.Step{
					{Kind: config.KindGet, GetEntries: []config.GetEntry{
						{VarName: "FOO", Info: "Enter foo", DefaultTmpl: "bar"},
					}},
				},
			},
		},
	}
	vars := map[string]value.Value{}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, false, dir)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["FOO"].String() != "bar" {
		t.Errorf("FOO: want %q, got %q", "bar", vars["FOO"].String())
	}
}

func TestTaskInput_False_PropagesToSubTasks(t *testing.T) {
	// given parent task with input: false calling child task with get step,
	// when executed, then child task also skips prompts
	// (why: input: false propagates down the entire call chain)

	// Arrange
	dir := t.TempDir()
	cfg := &config.ConfigFile{
		TaskfileDir: dir,
		Tasks: map[string]config.Task{
			"parent": {
				Interactive: boolPtr(false),
				Steps: []config.Step{
					{Kind: config.KindCall, CallTarget: "child"},
				},
			},
			"child": {
				Steps: []config.Step{
					{Kind: config.KindGet, GetEntries: []config.GetEntry{
						{VarName: "FOO", Info: "Enter foo", DefaultTmpl: "default-val"},
					}},
				},
			},
		},
	}
	vars := map[string]value.Value{}

	// Act
	err := ExecuteTask(context.Background(), "parent", makeScope(vars), cfg, false, dir)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskInput_False_OnChildOnly(t *testing.T) {
	// given parent without input set calling child with input: false,
	// when executed with noPrompts=true (to avoid actual prompting in test),
	// then child skips prompts (why: child's own input: false takes effect)

	// Arrange
	dir := t.TempDir()
	cfg := &config.ConfigFile{
		TaskfileDir: dir,
		Tasks: map[string]config.Task{
			"parent": {
				Steps: []config.Step{
					{Kind: config.KindCall, CallTarget: "child"},
				},
			},
			"child": {
				Interactive: boolPtr(false),
				Steps: []config.Step{
					{Kind: config.KindGet, GetEntries: []config.GetEntry{
						{VarName: "FOO", Info: "Enter foo", DefaultTmpl: "from-child"},
					}},
				},
			},
		},
	}
	vars := map[string]value.Value{}

	// Act
	err := ExecuteTask(context.Background(), "parent", makeScope(vars), cfg, true, dir)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskInput_False_NoDefault_ReturnsError(t *testing.T) {
	// given task with input: false and get step without default,
	// when executed, then returns error (why: can't satisfy prompt without interaction)

	// Arrange
	dir := t.TempDir()
	cfg := &config.ConfigFile{
		TaskfileDir: dir,
		Tasks: map[string]config.Task{
			"t": {
				Interactive: boolPtr(false),
				Steps: []config.Step{
					{Kind: config.KindGet, GetEntries: []config.GetEntry{
						{VarName: "FOO", Info: "Enter foo"},
					}},
				},
			},
		},
	}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(map[string]value.Value{}), cfg, false, dir)

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTaskIf_ConditionFalse_SkipsTask(t *testing.T) {
	// given task with if: that evaluates false, when executed,
	// then task skipped and no steps run (why: task-level guard prevents execution)

	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {
				IfExpr: `[ "{{.ENABLED}}" = "true" ]`,
				Steps: []config.Step{
					{Kind: config.KindSet, SetEntries: []config.SetEntry{
						{Key: "MARKER", ValTmpl: "ran"},
					}},
				},
			},
		},
	}
	vars := sv(map[string]string{"ENABLED": "false"})

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["MARKER"].String() == "ran" {
		t.Error("task should have been skipped but steps executed")
	}
}

func TestTaskIf_ConditionTrue_RunsTask(t *testing.T) {
	// given task with if: that evaluates true, when executed,
	// then steps run normally (why: guard passes, task proceeds)

	// Arrange
	dir := t.TempDir()
	cfg := &config.ConfigFile{
		TaskfileDir: dir,
		Tasks: map[string]config.Task{
			"t": {
				IfExpr: `[ "{{.ENABLED}}" = "true" ]`,
				Steps: []config.Step{
					{Kind: config.KindRun, Command: "echo hello"},
				},
			},
		},
	}
	vars := sv(map[string]string{"ENABLED": "true"})

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, dir)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskIf_NoCondition_RunsTask(t *testing.T) {
	// given task without if:, when executed,
	// then steps run normally (why: absence of guard means unconditional)

	// Arrange
	dir := t.TempDir()
	cfg := &config.ConfigFile{
		TaskfileDir: dir,
		Tasks: map[string]config.Task{
			"t": {
				Steps: []config.Step{
					{Kind: config.KindRun, Command: "echo hello"},
				},
			},
		},
	}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(map[string]value.Value{}), cfg, true, dir)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskIf_TemplateVarsResolved(t *testing.T) {
	// given task if: referencing scope var, when var matches condition,
	// then task runs (why: template rendering must work in task-level if:)

	// Arrange
	dir := t.TempDir()
	cfg := &config.ConfigFile{
		TaskfileDir: dir,
		Tasks: map[string]config.Task{
			"t": {
				IfExpr: `[ "{{.ENV}}" = "production" ]`,
				Steps: []config.Step{
					{Kind: config.KindRun, Command: "echo deploy"},
				},
			},
		},
	}
	vars := sv(map[string]string{"ENV": "production"})

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, dir)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskIf_BadCondition_ReturnsError(t *testing.T) {
	// given task with invalid if: template, when executed,
	// then returns error (why: malformed conditions must surface, not silently skip)

	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {
				IfExpr: `{{.MISSING_CLOSE_BRACE`,
				Steps: []config.Step{
					{Kind: config.KindRun, Command: "echo hello"},
				},
			},
		},
	}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(map[string]value.Value{}), cfg, true, t.TempDir())

	// Assert
	if err == nil {
		t.Fatal("expected error for malformed if condition, got nil")
	}
	if !strings.Contains(err.Error(), "task \"t\" if:") {
		t.Errorf("error should reference task name, got: %v", err)
	}
}

func TestTaskIf_EvaluatedInTaskDir(t *testing.T) {
	// given task with dir: and if: that checks a file relative to that dir,
	// when executed, then if: runs in the task's dir, not the caller's
	// (why: regression for if: being evaluated in the wrong working directory)

	// Arrange
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "marker"), nil, 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	cfg := &config.ConfigFile{
		TaskfileDir: root,
		Tasks: map[string]config.Task{
			"t": {
				Dir:    "sub",
				IfExpr: `[ -f marker ]`,
				Steps: []config.Step{
					{Kind: config.KindSet, SetEntries: []config.SetEntry{
						{Key: "MARKER", ValTmpl: "ran"},
					}},
				},
			},
		},
	}
	vars := map[string]value.Value{}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, root)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["MARKER"].String() != "ran" {
		t.Error("task should have run because marker exists in task dir, but was skipped")
	}
}

func TestExecuteSteps_CtxCancelledBetweenSteps_ReturnsErrInterrupted(t *testing.T) {
	// given ctx already cancelled before a step runs, when executeSteps
	// evaluates the between-step guard, then the error wraps ErrInterrupted
	// (why: must match execRun's wrapping so main.go's errors.Is check catches
	// cancellation that lands between steps, not just mid-command)

	// Arrange
	cfg := makeRunCfg("echo hello", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	err := ExecuteTask(ctx, "t", makeScope(map[string]value.Value{}), cfg, true, t.TempDir())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
}
