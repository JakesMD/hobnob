package config

import (
	"os"
	"strings"
	"testing"
)

func TestParseConfig_RunStep(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		taskName string
		want     []Step
	}{
		{
			name:     "given run-only task, when parsing, then returns run steps with if condition",
			fixture:  "testdata/run_step.yml",
			taskName: "greet",
			want: []Step{
				{Kind: KindRun, Command: `echo "hello world"`},
				{Kind: KindRun, Command: `echo "port {{.PORT}}"`, IfExpr: `{{.PORT}} == "8080"`},
				{Kind: KindRun, Command: `{{.GREETING}}`},
				{Kind: KindRun, Command: `./greet.sh`},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			cfg, err := ParseConfig(tc.fixture)

			// Assert
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			tsk, ok := cfg.Tasks[tc.taskName]
			if !ok {
				t.Fatalf("task %q not found", tc.taskName)
			}
			assertStepsEqual(t, tsk.Steps, tc.want)
		})
	}
}

func TestParseConfig_SetStep(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/set_step.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	tsk, ok := cfg.Tasks["setup"]
	if !ok {
		t.Fatal("task 'setup' not found")
	}
	steps := tsk.Steps
	if len(steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(steps))
	}
	s := steps[0]
	if s.Kind != KindSet {
		t.Fatalf("want KindSet, got %v", s.Kind)
	}
	wantEntries := []SetEntry{
		{Key: "LIMIT", ValTmpl: "1048576"},
		{Key: "LABEL", ValTmpl: `{{ run "echo hobnob" }}`},
		{Key: "DERIVED", ValTmpl: "{{.LIMIT}}/bytes"},
		{Key: "ALIAS", ValTmpl: "{{.LIMIT}}"},
	}
	if len(s.SetEntries) != len(wantEntries) {
		t.Fatalf("want %d set entries, got %d", len(wantEntries), len(s.SetEntries))
	}
	for i, w := range wantEntries {
		t.Run(w.Key, func(t *testing.T) {
			// Arrange
			got := s.SetEntries[i]

			// Act (none — just assertion)

			// Assert
			if got.Key != w.Key {
				t.Errorf("key[%d]: got %q, want %q", i, got.Key, w.Key)
			}
			if got.ValTmpl != w.ValTmpl {
				t.Errorf("val[%d]: got %q, want %q", i, got.ValTmpl, w.ValTmpl)
			}
		})
	}
}

func TestParseConfig_SetStep_ListValue(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/set_list_value.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	tsk, ok := cfg.Tasks["setup"]
	if !ok {
		t.Fatal("task 'setup' not found")
	}
	steps := tsk.Steps
	if len(steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(steps))
	}
	entries := steps[0].SetEntries
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}

	tests := []struct {
		name    string
		key     string
		wantVal string
	}{
		{
			name:    "given YAML sequence value, when parsed, then serialized as JSON array (why: scope is map[string]string; JSON round-trips through parseList)",
			key:     "PACKAGES",
			wantVal: `["packages/data/platform_client/","packages/data/database_client/","packages/data/auth_client/"]`,
		},
		{
			name:    "given scalar value, when parsed, then stored as-is (why: non-sequence path unchanged)",
			key:     "SCALAR",
			wantVal: "just a string",
		},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			got := entries[i]

			// Act (none)

			// Assert
			if got.Key != tc.key {
				t.Errorf("key: got %q, want %q", got.Key, tc.key)
			}
			if got.ValTmpl != tc.wantVal {
				t.Errorf("val: got %q, want %q", got.ValTmpl, tc.wantVal)
			}
		})
	}
}


func TestParseConfig_GetStep(t *testing.T) {
	cfg, err := ParseConfig("testdata/get_step.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	tests := []struct {
		name        string
		taskName    string
		wantEntries []GetEntry
	}{
		{
			name:     "given text get, when parsing, then returns single entry with info",
			taskName: "get_text",
			wantEntries: []GetEntry{
				{VarName: "NAME", Info: "Enter your name"},
			},
		},
		{
			name:     "given get with check, when parsing, then returns check expression",
			taskName: "get_text_check",
			wantEntries: []GetEntry{
				{VarName: "SIZE", Info: "Enter size", Check: "[ {{.SIZE}} -le 100 ]"},
			},
		},
		{
			name:     "given get with default, when parsing, then returns defaultTmpl",
			taskName: "get_text_default",
			wantEntries: []GetEntry{
				{VarName: "COLOR", Info: "Enter color", DefaultTmpl: "blue"},
			},
		},
		{
			name:     "given get with bare var default, when parsing, then defaultTmpl normalized to template (why: default: .VAR should work like default: '{{.VAR}}')",
			taskName: "get_text_default_from_var",
			wantEntries: []GetEntry{
				{VarName: "COLOR", Info: "Enter color", DefaultTmpl: "{{.FALLBACK}}"},
			},
		},
		{
			name:     "given get with bare var pipe default, when parsing, then defaultTmpl normalized to template (why: default: .VAR | filter should work like default: '{{.VAR | filter}}')",
			taskName: "get_text_default_from_var_pipe",
			wantEntries: []GetEntry{
				{VarName: "ITEM", Info: "Pick item", DefaultTmpl: "{{.MY_LIST | first}}"},
			},
		},
		{
			name:     "given select get, when parsing, then returns fromList",
			taskName: "get_select",
			wantEntries: []GetEntry{
				{VarName: "MOTOR", Info: "Select motor", FromList: []string{"X-Axis", "Y-Axis", "Z-Axis"}},
			},
		},
		{
			name:     "given select with default, when parsing, then returns defaultTmpl and fromList",
			taskName: "get_select_default",
			wantEntries: []GetEntry{
				{VarName: "MOTOR", Info: "Select motor", DefaultTmpl: "Y-Axis", FromList: []string{"X-Axis", "Y-Axis", "Z-Axis"}},
			},
		},
		{
			name:     "given multi get, when parsing, then returns multi flag",
			taskName: "get_multi",
			wantEntries: []GetEntry{
				{VarName: "TAGS", Info: "Pick tags", Multi: true, FromList: []string{"alpha", "beta", "gamma"}},
			},
		},
		{
			name:     "given two-var get, when parsing, then returns both entries in order",
			taskName: "get_multi_vars",
			wantEntries: []GetEntry{
				{VarName: "NAME", Info: "Enter name"},
				{VarName: "SIZE", Info: "Enter size", Check: "[ {{.SIZE}} -le 100 ]"},
			},
		},
		{
			name:     "given bare variable name, when parsing, then returns entry with no modifiers (why: shorthand for vars needing no info/default)",
			taskName: "get_bare_single",
			wantEntries: []GetEntry{
				{VarName: "COMMAND"},
			},
		},
		{
			name:     "given bare variable mixed with modifier variable, when parsing, then returns both entries in order (why: allows mixing shorthand and full syntax)",
			taskName: "get_bare_mixed",
			wantEntries: []GetEntry{
				{VarName: "COMMAND"},
				{VarName: "CHECK", DefaultTmpl: "pubspec.yaml"},
			},
		},
		{
			name:     "given options as bare .VAR reference, when parsing, then wraps in template syntax (why: shorthand avoids {{}} noise for dynamic option lists)",
			taskName: "get_select_from_var",
			wantEntries: []GetEntry{
				{VarName: "RELEASES", Info: "Select release", FromTmpl: "{{.RELEASE_LIST}}"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tsk, ok := cfg.Tasks[tc.taskName]
			if !ok {
				t.Fatalf("task %q not found", tc.taskName)
			}
			steps := tsk.Steps

			// Act
			if len(steps) != 1 {
				t.Fatalf("want 1 step, got %d", len(steps))
			}
			s := steps[0]

			// Assert
			if s.Kind != KindGet {
				t.Fatalf("kind: got %v, want KindGet", s.Kind)
			}
			if len(s.GetEntries) != len(tc.wantEntries) {
				t.Fatalf("GetEntries len: got %d, want %d", len(s.GetEntries), len(tc.wantEntries))
			}
			for i, w := range tc.wantEntries {
				got := s.GetEntries[i]
				if got.VarName != w.VarName {
					t.Errorf("entry[%d].VarName: got %q, want %q", i, got.VarName, w.VarName)
				}
				if got.Info != w.Info {
					t.Errorf("entry[%d].Info: got %q, want %q", i, got.Info, w.Info)
				}
				if got.Check != w.Check {
					t.Errorf("entry[%d].Check: got %q, want %q", i, got.Check, w.Check)
				}
				if got.DefaultTmpl != w.DefaultTmpl {
					t.Errorf("entry[%d].DefaultTmpl: got %q, want %q", i, got.DefaultTmpl, w.DefaultTmpl)
				}
				if got.Multi != w.Multi {
					t.Errorf("entry[%d].Multi: got %v, want %v", i, got.Multi, w.Multi)
				}
				if len(got.FromList) != len(w.FromList) {
					t.Fatalf("entry[%d].FromList len: got %d, want %d", i, len(got.FromList), len(w.FromList))
				}
				for j, fw := range w.FromList {
					if got.FromList[j] != fw {
						t.Errorf("entry[%d].FromList[%d]: got %q, want %q", i, j, got.FromList[j], fw)
					}
				}
				if got.FromTmpl != w.FromTmpl {
					t.Errorf("entry[%d].FromTmpl: got %q, want %q", i, got.FromTmpl, w.FromTmpl)
				}
			}
		})
	}
}

func TestParseConfig_CallStep(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/call_step.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	tsk, ok := cfg.Tasks["parent"]
	if !ok {
		t.Fatal("task 'parent' not found")
	}
	steps := tsk.Steps
	if len(steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(steps))
	}
	s := steps[0]
	if s.Kind != KindCall {
		t.Fatalf("kind: got %v, want KindCall", s.Kind)
	}
	if s.CallTarget != "child" {
		t.Errorf("CallTarget: got %q, want %q", s.CallTarget, "child")
	}
	if len(s.CallVars) != 1 || s.CallVars[0].Key != "INPUT" || s.CallVars[0].ValTmpl != "hello" {
		t.Errorf("CallVars: got %v, want [{INPUT hello}]", s.CallVars)
	}
	wantInto := []IntoEntry{
		{ParentKey: "RESULT", ValueTmpl: "OUTPUT"},
		{ParentKey: "DERIVED", ValueTmpl: "{{.RESULT}}_done"},
	}
	if len(s.IntoEntries) != len(wantInto) {
		t.Fatalf("IntoEntries len: got %d, want %d", len(s.IntoEntries), len(wantInto))
	}
	for i, w := range wantInto {
		got := s.IntoEntries[i]
		if got.ParentKey != w.ParentKey || got.ValueTmpl != w.ValueTmpl {
			t.Errorf("into[%d]: got {%q, %q}, want {%q, %q}",
				i, got.ParentKey, got.ValueTmpl, w.ParentKey, w.ValueTmpl)
		}
	}
}

func TestParseConfig_ForStep(t *testing.T) {
	tests := []struct {
		name     string
		taskName string
		wantList []string
		wantTmpl string
	}{
		{
			name:     "given literal list, when parsing for step, then stores items in ForList",
			taskName: "loop_literal",
			wantList: []string{"alpha", "beta", "gamma"},
		},
		{
			name:     "given template target, when parsing for step, then stores template in ForTarget",
			taskName: "loop_dynamic",
			wantTmpl: "{{.FILES}}",
		},
		{
			name:     "given bare .VAR loop target, when parsing for step, then wraps in template syntax (why: shorthand avoids {{}} noise)",
			taskName: "loop_dynamic_bare",
			wantTmpl: "{{.FILES}}",
		},
	}
	cfg, err := ParseConfig("testdata/for_step.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tsk, ok := cfg.Tasks[tc.taskName]
			if !ok {
				t.Fatalf("task %q not found", tc.taskName)
			}
			steps := tsk.Steps

			// Act — find the for step (may be after a set step)
			var fs *Step
			for i := range steps {
				if steps[i].Kind == KindFor {
					fs = &steps[i]
					break
				}
			}

			// Assert
			if fs == nil {
				t.Fatal("no for step found")
			}
			if fs.ForTarget != tc.wantTmpl {
				t.Errorf("ForTarget: got %q, want %q", fs.ForTarget, tc.wantTmpl)
			}
			if len(fs.ForList) != len(tc.wantList) {
				t.Fatalf("ForList len: got %d, want %d", len(fs.ForList), len(tc.wantList))
			}
			for i, w := range tc.wantList {
				if fs.ForList[i] != w {
					t.Errorf("ForList[%d]: got %q, want %q", i, fs.ForList[i], w)
				}
			}
			if len(fs.ForSteps) == 0 {
				t.Error("ForSteps should not be empty")
			}
		})
	}
}

func TestParseConfig_ForStep_Matrix(t *testing.T) {
	cfg, err := ParseConfig("testdata/for_step.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	tests := []struct {
		name       string
		taskName   string
		wantMatrix []ForMatrixEntry
	}{
		{
			name:     "given single-var map form, when parsing, then stores one ForMatrix entry with static list",
			taskName: "loop_matrix_single",
			wantMatrix: []ForMatrixEntry{
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos", "windows"}},
			},
		},
		{
			name:     "given two-var map form, when parsing, then stores two ForMatrix entries in declaration order (why: key order determines cartesian product nesting)",
			taskName: "loop_matrix_multi",
			wantMatrix: []ForMatrixEntry{
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos"}},
				{VarName: "DB", List: []string{"postgres", "sqlite"}},
			},
		},
		{
			name:     "given map form with template value, when parsing, then stores ListTmpl in entry (why: dynamic lists resolved at runtime)",
			taskName: "loop_matrix_template",
			wantMatrix: []ForMatrixEntry{
				{VarName: "NODE", ListTmpl: "{{.SERVERS}}"},
			},
		},
		{
			name:     "given map form with bare .VAR value, when parsing, then wraps in template syntax (why: shorthand avoids {{}} noise for dynamic lists)",
			taskName: "loop_matrix_template_bare",
			wantMatrix: []ForMatrixEntry{
				{VarName: "NODE", ListTmpl: "{{.SERVERS}}"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tsk, ok := cfg.Tasks[tc.taskName]
			if !ok {
				t.Fatalf("task %q not found", tc.taskName)
			}

			// Act — find the for step
			var fs *Step
			for i := range tsk.Steps {
				if tsk.Steps[i].Kind == KindFor {
					fs = &tsk.Steps[i]
					break
				}
			}

			// Assert
			if fs == nil {
				t.Fatal("no for step found")
			}
			if len(fs.ForList) != 0 {
				t.Errorf("ForList should be empty for map form, got %v", fs.ForList)
			}
			if fs.ForTarget != "" {
				t.Errorf("ForTarget should be empty for map form, got %q", fs.ForTarget)
			}
			if len(fs.ForMatrix) != len(tc.wantMatrix) {
				t.Fatalf("ForMatrix len: got %d, want %d", len(fs.ForMatrix), len(tc.wantMatrix))
			}
			for i, w := range tc.wantMatrix {
				got := fs.ForMatrix[i]
				if got.VarName != w.VarName {
					t.Errorf("ForMatrix[%d].VarName: got %q, want %q", i, got.VarName, w.VarName)
				}
				if got.ListTmpl != w.ListTmpl {
					t.Errorf("ForMatrix[%d].ListTmpl: got %q, want %q", i, got.ListTmpl, w.ListTmpl)
				}
				if len(got.List) != len(w.List) {
					t.Fatalf("ForMatrix[%d].List len: got %d, want %d", i, len(got.List), len(w.List))
				}
				for j, item := range w.List {
					if got.List[j] != item {
						t.Errorf("ForMatrix[%d].List[%d]: got %q, want %q", i, j, got.List[j], item)
					}
				}
			}
			if len(fs.ForSteps) == 0 {
				t.Error("ForSteps should not be empty")
			}
		})
	}
}

func TestParseConfig_IfCondition(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/if_cond.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	tsk, ok := cfg.Tasks["conditional"]
	if !ok {
		t.Fatal("task 'conditional' not found")
	}
	steps := tsk.Steps
	if len(steps) != 4 {
		t.Fatalf("want 4 steps, got %d", len(steps))
	}
	if steps[0].IfExpr != "" {
		t.Errorf("step 0 should have no if condition, got %q", steps[0].IfExpr)
	}
	if steps[1].IfExpr != `{{.ENABLED}} == "true"` {
		t.Errorf("step 1 IfExpr: got %q", steps[1].IfExpr)
	}
	if steps[2].IfExpr != `{{.MODE}} == "fast"` {
		t.Errorf("step 2 IfExpr: got %q", steps[2].IfExpr)
	}
	if steps[3].IfExpr != `{{.RUN_LOOP}} == "yes"` {
		t.Errorf("step 3 IfExpr: got %q", steps[3].IfExpr)
	}
}

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

func TestParseConfig_GlobalVars(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/global_vars.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	wantEntries := []SetEntry{
		{Key: "TIMEOUT", ValTmpl: `{{.TIMEOUT | default "30"}}`},
		{Key: "HOST", ValTmpl: "localhost"},
	}
	if len(cfg.Vars) != len(wantEntries) {
		t.Fatalf("Vars len: got %d, want %d", len(cfg.Vars), len(wantEntries))
	}
	for i, w := range wantEntries {
		if cfg.Vars[i].Key != w.Key {
			t.Errorf("Vars[%d].Key: got %q, want %q", i, cfg.Vars[i].Key, w.Key)
		}
		if cfg.Vars[i].ValTmpl != w.ValTmpl {
			t.Errorf("Vars[%d].ValTmpl: got %q, want %q", i, cfg.Vars[i].ValTmpl, w.ValTmpl)
		}
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tsk, ok := cfg.Tasks[tc.taskName]
			if !ok {
				t.Fatalf("task %q not found", tc.taskName)
			}

			// Act (none — parsing already done)

			// Assert
			if tsk.Info != tc.wantInfo {
				t.Errorf("info: got %q, want %q", tsk.Info, tc.wantInfo)
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
	for i, w := range want {
		if cfg.TaskNames[i] != w {
			t.Errorf("TaskNames[%d]: got %q, want %q", i, cfg.TaskNames[i], w)
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
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			m := cfg.Modules[i]

			// Act (none)

			// Assert
			if m.Prefix != tc.wantPrefix {
				t.Errorf("Prefix: got %q, want %q", m.Prefix, tc.wantPrefix)
			}
			if m.FileTmpl != tc.wantFile {
				t.Errorf("FileTmpl: got %q, want %q", m.FileTmpl, tc.wantFile)
			}
			if len(m.ShowTmpls) != len(tc.wantShow) {
				t.Fatalf("ShowTmpls len: got %d, want %d", len(m.ShowTmpls), len(tc.wantShow))
			}
			for j, w := range tc.wantShow {
				if m.ShowTmpls[j] != w {
					t.Errorf("ShowTmpls[%d]: got %q, want %q", j, m.ShowTmpls[j], w)
				}
			}
			if len(m.HideTmpls) != len(tc.wantHide) {
				t.Fatalf("HideTmpls len: got %d, want %d", len(m.HideTmpls), len(tc.wantHide))
			}
			for j, w := range tc.wantHide {
				if m.HideTmpls[j] != w {
					t.Errorf("HideTmpls[%d]: got %q, want %q", j, m.HideTmpls[j], w)
				}
			}
			if m.FlattenTmpl != tc.wantFlatten {
				t.Errorf("FlattenTmpl: got %q, want %q", m.FlattenTmpl, tc.wantFlatten)
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
		name         string
		taskName     string
		wantTaskDir  string
		stepIdx      int
		wantStepDir  string
		wantCallTgt  string
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tsk, ok := cfg.Tasks[tc.taskName]
			if !ok {
				t.Fatalf("task %q not found", tc.taskName)
			}

			// Act (none — parsing already done)

			// Assert
			if tsk.Dir != tc.wantTaskDir {
				t.Errorf("task Dir: got %q, want %q", tsk.Dir, tc.wantTaskDir)
			}
			if tc.stepIdx >= len(tsk.Steps) {
				t.Fatalf("step[%d] missing", tc.stepIdx)
			}
			s := tsk.Steps[tc.stepIdx]
			if s.DirTmpl != tc.wantStepDir {
				t.Errorf("step DirTmpl: got %q, want %q", s.DirTmpl, tc.wantStepDir)
			}
			if tc.wantCallTgt != "" && s.CallTarget != tc.wantCallTgt {
				t.Errorf("step CallTarget: got %q, want %q", s.CallTarget, tc.wantCallTgt)
			}
		})
	}
}

func TestParseConfig_SecretGetStep(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/secret_get.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	tsk, ok := cfg.Tasks["login"]
	if !ok {
		t.Fatal("task 'login' not found")
	}
	if len(tsk.Steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(tsk.Steps))
	}
	entries := tsk.Steps[0].GetEntries

	// Act / Assert
	tests := []struct {
		name        string
		wantVar     string
		wantSecret  bool
		wantDefault string
	}{
		{
			name:       "given secret:true, when parsed, then Secret flag set (why: input must be obscured)",
			wantVar:    "PASSWORD",
			wantSecret: true,
		},
		{
			name:       "given no secret modifier, when parsed, then Secret flag false (why: default not secret)",
			wantVar:    "USERNAME",
			wantSecret: false,
		},
		{
			name:        "given secret:true with default, when parsed, then both Secret and DefaultTmpl set",
			wantVar:     "TOKEN",
			wantSecret:  true,
			wantDefault: "default-token",
		},
	}

	if len(entries) != len(tests) {
		t.Fatalf("GetEntries len: got %d, want %d", len(entries), len(tests))
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := entries[i]
			if got.VarName != tc.wantVar {
				t.Errorf("VarName: got %q, want %q", got.VarName, tc.wantVar)
			}
			if got.Secret != tc.wantSecret {
				t.Errorf("Secret: got %v, want %v", got.Secret, tc.wantSecret)
			}
			if got.DefaultTmpl != tc.wantDefault {
				t.Errorf("DefaultTmpl: got %q, want %q", got.DefaultTmpl, tc.wantDefault)
			}
		})
	}
}

func TestParseConfig_SecretSetStep(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/secret_set.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	tsk, ok := cfg.Tasks["configure"]
	if !ok {
		t.Fatal("task 'configure' not found")
	}
	if len(tsk.Steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(tsk.Steps))
	}
	entries := tsk.Steps[0].SetEntries

	// Act / Assert
	tests := []struct {
		name       string
		wantKey    string
		wantVal    string
		wantSecret bool
	}{
		{
			name:       "given expanded form secret:true, when parsed, then Secret flag set (why: value must be obscured)",
			wantKey:    "API_KEY",
			wantVal:    "abc123",
			wantSecret: true,
		},
		{
			name:       "given plain scalar value, when parsed, then Secret flag false (why: not a secret)",
			wantKey:    "HOST",
			wantVal:    "localhost",
			wantSecret: false,
		},
		{
			name:       "given expanded form with template value and secret:true, when parsed, then both stored",
			wantKey:    "DB_PASS",
			wantVal:    `{{.DB_PASS | default "changeme"}}`,
			wantSecret: true,
		},
	}

	if len(entries) != len(tests) {
		t.Fatalf("SetEntries len: got %d, want %d", len(entries), len(tests))
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := entries[i]
			if got.Key != tc.wantKey {
				t.Errorf("Key: got %q, want %q", got.Key, tc.wantKey)
			}
			if got.ValTmpl != tc.wantVal {
				t.Errorf("ValTmpl: got %q, want %q", got.ValTmpl, tc.wantVal)
			}
			if got.Secret != tc.wantSecret {
				t.Errorf("Secret: got %v, want %v", got.Secret, tc.wantSecret)
			}
		})
	}
}

func TestParseConfig_OptionalGetStep(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/optional_get.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	tsk, ok := cfg.Tasks["get_optional"]
	if !ok {
		t.Fatal("task 'get_optional' not found")
	}
	if len(tsk.Steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(tsk.Steps))
	}
	entries := tsk.Steps[0].GetEntries

	tests := []struct {
		name         string
		wantVar      string
		wantOptional bool
	}{
		{
			name:         "given optional:true, when parsed, then Optional flag set (why: optional must be recognised as a modifier)",
			wantVar:      "NOTE",
			wantOptional: true,
		},
		{
			name:         "given optional:true with check, when parsed, then both Optional and Check set",
			wantVar:      "SIZE",
			wantOptional: true,
		},
	}

	if len(entries) != len(tests) {
		t.Fatalf("GetEntries len: got %d, want %d", len(entries), len(tests))
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			got := entries[i]

			// Act (none — parsing already done)

			// Assert
			if got.VarName != tc.wantVar {
				t.Errorf("VarName: got %q, want %q", got.VarName, tc.wantVar)
			}
			if got.Optional != tc.wantOptional {
				t.Errorf("Optional: got %v, want %v", got.Optional, tc.wantOptional)
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
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			f, err := os.CreateTemp("", "hobnob-test-*.yml")
			if err != nil {
				t.Fatalf("temp file: %v", err)
			}
			defer os.Remove(f.Name())
			if _, err := f.WriteString(tc.yaml); err != nil {
				t.Fatalf("write: %v", err)
			}
			f.Close()

			// Act
			_, err = ParseConfig(f.Name())

			// Assert
			if err == nil {
				t.Fatal("expected parse error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// assertStepsEqual compares two step slices at a structural level.
func assertStepsEqual(t *testing.T, got, want []Step) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("steps len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Kind != w.Kind {
			t.Errorf("step[%d] kind: got %v, want %v", i, g.Kind, w.Kind)
		}
		if g.Command != w.Command {
			t.Errorf("step[%d] command: got %q, want %q", i, g.Command, w.Command)
		}
		if g.IfExpr != w.IfExpr {
			t.Errorf("step[%d] IfExpr: got %q, want %q", i, g.IfExpr, w.IfExpr)
		}
	}
}

func TestCollectGetParams(t *testing.T) {
	tests := []struct {
		name      string
		steps     []Step
		cfg       *ConfigFile
		wantNames []string
	}{
		{
			name:      "given steps with no get, when collected, then returns empty (why: no params to surface)",
			steps:     []Step{{Kind: KindRun, Command: "echo hi"}},
			wantNames: nil,
		},
		{
			name: "given steps with get entries, when collected, then returns all var names (why: params are inputs to the task)",
			steps: []Step{{Kind: KindGet, GetEntries: []GetEntry{
				{VarName: "ENV"},
				{VarName: "PORT", DefaultTmpl: "8080"},
			}}},
			wantNames: []string{"ENV", "PORT"},
		},
		{
			name: "given for loop containing get, when collected, then includes nested entries (why: params inside loops are still task inputs)",
			steps: []Step{{Kind: KindFor, ForSteps: []Step{{Kind: KindGet, GetEntries: []GetEntry{
				{VarName: "ITEM_CONFIRM"},
			}}}}},
			wantNames: []string{"ITEM_CONFIRM"},
		},
		{
			name: "given mixed steps with get and for-containing-get, when collected, then returns all (why: full param surface)",
			steps: []Step{
				{Kind: KindSet, SetEntries: []SetEntry{{Key: "X", ValTmpl: "1"}}},
				{Kind: KindGet, GetEntries: []GetEntry{{VarName: "REGION"}}},
				{Kind: KindFor, ForSteps: []Step{{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ZONE"}}}}},
				{Kind: KindRun, Command: "echo done"},
			},
			wantNames: []string{"REGION", "ZONE"},
		},
		{
			name: "given call step referencing sub-task with get, when collected, then includes sub-task params (why: --list misleads when called sub-task params are hidden)",
			steps: []Step{
				{Kind: KindGet, GetEntries: []GetEntry{{VarName: "TOP"}}},
				{Kind: KindCall, CallTarget: "_helper"},
			},
			cfg: &ConfigFile{
				Tasks: map[string]Task{
					"_helper": {Steps: []Step{
						{Kind: KindGet, GetEntries: []GetEntry{{VarName: "HELPER_VAR"}}},
					}},
				},
			},
			wantNames: []string{"TOP", "HELPER_VAR"},
		},
		{
			name: "given call step with template target, when collected, then skips dynamic call (why: cannot statically resolve template dispatch)",
			steps: []Step{
				{Kind: KindCall, CallTarget: "{{.PROFILE}}_setup"},
			},
			wantNames: nil,
		},
		{
			name: "given mutually recursive call tasks, when collected, then does not infinite loop (why: cycle guard prevents stack overflow)",
			steps: []Step{
				{Kind: KindCall, CallTarget: "b"},
			},
			cfg: &ConfigFile{
				Tasks: map[string]Task{
					"b": {Steps: []Step{
						{Kind: KindGet, GetEntries: []GetEntry{{VarName: "FROM_B"}}},
						{Kind: KindCall, CallTarget: "a"},
					}},
					"a": {Steps: []Step{
						{Kind: KindCall, CallTarget: "b"},
					}},
				},
			},
			wantNames: []string{"FROM_B"},
		},
		{
			name: "given set before get for same var, when collected, then omits already-set var (why: set satisfies the var so no prompt needed)",
			steps: []Step{
				{Kind: KindSet, SetEntries: []SetEntry{{Key: "ENV", ValTmpl: "production"}}},
				{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ENV"}, {VarName: "PORT"}}},
			},
			wantNames: []string{"PORT"},
		},
		{
			name: "given call with: passes var, when sub-task has get for that var, then omits it (why: with: satisfies the sub-task var so no prompt needed)",
			steps: []Step{
				{Kind: KindCall, CallTarget: "_sub", CallVars: []SetEntry{{Key: "ENV", ValTmpl: "staging"}}},
			},
			cfg: &ConfigFile{
				Tasks: map[string]Task{
					"_sub": {Steps: []Step{
						{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ENV"}, {VarName: "REGION"}}},
					}},
				},
			},
			wantNames: []string{"REGION"},
		},
		{
			name: "given run into: captures var, when later sub-task has get for that var, then omits it (why: run output satisfies the var so no prompt needed)",
			steps: []Step{
				{Kind: KindRun, Command: "git rev-parse HEAD", IntoEntries: []IntoEntry{{ParentKey: "SHA", ValueTmpl: "stdout | trim"}}},
				{Kind: KindCall, CallTarget: "_sub"},
			},
			cfg: &ConfigFile{
				Tasks: map[string]Task{
					"_sub": {Steps: []Step{
						{Kind: KindGet, GetEntries: []GetEntry{{VarName: "SHA"}, {VarName: "REGION"}}},
					}},
				},
			},
			wantNames: []string{"REGION"},
		},
		{
			name: "given get in parent and same var in sub-task get, when collected, then deduplicates (why: already surfaced once so no double prompt)",
			steps: []Step{
				{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ENV"}}},
				{Kind: KindCall, CallTarget: "_sub"},
			},
			cfg: &ConfigFile{
				Tasks: map[string]Task{
					"_sub": {Steps: []Step{
						{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ENV"}, {VarName: "REGION"}}},
					}},
				},
			},
			wantNames: []string{"ENV", "REGION"},
		},
		{
			name: "given for loop default ITEM iterator, when loop body has get for ITEM, then omits it (why: iterator is injected by loop engine not user input)",
			steps: []Step{
				{Kind: KindFor, ForSteps: []Step{
					{Kind: KindGet, GetEntries: []GetEntry{{VarName: "ITEM"}, {VarName: "CONFIRM"}}},
				}},
			},
			wantNames: []string{"CONFIRM"},
		},
		{
			name: "given for matrix loop, when loop body has get for matrix vars, then omits them (why: matrix vars are injected by loop engine not user input)",
			steps: []Step{
				{Kind: KindFor, ForMatrix: []ForMatrixEntry{{VarName: "OS"}, {VarName: "ARCH"}}, ForSteps: []Step{
					{Kind: KindGet, GetEntries: []GetEntry{{VarName: "OS"}, {VarName: "ARCH"}, {VarName: "CONFIRM"}}},
				}},
			},
			wantNames: []string{"CONFIRM"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc.steps and tc.cfg are the arrangement)

			// Act
			got := CollectGetParams(tc.steps, tc.cfg)

			// Assert
			if len(got) != len(tc.wantNames) {
				t.Fatalf("got %d entries, want %d: %v", len(got), len(tc.wantNames), got)
			}
			for i, name := range tc.wantNames {
				if got[i].VarName != name {
					t.Errorf("entry[%d].VarName = %q, want %q", i, got[i].VarName, name)
				}
			}
		})
	}
}
