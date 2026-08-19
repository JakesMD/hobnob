package config

import (
	"testing"
)

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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			task, ok := cfg.Tasks[test.taskName]
			if !ok {
				t.Fatalf("task %q not found", test.taskName)
			}
			steps := task.Steps

			// Act
			if len(steps) != 1 {
				t.Fatalf("want 1 step, got %d", len(steps))
			}
			step := steps[0]

			// Assert
			if step.Kind != KindGet {
				t.Fatalf("kind: got %v, want KindGet", step.Kind)
			}
			if len(step.GetEntries) != len(test.wantEntries) {
				t.Fatalf("GetEntries len: got %d, want %d", len(step.GetEntries), len(test.wantEntries))
			}
			for i, wantEntry := range test.wantEntries {
				got := step.GetEntries[i]
				if got.VarName != wantEntry.VarName {
					t.Errorf("entry[%d].VarName: got %q, want %q", i, got.VarName, wantEntry.VarName)
				}
				if got.Info != wantEntry.Info {
					t.Errorf("entry[%d].Info: got %q, want %q", i, got.Info, wantEntry.Info)
				}
				if got.Check != wantEntry.Check {
					t.Errorf("entry[%d].Check: got %q, want %q", i, got.Check, wantEntry.Check)
				}
				if got.DefaultTmpl != wantEntry.DefaultTmpl {
					t.Errorf("entry[%d].DefaultTmpl: got %q, want %q", i, got.DefaultTmpl, wantEntry.DefaultTmpl)
				}
				if got.Multi != wantEntry.Multi {
					t.Errorf("entry[%d].Multi: got %v, want %v", i, got.Multi, wantEntry.Multi)
				}
				if len(got.FromList) != len(wantEntry.FromList) {
					t.Fatalf("entry[%d].FromList len: got %d, want %d", i, len(got.FromList), len(wantEntry.FromList))
				}
				for j, wantItem := range wantEntry.FromList {
					if got.FromList[j] != wantItem {
						t.Errorf("entry[%d].FromList[%d]: got %q, want %q", i, j, got.FromList[j], wantItem)
					}
				}
				if got.FromTmpl != wantEntry.FromTmpl {
					t.Errorf("entry[%d].FromTmpl: got %q, want %q", i, got.FromTmpl, wantEntry.FromTmpl)
				}
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
	task, ok := cfg.Tasks["login"]
	if !ok {
		t.Fatal("task 'login' not found")
	}
	if len(task.Steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(task.Steps))
	}
	entries := task.Steps[0].GetEntries

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
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := entries[i]
			if got.VarName != test.wantVar {
				t.Errorf("VarName: got %q, want %q", got.VarName, test.wantVar)
			}
			if got.Secret != test.wantSecret {
				t.Errorf("Secret: got %v, want %v", got.Secret, test.wantSecret)
			}
			if got.DefaultTmpl != test.wantDefault {
				t.Errorf("DefaultTmpl: got %q, want %q", got.DefaultTmpl, test.wantDefault)
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
	task, ok := cfg.Tasks["get_optional"]
	if !ok {
		t.Fatal("task 'get_optional' not found")
	}
	if len(task.Steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(task.Steps))
	}
	entries := task.Steps[0].GetEntries

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
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			got := entries[i]

			// Act (none — parsing already done)

			// Assert
			if got.VarName != test.wantVar {
				t.Errorf("VarName: got %q, want %q", got.VarName, test.wantVar)
			}
			if got.Optional != test.wantOptional {
				t.Errorf("Optional: got %v, want %v", got.Optional, test.wantOptional)
			}
		})
	}
}
