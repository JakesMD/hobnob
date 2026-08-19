package config

import (
	"os"
	"strings"
	"testing"
)

func TestParseConfig_TaskIfCondition(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/task_if.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	guarded := cfg.Tasks["guarded"]
	if guarded.IfExpr != `[ "{{.ENABLED}}" = "true" ]` {
		t.Errorf("guarded task IfExpr: got %q", guarded.IfExpr)
	}
	unguarded := cfg.Tasks["unguarded"]
	if unguarded.IfExpr != "" {
		t.Errorf("unguarded task should have no IfExpr, got %q", unguarded.IfExpr)
	}
}

func TestParseConfig_TaskInput(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/task_input.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	noPrompts := cfg.Tasks["no-prompts"]
	if noPrompts.Interactive == nil || *noPrompts.Interactive != false {
		t.Errorf("no-prompts task Input: want false, got %v", noPrompts.Interactive)
	}

	withPrompts := cfg.Tasks["with-prompts"]
	if withPrompts.Interactive == nil || *withPrompts.Interactive != true {
		t.Errorf("with-prompts task Input: want true, got %v", withPrompts.Interactive)
	}

	defaultInput := cfg.Tasks["default-input"]
	if defaultInput.Interactive != nil {
		t.Errorf("default-input task Input: want nil, got %v", defaultInput.Interactive)
	}
}

func TestParseConfig_EnvBlock(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/env_block.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	wantPaths := []string{".env", "./scripts/setup.sh"}
	if len(cfg.EnvFileTmpls) != len(wantPaths) {
		t.Fatalf("EnvFileTmpls len: got %d, want %d", len(cfg.EnvFileTmpls), len(wantPaths))
	}
	for i, wantPath := range wantPaths {
		if cfg.EnvFileTmpls[i].PathTmpl != wantPath {
			t.Errorf("EnvFileTmpls[%d].PathTmpl: got %q, want %q", i, cfg.EnvFileTmpls[i].PathTmpl, wantPath)
		}
		if cfg.EnvFileTmpls[i].SecretOverride != nil {
			t.Errorf("EnvFileTmpls[%d].SecretOverride: got %v, want nil (no explicit secret: given)", i, *cfg.EnvFileTmpls[i].SecretOverride)
		}
	}
}

func TestParseConfig_EnvBlockSecretOverride(t *testing.T) {
	// given an env: entry in expanded form with an explicit secret: override, when parsed, then SecretOverride reflects the given value (why: lets callers opt individual files in or out of the default secret status)
	// Arrange
	cfg, err := ParseConfig("testdata/env_block_secret_override.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(cfg.EnvFileTmpls) != 2 {
		t.Fatalf("EnvFileTmpls len: got %d, want 2", len(cfg.EnvFileTmpls))
	}
	entry := cfg.EnvFileTmpls[0]
	if entry.PathTmpl != ".env" {
		t.Errorf("EnvFileTmpls[0].PathTmpl: got %q, want %q", entry.PathTmpl, ".env")
	}
	if entry.SecretOverride == nil || *entry.SecretOverride != false {
		t.Errorf("EnvFileTmpls[0].SecretOverride: got %v, want false", entry.SecretOverride)
	}
	entry = cfg.EnvFileTmpls[1]
	if entry.PathTmpl != "setup.sh" {
		t.Errorf("EnvFileTmpls[1].PathTmpl: got %q, want %q", entry.PathTmpl, "setup.sh")
	}
	if entry.SecretOverride == nil || *entry.SecretOverride != true {
		t.Errorf("EnvFileTmpls[1].SecretOverride: got %v, want true", entry.SecretOverride)
	}
}

func TestParseConfig_EnvBlockMalformedMultiKeyEntryErrors(t *testing.T) {
	// given an env: sequence item with two sibling path keys (a mis-indented mapping), when parsed, then an error is returned (why: silently keeping only the first pair would drop a declared file with no warning)
	// Arrange
	cfg, err := ParseConfig("testdata/env_block_malformed_multikey.yml")

	// Act + Assert
	if err == nil {
		t.Fatalf("expected parse error, got cfg with EnvFileTmpls=%v", cfg.EnvFileTmpls)
	}
}

func TestParseConfig_MissingTask(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/run_step.yml")

	// Act
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	_, ok := cfg.Tasks["nonexistent"]

	// Assert
	if ok {
		t.Error("expected missing task to not be found")
	}
}

func TestParseConfig_MissingFile(t *testing.T) {
	// Arrange + Act
	_, err := ParseConfig("testdata/does_not_exist.yml")

	// Assert
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestParseConfig_EmptyOrNullDocument(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
	}{
		{
			name:    "given zero-byte file, when parsing, then returns config with no tasks",
			fixture: "testdata/empty_file.yml",
		},
		{
			name:    "given comment-only file, when parsing, then returns config with no tasks",
			fixture: "testdata/comment_only.yml",
		},
		{
			name:    "given file containing only null, when parsing, then returns config with no tasks",
			fixture: "testdata/null_root.yml",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (fixture path is the arrangement)

			// Act
			cfg, err := ParseConfig(test.fixture)

			// Assert
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			if len(cfg.TaskNames) != 0 {
				t.Errorf("TaskNames: got %v, want empty", cfg.TaskNames)
			}
			if len(cfg.Vars) != 0 {
				t.Errorf("Vars: got %v, want empty", cfg.Vars)
			}
			if len(cfg.Modules) != 0 {
				t.Errorf("Modules: got %v, want empty", cfg.Modules)
			}
		})
	}
}

func TestParseConfig_EmptyBlocks(t *testing.T) {
	// Arrange
	// (testdata/empty_blocks.yml has bare vars:/modules:/tasks: keys)

	// Act
	cfg, err := ParseConfig("testdata/empty_blocks.yml")

	// Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(cfg.TaskNames) != 0 {
		t.Errorf("TaskNames: got %v, want empty", cfg.TaskNames)
	}
	if len(cfg.Vars) != 0 {
		t.Errorf("Vars: got %v, want empty", cfg.Vars)
	}
	if len(cfg.Modules) != 0 {
		t.Errorf("Modules: got %v, want empty", cfg.Modules)
	}
}

func TestParseConfig_TaskInfo(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/task_info.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	tests := []struct {
		name     string
		taskName string
		wantInfo string
	}{
		{
			name:     "given task with template info, when parsed, then raw template stored",
			taskName: "adopt",
			wantInfo: "Adopt a {{.ANIMAL}} from the shelter",
		},
		{
			name:     "given task with plain info, when parsed, then info stored",
			taskName: "list_animals",
			wantInfo: "List all available animals",
		},
		{
			name:     "given task with no info, when parsed, then info is empty",
			taskName: "no_info",
			wantInfo: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			task, ok := cfg.Tasks[test.taskName]
			if !ok {
				t.Fatalf("task %q not found", test.taskName)
			}

			// Act (none — parsing already done)

			// Assert
			if task.Info != test.wantInfo {
				t.Errorf("info: got %q, want %q", task.Info, test.wantInfo)
			}
		})
	}
}

func TestParseConfig_TaskNames(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/task_info.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act (none — parsing already done)

	// Assert — declaration order preserved
	want := []string{"adopt", "list_animals", "no_info"}
	if len(cfg.TaskNames) != len(want) {
		t.Fatalf("TaskNames len: got %d, want %d: %v", len(cfg.TaskNames), len(want), cfg.TaskNames)
	}
	for i, wantName := range want {
		if cfg.TaskNames[i] != wantName {
			t.Errorf("TaskNames[%d]: got %q, want %q", i, cfg.TaskNames[i], wantName)
		}
	}
}

func TestParseConfig_ModulesBlock(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/modules_parent.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if len(cfg.Modules) != 2 {
		t.Fatalf("Modules len: got %d, want 2", len(cfg.Modules))
	}

	tests := []struct {
		name        string
		wantPrefix  string
		wantFile    string
		wantShow    []string
		wantHide    []string
		wantFlatten string
	}{
		{
			name:       "given internal module, when parsed, then prefix and file stored",
			wantPrefix: "_farm",
			wantFile:   "modules_farm.yml",
		},
		{
			name:        "given public module with show/hide/flatten, when parsed, then all fields stored",
			wantPrefix:  "yard",
			wantFile:    "modules_yard.yml",
			wantShow:    []string{"clean", "fix"},
			wantHide:    []string{"die"},
			wantFlatten: "true",
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			module := cfg.Modules[i]

			// Act (none)

			// Assert
			if module.Prefix != test.wantPrefix {
				t.Errorf("Prefix: got %q, want %q", module.Prefix, test.wantPrefix)
			}
			if module.FileTmpl != test.wantFile {
				t.Errorf("FileTmpl: got %q, want %q", module.FileTmpl, test.wantFile)
			}
			if len(module.ShowTmpls) != len(test.wantShow) {
				t.Fatalf("ShowTmpls len: got %d, want %d", len(module.ShowTmpls), len(test.wantShow))
			}
			for j, wantItem := range test.wantShow {
				if module.ShowTmpls[j] != wantItem {
					t.Errorf("ShowTmpls[%d]: got %q, want %q", j, module.ShowTmpls[j], wantItem)
				}
			}
			if len(module.HideTmpls) != len(test.wantHide) {
				t.Fatalf("HideTmpls len: got %d, want %d", len(module.HideTmpls), len(test.wantHide))
			}
			for j, wantItem := range test.wantHide {
				if module.HideTmpls[j] != wantItem {
					t.Errorf("HideTmpls[%d]: got %q, want %q", j, module.HideTmpls[j], wantItem)
				}
			}
			if module.FlattenTmpl != test.wantFlatten {
				t.Errorf("FlattenTmpl: got %q, want %q", module.FlattenTmpl, test.wantFlatten)
			}
		})
	}
}

func TestParseConfig_DirField(t *testing.T) {
	cfg, err := ParseConfig("testdata/dir_field.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	tests := []struct {
		name        string
		taskName    string
		wantTaskDir string
		stepIdx     int
		wantStepDir string
		wantCallTgt string
	}{
		{
			name:        "given run step with dir, when parsed, then DirTmpl stored on step",
			taskName:    "run_with_dir",
			wantTaskDir: "",
			stepIdx:     0,
			wantStepDir: "/tmp/custom",
		},
		{
			name:        "given task with dir, when parsed, then Dir stored on task",
			taskName:    "task_with_dir",
			wantTaskDir: "/tmp/task",
			stepIdx:     0,
			wantStepDir: "",
		},
		{
			name:        "given task with template dir, when parsed, then Dir template stored",
			taskName:    "task_with_template_dir",
			wantTaskDir: "{{.BASE_DIR}}/sub",
			stepIdx:     0,
			wantStepDir: "",
		},
		{
			name:        "given call step with dir, when parsed, then DirTmpl stored on step",
			taskName:    "call_with_dir",
			wantTaskDir: "",
			stepIdx:     0,
			wantStepDir: "/tmp/override",
			wantCallTgt: "task_with_dir",
		},
		{
			name:        "given run step with bare var dir, when parsed, then DirTmpl normalized to template (why: dir: .VAR should work like dir: '{{.VAR}}')",
			taskName:    "run_with_bare_var_dir",
			wantTaskDir: "",
			stepIdx:     0,
			wantStepDir: "{{.CUSTOM_DIR}}",
		},
		{
			name:        "given task with bare var dir, when parsed, then Dir normalized to template",
			taskName:    "task_with_bare_var_dir",
			wantTaskDir: "{{.BASE_DIR}}",
			stepIdx:     0,
			wantStepDir: "",
		},
		{
			name:        "given task with relative path dir, when parsed, then Dir kept literal (why: ./relative starts with '.' but is not a var ref)",
			taskName:    "task_with_relative_dir",
			wantTaskDir: "./relative/path",
			stepIdx:     0,
			wantStepDir: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			task, ok := cfg.Tasks[test.taskName]
			if !ok {
				t.Fatalf("task %q not found", test.taskName)
			}

			// Act (none — parsing already done)

			// Assert
			if task.Dir != test.wantTaskDir {
				t.Errorf("task Dir: got %q, want %q", task.Dir, test.wantTaskDir)
			}
			if test.stepIdx >= len(task.Steps) {
				t.Fatalf("step[%d] missing", test.stepIdx)
			}
			step := task.Steps[test.stepIdx]
			if step.DirTmpl != test.wantStepDir {
				t.Errorf("step DirTmpl: got %q, want %q", step.DirTmpl, test.wantStepDir)
			}
			if test.wantCallTgt != "" && step.CallTarget != test.wantCallTgt {
				t.Errorf("step CallTarget: got %q, want %q", step.CallTarget, test.wantCallTgt)
			}
		})
	}
}

func TestParseConfig_TemplateInVarName_Rejected(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "given template syntax in set key, when parsed, then returns error (why: var names must be static identifiers)",
			yaml: `tasks:
  t:
    steps:
      - set:
          - '{{.FOO}}': hello`,
			wantErr: "must not contain template syntax",
		},
		{
			name: "given template syntax in into key, when parsed, then returns error (why: var names must be static identifiers)",
			yaml: `tasks:
  t:
    steps:
      - run: echo hi
        into:
          - '{{.OUT}}': stdout`,
			wantErr: "must not contain template syntax",
		},
		{
			name: "given template syntax in get bare var name, when parsed, then returns error (why: var names must be static identifiers)",
			yaml: `tasks:
  t:
    steps:
      - get:
          - '{{.VAR}}'`,
			wantErr: "must not contain template syntax",
		},
		{
			name: "given template syntax in get mapping var name, when parsed, then returns error (why: var names must be static identifiers)",
			yaml: `tasks:
  t:
    steps:
      - get:
          - '{{.VAR}}':
              info: test`,
			wantErr: "must not contain template syntax",
		},
		{
			name: "given template syntax in for matrix var name, when parsed, then returns error (why: var names must be static identifiers)",
			yaml: `tasks:
  t:
    steps:
      - loop:
          '{{.VAR}}': [a, b]
        steps:
          - run: echo done`,
			wantErr: "must not contain template syntax",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			f, err := os.CreateTemp("", "hobnob-test-*.yml")
			if err != nil {
				t.Fatalf("temp file: %v", err)
			}
			defer os.Remove(f.Name())
			if _, err := f.WriteString(test.yaml); err != nil {
				t.Fatalf("write: %v", err)
			}
			f.Close()

			// Act
			_, err = ParseConfig(f.Name())

			// Assert
			if err == nil {
				t.Fatal("expected parse error, got nil")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), test.wantErr)
			}
		})
	}
}
