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

	"hobnob/internal/cli"
	"hobnob/internal/config"
)

func TestExecCall_SecretPassedThroughWith_StaysMaskedUnderNewName(t *testing.T) {
	// given a parent-scope secret passed into a child via with: under a different name,
	// when the child displays a run: command referencing the new name, then the value is
	// still masked (why: this is why with: needs no secret: of its own — Copy() carries
	// the parent's secrets across and masking matches on value, not key)
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"parent": {Steps: []config.Step{
				{
					Kind:       config.KindCall,
					CallTarget: "child",
					CallVars:   []config.SetEntry{{Key: "TOKEN", ValTmpl: "{{.VAULT_TOKEN}}"}},
				},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindRun, Command: "echo {{.TOKEN}}"},
			}},
		},
	}
	scope := &cli.Scope{
		Vars:    map[string]string{"VAULT_TOKEN": "s3cr3t-value"},
		Secrets: map[string]bool{"VAULT_TOKEN": true},
	}

	// Act
	output := captureStdout(t, func() {
		if err := ExecuteTask(context.Background(), "parent", scope, cfg, true, t.TempDir()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Assert — check only the "run:" announcement line: the command's own stdout
	// (the literal echo of the secret) is a separate, expected concern unrelated
	// to whether the displayed command text is masked.
	displayLine, _, _ := strings.Cut(output, "\n")
	if strings.Contains(displayLine, "s3cr3t-value") {
		t.Errorf("run: display line leaked secret value unmasked: %q", displayLine)
	}
	if !strings.Contains(displayLine, "****") {
		t.Errorf("run: display line missing mask marker: %q", displayLine)
	}
}

func TestExecCall_ComposedSecretThroughWith_MasksOnlySecretComponent(t *testing.T) {
	// given a with: value composing a secret and a non-secret var, when the child displays
	// the run: command, then only the secret component is masked (why: value-based masking
	// keeps the useful half of the line readable — a with:-level secret: flag would blank
	// the whole composed string, which is why it isn't offered)
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"parent": {Steps: []config.Step{
				{
					Kind:       config.KindCall,
					CallTarget: "child",
					CallVars:   []config.SetEntry{{Key: "DSN", ValTmpl: "postgres://{{.USER}}:{{.PASS}}@db"}},
				},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindRun, Command: "true # {{.DSN}}"},
			}},
		},
	}
	scope := &cli.Scope{
		Vars:    map[string]string{"USER": "admin", "PASS": "hunter2"},
		Secrets: map[string]bool{"PASS": true},
	}

	// Act
	output := captureStdout(t, func() {
		if err := ExecuteTask(context.Background(), "parent", scope, cfg, true, t.TempDir()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Assert
	displayLine, _, _ := strings.Cut(output, "\n")
	if !strings.Contains(displayLine, "postgres://admin:****@db") {
		t.Errorf("expected only the password masked, got: %q", displayLine)
	}
}

func TestDir_CallStep_Priorities(t *testing.T) {
	parentDir := t.TempDir()
	taskDir := t.TempDir()
	callStepDir := t.TempDir()

	outFile := filepath.Join(t.TempDir(), "out.txt")

	childSteps := []config.Step{
		{Kind: config.KindRun, Command: fmt.Sprintf("pwd > '%s'", outFile)},
	}

	tests := []struct {
		name        string
		callDirTmpl string
		taskDirTmpl string
		wantDir     string
	}{
		{
			name:        "given no call dir and no task dir, when called, then child inherits parentDir (why: Priority C fallback)",
			callDirTmpl: "",
			taskDirTmpl: "",
			wantDir:     realPath(t, parentDir),
		},
		{
			name:        "given no call dir but task has dir, when called, then child uses task dir (why: Priority B)",
			callDirTmpl: "",
			taskDirTmpl: taskDir,
			wantDir:     realPath(t, taskDir),
		},
		{
			name:        "given call step dir and task also has dir, when called, then child uses call step dir (why: Priority A overrides B)",
			callDirTmpl: callStepDir,
			taskDirTmpl: taskDir,
			wantDir:     realPath(t, callStepDir),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg := &config.ConfigFile{
				Tasks: map[string]config.Task{
					"parent": {Steps: []config.Step{
						{Kind: config.KindCall, CallTarget: "child", DirTmpl: test.callDirTmpl},
					}},
					"child": {Dir: test.taskDirTmpl, Steps: childSteps},
				},
				TaskfileDir: t.TempDir(),
			}

			// Act
			if err := ExecuteTask(context.Background(), "parent", makeScope(map[string]string{}), cfg, true, parentDir); err != nil {
				t.Fatalf("ExecuteTask: %v", err)
			}
			data, err := os.ReadFile(outFile)
			if err != nil {
				t.Fatalf("read out file: %v", err)
			}

			// Assert
			got := strings.TrimRight(string(data), "\n")
			if got != test.wantDir {
				t.Errorf("child run dir: got %q, want %q", got, test.wantDir)
			}
		})
	}
}

func TestExecCall_IntoTemplatePath(t *testing.T) {
	// given child sets LOG_FILE, when into: uses plain entry then template entry,
	// then template entry resolves against the already-mapped LOG_PATH value
	// (why: sequential into: resolution must see prior entries in same block)
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"parent": {Steps: []config.Step{
				{
					Kind:       config.KindCall,
					CallTarget: "child",
					IntoEntries: []config.IntoEntry{
						{ParentKey: "LOG_PATH", ValueTmpl: "LOG_FILE"},
						{ParentKey: "ARCHIVE_PATH", ValueTmpl: "{{.LOG_PATH}}/archive"},
					},
				},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindSet, SetEntries: []config.SetEntry{
					{Key: "LOG_FILE", ValTmpl: "/var/log/app.log"},
				}},
			}},
		},
	}
	vars := map[string]string{}

	// Act
	err := ExecuteTask(context.Background(), "parent", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["LOG_PATH"] != "/var/log/app.log" {
		t.Errorf("LOG_PATH: got %q, want /var/log/app.log", vars["LOG_PATH"])
	}
	if vars["ARCHIVE_PATH"] != "/var/log/app.log/archive" {
		t.Errorf("ARCHIVE_PATH: got %q, want /var/log/app.log/archive", vars["ARCHIVE_PATH"])
	}
}

func TestExecCall_Integration_ParentChildInto(t *testing.T) {
	// given parent passes INPUT via with:, when child sets OUTPUT and CHILD_ONLY,
	// then RESULT is mapped back via into: and CHILD_ONLY does not leak
	// (why: copy-on-write scoping must isolate child mutations from parent)
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"parent": {Steps: []config.Step{
				{Kind: config.KindSet, SetEntries: []config.SetEntry{
					{Key: "INPUT", ValTmpl: "hello"},
				}},
				{
					Kind:       config.KindCall,
					CallTarget: "child",
					CallVars:   []config.SetEntry{{Key: "MSG", ValTmpl: "{{.INPUT}}"}},
					IntoEntries: []config.IntoEntry{
						{ParentKey: "RESULT", ValueTmpl: "OUTPUT"},
					},
				},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindSet, SetEntries: []config.SetEntry{
					{Key: "OUTPUT", ValTmpl: "{{.MSG}}_processed"},
					{Key: "CHILD_ONLY", ValTmpl: "should_not_leak"},
				}},
			}},
		},
	}
	vars := map[string]string{}

	// Act
	err := ExecuteTask(context.Background(), "parent", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["RESULT"] != "hello_processed" {
		t.Errorf("RESULT: got %q, want hello_processed", vars["RESULT"])
	}
	if _, exists := vars["CHILD_ONLY"]; exists {
		t.Errorf("CHILD_ONLY leaked into parent scope: %q", vars["CHILD_ONLY"])
	}
	if vars["OUTPUT"] != "" {
		t.Errorf("OUTPUT leaked into parent scope: %q", vars["OUTPUT"])
	}
}

func TestExecCall_Into_DotPrefixStripped(t *testing.T) {
	// given into: uses dot-prefixed value (the intended syntax, e.g. ".OUTPUT"),
	// when call completes, then parent receives child's value correctly
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"parent": {Steps: []config.Step{
				{
					Kind:       config.KindCall,
					CallTarget: "child",
					IntoEntries: []config.IntoEntry{
						{ParentKey: "RESULT", ValueTmpl: ".OUTPUT"},
					},
				},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindSet, SetEntries: []config.SetEntry{
					{Key: "OUTPUT", ValTmpl: "ok"},
				}},
			}},
		},
	}
	vars := map[string]string{}

	// Act
	err := ExecuteTask(context.Background(), "parent", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["RESULT"] != "ok" {
		t.Errorf("RESULT: got %q, want ok", vars["RESULT"])
	}
}

func TestExecCall_Into_NestedObject(t *testing.T) {
	// given a nested into: object mixing a bare .FIELD leaf and a {{}} template
	// leaf, when call completes, then both leaf grammars resolve into one
	// assembled JSON var (why: into:'s nested form reuses its existing dual-mode
	// leaf grammar — .FIELD childScope lookup or {{}} template — it doesn't
	// introduce a third one)
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"parent": {Steps: []config.Step{
				{Kind: config.KindSet, SetEntries: []config.SetEntry{
					{Key: "PREFIX", ValTmpl: "log"},
				}},
				{
					Kind:       config.KindCall,
					CallTarget: "child",
					IntoEntries: []config.IntoEntry{
						{ParentKey: "CUSTOM", ValNode: &config.JSONNode{
							Kind: config.JSONObject,
							Fields: []config.JSONField{
								{Key: "output", Node: config.JSONNode{Kind: config.JSONString, Tmpl: ".OUTPUT"}},
								{Key: "label", Node: config.JSONNode{Kind: config.JSONString, Tmpl: "{{.PREFIX}}-entry"}},
							},
						}},
					},
				},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindSet, SetEntries: []config.SetEntry{
					{Key: "OUTPUT", ValTmpl: "ok"},
				}},
			}},
		},
	}
	vars := map[string]string{}

	// Act
	err := ExecuteTask(context.Background(), "parent", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{"label":"log-entry","output":"ok"}`
	if vars["CUSTOM"] != want {
		t.Errorf("CUSTOM: got %q, want %q", vars["CUSTOM"], want)
	}
}

func TestExecCall_With_MapLiteralTemplateLeaf_QuoteEscaped(t *testing.T) {
	// given a with: map literal whose template leaf evaluates to a value
	// containing a double quote, when the child reads it back via pluck, then
	// the quote is escaped by json.Marshal rather than corrupting the JSON
	// (why: with: shares parseSetNode with set:, so it inherited the same
	// marshal-before-eval injection bug — this pins the fix on that side too)
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"parent": {Steps: []config.Step{
				{Kind: config.KindSet, SetEntries: []config.SetEntry{
					{Key: "NAME", ValTmpl: `he said "hi"`},
				}},
				{
					Kind:       config.KindCall,
					CallTarget: "child",
					CallVars: []config.SetEntry{
						{Key: "CUSTOM", ValNode: &config.JSONNode{
							Kind: config.JSONObject,
							Fields: []config.JSONField{
								{Key: "name", Node: config.JSONNode{Kind: config.JSONString, Tmpl: "{{.NAME}}"}},
							},
						}},
					},
					IntoEntries: []config.IntoEntry{
						{ParentKey: "RESULT", ValueTmpl: ".PLUCKED"},
					},
				},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindSet, SetEntries: []config.SetEntry{
					{Key: "PLUCKED", ValTmpl: `{{ .CUSTOM | pluck "name" }}`},
				}},
			}},
		},
	}
	vars := map[string]string{}

	// Act
	err := ExecuteTask(context.Background(), "parent", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `he said "hi"`
	if vars["RESULT"] != want {
		t.Errorf("RESULT: got %q, want %q", vars["RESULT"], want)
	}
}

func TestDir_CallStep_DirUsesWithVars(t *testing.T) {
	// given call step has dir: template referencing a with: variable,
	// when called, then dir resolves using the with: variable's value
	// (why: childScope is populated with with: vars before dir: is evaluated)
	targetDir := t.TempDir()
	outFile := filepath.Join(t.TempDir(), "out.txt")

	cfg := &config.ConfigFile{
		TaskfileDir: t.TempDir(),
		Tasks: map[string]config.Task{
			"parent": {Steps: []config.Step{
				{
					Kind:       config.KindCall,
					CallTarget: "child",
					DirTmpl:    "{{.TARGET_DIR}}",
					CallVars:   []config.SetEntry{{Key: "TARGET_DIR", ValTmpl: targetDir}},
				},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindRun, Command: fmt.Sprintf("pwd > '%s'", outFile)},
			}},
		},
	}

	// Act
	if err := ExecuteTask(context.Background(), "parent", makeScope(map[string]string{}), cfg, true, t.TempDir()); err != nil {
		t.Fatalf("ExecuteTask: %v", err)
	}

	// Assert
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read out file: %v", err)
	}
	got := strings.TrimRight(string(data), "\n")
	want := realPath(t, targetDir)
	if got != want {
		t.Errorf("child run dir: got %q, want %q", got, want)
	}
}

func TestDir_CallStep_RelativeToCallerDir(t *testing.T) {
	// given call step has a relative dir: and the called task is from a different module,
	// when called, then the relative path resolves against the caller's file directory
	// (why: dir: on a call step is written in the caller's file and should be relative to it)
	callerDir := t.TempDir()
	calleeDir := t.TempDir()
	subDir := filepath.Join(callerDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	outFile := filepath.Join(t.TempDir(), "out.txt")

	calleeCfg := &config.ConfigFile{
		TaskfileDir: calleeDir,
		Tasks:       map[string]config.Task{},
	}
	childTask := config.Task{
		Cfg: calleeCfg,
		Steps: []config.Step{
			{Kind: config.KindRun, Command: fmt.Sprintf("pwd > '%s'", outFile)},
		},
	}
	calleeCfg.Tasks["child"] = childTask

	callerCfg := &config.ConfigFile{
		TaskfileDir: callerDir,
		Tasks: map[string]config.Task{
			"parent": {Steps: []config.Step{
				{Kind: config.KindCall, CallTarget: "child", DirTmpl: "sub"},
			}},
			"child": childTask,
		},
	}

	// Act
	if err := ExecuteTask(context.Background(), "parent", makeScope(map[string]string{}), callerCfg, true, t.TempDir()); err != nil {
		t.Fatalf("ExecuteTask: %v", err)
	}

	// Assert
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read out file: %v", err)
	}
	got := strings.TrimRight(string(data), "\n")
	want := realPath(t, subDir)
	if got != want {
		t.Errorf("child run dir: got %q, want %q (expected relative to caller, not callee)", got, want)
	}
}

func TestCallStep_Interactive_False_DisablesPrompts(t *testing.T) {
	// given call step with interactive: false,
	// when child task has get step with default,
	// then uses default without prompting (why: step-level interactive overrides for that sub-tree)

	// Arrange
	dir := t.TempDir()
	cfg := &config.ConfigFile{
		TaskfileDir: dir,
		Tasks: map[string]config.Task{
			"parent": {
				Steps: []config.Step{
					{Kind: config.KindCall, CallTarget: "child", Interactive: boolPtr(false)},
				},
			},
			"child": {
				Steps: []config.Step{
					{Kind: config.KindGet, GetEntries: []config.GetEntry{
						{VarName: "FOO", Info: "Enter foo", DefaultTmpl: "silent-val"},
					}},
				},
			},
		},
	}

	// Act
	err := ExecuteTask(context.Background(), "parent", makeScope(map[string]string{}), cfg, false, dir)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCallStep_Interactive_False_PropagatesDown(t *testing.T) {
	// given call step with interactive: false calling task that calls another task,
	// when grandchild has get step, then grandchild also skips prompts
	// (why: interactive: false on call step propagates through entire sub-tree)

	// Arrange
	dir := t.TempDir()
	cfg := &config.ConfigFile{
		TaskfileDir: dir,
		Tasks: map[string]config.Task{
			"root": {
				Steps: []config.Step{
					{Kind: config.KindCall, CallTarget: "mid", Interactive: boolPtr(false)},
				},
			},
			"mid": {
				Steps: []config.Step{
					{Kind: config.KindCall, CallTarget: "leaf"},
				},
			},
			"leaf": {
				Steps: []config.Step{
					{Kind: config.KindGet, GetEntries: []config.GetEntry{
						{VarName: "X", Info: "val", DefaultTmpl: "deep"},
					}},
				},
			},
		},
	}

	// Act
	err := ExecuteTask(context.Background(), "root", makeScope(map[string]string{}), cfg, false, dir)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecCall_Soft_DoesNotSwallowInterrupt(t *testing.T) {
	// given a soft: true call step whose child task is interrupted by ctx
	// cancellation, when executed, then the interrupt still propagates (why:
	// soft: true is meant to tolerate ordinary command failures, not mask a
	// user-requested shutdown as false success)

	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{Kind: config.KindCall, CallTarget: "child", Soft: true},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindRun, Command: "sleep 5"},
			}},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Act
	err := ExecuteTask(ctx, "t", makeScope(map[string]string{}), cfg, true, t.TempDir())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted to survive soft:true, got: %v", err)
	}
}

func TestExecCall_Soft_SwallowsOrdinaryError(t *testing.T) {
	// given a soft: true call step whose child task fails with an ordinary
	// command error, when executed, then the error is swallowed and execution
	// continues (why: regression guard for the pre-existing soft: true
	// behavior — only ctx-cancellation errors should bypass it)

	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{Kind: config.KindCall, CallTarget: "child", Soft: true},
				{Kind: config.KindSet, SetEntries: []config.SetEntry{{Key: "AFTER", ValTmpl: "reached"}}},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindRun, Command: "exit 1"},
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
	if vars["AFTER"] != "reached" {
		t.Errorf("expected execution to continue past soft-failed call, AFTER = %q", vars["AFTER"])
	}
}
