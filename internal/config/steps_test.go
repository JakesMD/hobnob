package config

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			cfg, err := ParseConfig(test.fixture)

			// Assert
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			task, ok := cfg.Tasks[test.taskName]
			if !ok {
				t.Fatalf("task %q not found", test.taskName)
			}
			assertStepsEqual(t, task.Steps, test.want)
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
	task, ok := cfg.Tasks["parent"]
	if !ok {
		t.Fatal("task 'parent' not found")
	}
	steps := task.Steps
	if len(steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(steps))
	}
	step := steps[0]
	if step.Kind != KindCall {
		t.Fatalf("kind: got %v, want KindCall", step.Kind)
	}
	if step.CallTarget != "child" {
		t.Errorf("CallTarget: got %q, want %q", step.CallTarget, "child")
	}
	if len(step.CallVars) != 1 || step.CallVars[0].Key != "INPUT" || step.CallVars[0].ValTmpl != "hello" {
		t.Errorf("CallVars: got %v, want [{INPUT hello}]", step.CallVars)
	}
	wantInto := []IntoEntry{
		{ParentKey: "RESULT", ValueTmpl: "OUTPUT"},
		{ParentKey: "DERIVED", ValueTmpl: "{{.RESULT}}_done"},
	}
	if len(step.IntoEntries) != len(wantInto) {
		t.Fatalf("IntoEntries len: got %d, want %d", len(step.IntoEntries), len(wantInto))
	}
	for i, wantIntoEntry := range wantInto {
		got := step.IntoEntries[i]
		if got.ParentKey != wantIntoEntry.ParentKey || got.ValueTmpl != wantIntoEntry.ValueTmpl {
			t.Errorf("into[%d]: got {%q, %q}, want {%q, %q}",
				i, got.ParentKey, got.ValueTmpl, wantIntoEntry.ParentKey, wantIntoEntry.ValueTmpl)
		}
	}
}

func TestParseConfig_RunStep_IntoNestedObject(t *testing.T) {
	// given a run: step's into: entry has a nested mapping value, one field
	// two levels deep, when parsed, then it's deferred as a JSONNode tree
	// instead of being read as an empty scalar (why: into:'s leaves keep
	// their normal stdout|filter grammar, just nested under keys)
	cfg, err := ParseConfig("testdata/into_nested.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	task, ok := cfg.Tasks["parent"]
	if !ok {
		t.Fatal("task 'parent' not found")
	}
	if len(task.Steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(task.Steps))
	}
	into := task.Steps[0].IntoEntries
	if len(into) != 2 {
		t.Fatalf("IntoEntries len: got %d, want 2", len(into))
	}

	custom := into[0]
	if custom.ParentKey != "CUSTOM" {
		t.Errorf("into[0].ParentKey: got %q, want CUSTOM", custom.ParentKey)
	}
	if custom.ValNode == nil {
		t.Fatal("into[0].ValNode is nil, want a JSON object node")
	}
	if custom.ValNode.Kind != JSONObject {
		t.Fatalf("into[0].ValNode.Kind: got %v, want JSONObject", custom.ValNode.Kind)
	}
	if len(custom.ValNode.Fields) != 2 {
		t.Fatalf("into[0].ValNode.Fields len: got %d, want 2", len(custom.ValNode.Fields))
	}
	idField := custom.ValNode.Fields[0]
	if idField.Key != "id" || idField.Node.Kind != JSONString || idField.Node.Tmpl != `stdout | pluck "id"` {
		t.Errorf("into[0].ValNode.Fields[0]: got %+v, want {id {JSONString stdout | pluck \"id\"}}", idField)
	}
	profileField := custom.ValNode.Fields[1]
	if profileField.Key != "profile" || profileField.Node.Kind != JSONObject {
		t.Fatalf("into[0].ValNode.Fields[1]: got %+v, want nested object under 'profile'", profileField)
	}
	nameField := profileField.Node.Fields[0]
	if nameField.Key != "name" || nameField.Node.Kind != JSONString || nameField.Node.Tmpl != `stdout | pluck "profile.name"` {
		t.Errorf("into[0].ValNode.Fields[1].Fields[0]: got %+v, want {name {JSONString stdout | pluck \"profile.name\"}}", nameField)
	}

	plain := into[1]
	if plain.ParentKey != "PLAIN" || plain.ValNode != nil || plain.ValueTmpl != "stdout" {
		t.Errorf("into[1]: got %+v, want scalar {PLAIN stdout} with nil ValNode", plain)
	}
}

func TestParseSetNode_MapLiteral_NestedKeyErrorIncludesPath(t *testing.T) {
	// given a map literal with an invalid key two levels deep, when parsed,
	// then the error names the full dotted path to the bad key, not just the
	// top-level set entry (why: parseJSONLiteralNode recurses per-field, so
	// without threading a path through, a deeply nested failure only ever
	// reported the outermost key — useless for finding it in a large literal)
	node := &yaml.Node{}
	src := `
- set:
    - OUTER:
        inner:
          "{{bad}}": x
`
	if err := yaml.Unmarshal([]byte(src), node); err != nil {
		t.Fatalf("unexpected yaml.Unmarshal error: %v", err)
	}
	setNode := node.Content[0].Content[0].Content[1]

	_, err := parseSetNode(setNode)

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "OUTER.inner.{{bad}}") {
		t.Errorf("error %q does not contain expected nested path %q", err.Error(), "OUTER.inner.{{bad}}")
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			task, ok := cfg.Tasks[test.taskName]
			if !ok {
				t.Fatalf("task %q not found", test.taskName)
			}
			steps := task.Steps

			// Act — find the for step (may be after a set step)
			var forStep *Step
			for i := range steps {
				if steps[i].Kind == KindFor {
					forStep = &steps[i]
					break
				}
			}

			// Assert
			if forStep == nil {
				t.Fatal("no for step found")
			}
			if forStep.ForTarget != test.wantTmpl {
				t.Errorf("ForTarget: got %q, want %q", forStep.ForTarget, test.wantTmpl)
			}
			if len(forStep.ForList) != len(test.wantList) {
				t.Fatalf("ForList len: got %d, want %d", len(forStep.ForList), len(test.wantList))
			}
			for i, wantItem := range test.wantList {
				if forStep.ForList[i] != wantItem {
					t.Errorf("ForList[%d]: got %q, want %q", i, forStep.ForList[i], wantItem)
				}
			}
			if len(forStep.ForSteps) == 0 {
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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			task, ok := cfg.Tasks[test.taskName]
			if !ok {
				t.Fatalf("task %q not found", test.taskName)
			}

			// Act — find the for step
			var forStep *Step
			for i := range task.Steps {
				if task.Steps[i].Kind == KindFor {
					forStep = &task.Steps[i]
					break
				}
			}

			// Assert
			if forStep == nil {
				t.Fatal("no for step found")
			}
			if len(forStep.ForList) != 0 {
				t.Errorf("ForList should be empty for map form, got %v", forStep.ForList)
			}
			if forStep.ForTarget != "" {
				t.Errorf("ForTarget should be empty for map form, got %q", forStep.ForTarget)
			}
			if len(forStep.ForMatrix) != len(test.wantMatrix) {
				t.Fatalf("ForMatrix len: got %d, want %d", len(forStep.ForMatrix), len(test.wantMatrix))
			}
			for i, wantEntry := range test.wantMatrix {
				got := forStep.ForMatrix[i]
				if got.VarName != wantEntry.VarName {
					t.Errorf("ForMatrix[%d].VarName: got %q, want %q", i, got.VarName, wantEntry.VarName)
				}
				if got.ListTmpl != wantEntry.ListTmpl {
					t.Errorf("ForMatrix[%d].ListTmpl: got %q, want %q", i, got.ListTmpl, wantEntry.ListTmpl)
				}
				if len(got.List) != len(wantEntry.List) {
					t.Fatalf("ForMatrix[%d].List len: got %d, want %d", i, len(got.List), len(wantEntry.List))
				}
				for j, item := range wantEntry.List {
					if got.List[j] != item {
						t.Errorf("ForMatrix[%d].List[%d]: got %q, want %q", i, j, got.List[j], item)
					}
				}
			}
			if len(forStep.ForSteps) == 0 {
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
	task, ok := cfg.Tasks["conditional"]
	if !ok {
		t.Fatal("task 'conditional' not found")
	}
	steps := task.Steps
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

func TestParseConfig_SecretInWith_Rejected(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "given secret: true on a with: entry, when parsed, then returns error (why: secrecy belongs where the value is defined — Copy() carries it into the child and masking matches on value)",
			yaml: `tasks:
  parent:
    steps:
      - call: child
        with:
          - TOKEN:
              value: .VAULT_TOKEN
              secret: true`,
			wantErr: "secret: is not supported here",
		},
		{
			name: "given secret: false on a with: entry, when parsed, then parses fine (why: only an affirmative flag is a mistake worth blocking)",
			yaml: `tasks:
  parent:
    steps:
      - call: child
        with:
          - TOKEN:
              value: .VAULT_TOKEN
              secret: false`,
			wantErr: "",
		},
		{
			name: "given secret: true on a set: entry, when parsed, then parses fine (why: the rejection is scoped to with:, set: is where secrecy is declared)",
			yaml: `tasks:
  parent:
    steps:
      - set:
          - TOKEN:
              value: .VAULT_TOKEN
              secret: true`,
			wantErr: "",
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
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected parse error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected parse error, got nil")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), test.wantErr)
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
		gotStep, wantStep := got[i], want[i]
		if gotStep.Kind != wantStep.Kind {
			t.Errorf("step[%d] kind: got %v, want %v", i, gotStep.Kind, wantStep.Kind)
		}
		if gotStep.Command != wantStep.Command {
			t.Errorf("step[%d] command: got %q, want %q", i, gotStep.Command, wantStep.Command)
		}
		if gotStep.IfExpr != wantStep.IfExpr {
			t.Errorf("step[%d] IfExpr: got %q, want %q", i, gotStep.IfExpr, wantStep.IfExpr)
		}
	}
}
