package config

import (
	"encoding/json"
	"testing"
)

// renderJSONNode mirrors runner.evalJSONNode/evalJSONNodeToJSON (which
// vars_test.go can't import — runner already imports config) for fixtures
// with no template leaves, so these parse-level tests can still assert on
// the resulting JSON string rather than raw tree shape.
func renderJSONNode(t *testing.T, n JSONNode) any {
	t.Helper()
	switch n.Kind {
	case JSONObject:
		obj := make(map[string]any, len(n.Fields))
		for _, field := range n.Fields {
			obj[field.Key] = renderJSONNode(t, field.Node)
		}
		return obj
	case JSONArray:
		arr := make([]any, len(n.Elements))
		for i, elem := range n.Elements {
			arr[i] = renderJSONNode(t, elem)
		}
		return arr
	case JSONLiteral:
		return n.Literal
	default: // JSONString
		return n.Tmpl
	}
}

func renderJSONNodeString(t *testing.T, n JSONNode) string {
	t.Helper()
	jsonBytes, err := json.Marshal(renderJSONNode(t, n))
	if err != nil {
		t.Fatalf("failed to marshal JSONNode: %v", err)
	}
	return string(jsonBytes)
}

func TestParseConfig_SetStep(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/set_step.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	task, ok := cfg.Tasks["setup"]
	if !ok {
		t.Fatal("task 'setup' not found")
	}
	steps := task.Steps
	if len(steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(steps))
	}
	step := steps[0]
	if step.Kind != KindSet {
		t.Fatalf("want KindSet, got %v", step.Kind)
	}
	wantEntries := []SetEntry{
		{Key: "LIMIT", ValTmpl: "1048576"},
		{Key: "LABEL", ValTmpl: `{{ run "echo hobnob" }}`},
		{Key: "DERIVED", ValTmpl: "{{.LIMIT}}/bytes"},
		{Key: "ALIAS", ValTmpl: "{{.LIMIT}}"},
	}
	if len(step.SetEntries) != len(wantEntries) {
		t.Fatalf("want %d set entries, got %d", len(wantEntries), len(step.SetEntries))
	}
	for i, wantEntry := range wantEntries {
		t.Run(wantEntry.Key, func(t *testing.T) {
			// Arrange
			got := step.SetEntries[i]

			// Act (none — just assertion)

			// Assert
			if got.Key != wantEntry.Key {
				t.Errorf("key[%d]: got %q, want %q", i, got.Key, wantEntry.Key)
			}
			if got.ValTmpl != wantEntry.ValTmpl {
				t.Errorf("val[%d]: got %q, want %q", i, got.ValTmpl, wantEntry.ValTmpl)
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
	task, ok := cfg.Tasks["setup"]
	if !ok {
		t.Fatal("task 'setup' not found")
	}
	steps := task.Steps
	if len(steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(steps))
	}
	entries := steps[0].SetEntries
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}

	tests := []struct {
		name       string
		key        string
		wantIsNode bool
		wantVal    string
	}{
		{
			name:       "given YAML sequence value, when parsed, then deferred as a JSON array node (why: leaves must be evaluated before the array is marshaled, not before)",
			key:        "PACKAGES",
			wantIsNode: true,
			wantVal:    `["packages/data/platform_client/","packages/data/database_client/","packages/data/auth_client/"]`,
		},
		{
			name:       "given scalar value, when parsed, then stored as-is (why: non-sequence path unchanged)",
			key:        "SCALAR",
			wantIsNode: false,
			wantVal:    "just a string",
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			got := entries[i]

			// Act (none)

			// Assert
			if got.Key != test.key {
				t.Errorf("key: got %q, want %q", got.Key, test.key)
			}
			if test.wantIsNode {
				if got.ValNode == nil {
					t.Fatalf("val: ValNode is nil, want a JSON array node")
				}
				if gotVal := renderJSONNodeString(t, *got.ValNode); gotVal != test.wantVal {
					t.Errorf("val: got %q, want %q", gotVal, test.wantVal)
				}
				return
			}
			if got.ValNode != nil {
				t.Fatalf("val: ValNode is set, want nil (scalar path)")
			}
			if got.ValTmpl != test.wantVal {
				t.Errorf("val: got %q, want %q", got.ValTmpl, test.wantVal)
			}
		})
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
	for i, wantEntry := range wantEntries {
		if cfg.Vars[i].Key != wantEntry.Key {
			t.Errorf("Vars[%d].Key: got %q, want %q", i, cfg.Vars[i].Key, wantEntry.Key)
		}
		if cfg.Vars[i].ValTmpl != wantEntry.ValTmpl {
			t.Errorf("Vars[%d].ValTmpl: got %q, want %q", i, cfg.Vars[i].ValTmpl, wantEntry.ValTmpl)
		}
	}
}

func TestParseConfig_SetStep_MapValue(t *testing.T) {
	// Arrange
	cfg, err := ParseConfig("testdata/set_map_value.yml")

	// Act + Assert
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	task, ok := cfg.Tasks["setup"]
	if !ok {
		t.Fatal("task 'setup' not found")
	}
	steps := task.Steps
	if len(steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(steps))
	}
	entries := steps[0].SetEntries
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}

	tests := []struct {
		name       string
		key        string
		wantIsNode bool
		wantVal    string
	}{
		{
			name:       "given YAML mapping value, when parsed, then deferred as a JSON object node (why: leaves must be evaluated before the object is marshaled, not before — see JSONNode)",
			key:        "REGION_MAP",
			wantIsNode: true,
			wantVal:    `{"eu":"eu-west-1","us":"us-east-1"}`,
		},
		{
			name:       "given nested mapping with typed values, when parsed, then types preserved in the JSON object (why: non-string YAML scalars decode to JSONLiteral at parse time and are never routed through template eval, so numbers/bools/nested lists survive for pluck to return them faithfully)",
			key:        "NESTED_MAP",
			wantIsNode: true,
			wantVal:    `{"active":true,"count":3,"tags":["a","b"]}`,
		},
		{
			name:       "given scalar value, when parsed, then stored as-is (why: non-mapping path unchanged)",
			key:        "SCALAR",
			wantIsNode: false,
			wantVal:    "just a string",
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			got := entries[i]

			// Act (none)

			// Assert
			if got.Key != test.key {
				t.Errorf("key[%d]: got %q, want %q", i, got.Key, test.key)
			}
			if test.wantIsNode {
				if got.ValNode == nil {
					t.Fatalf("val[%d]: ValNode is nil, want a JSON object node", i)
				}
				if gotVal := renderJSONNodeString(t, *got.ValNode); gotVal != test.wantVal {
					t.Errorf("val[%d]: got %q, want %q", i, gotVal, test.wantVal)
				}
				return
			}
			if got.ValNode != nil {
				t.Fatalf("val[%d]: ValNode is set, want nil (scalar path)", i)
			}
			if got.ValTmpl != test.wantVal {
				t.Errorf("val[%d]: got %q, want %q", i, got.ValTmpl, test.wantVal)
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
	task, ok := cfg.Tasks["configure"]
	if !ok {
		t.Fatal("task 'configure' not found")
	}
	if len(task.Steps) != 1 {
		t.Fatalf("want 1 step, got %d", len(task.Steps))
	}
	entries := task.Steps[0].SetEntries

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
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := entries[i]
			if got.Key != test.wantKey {
				t.Errorf("Key: got %q, want %q", got.Key, test.wantKey)
			}
			if got.ValTmpl != test.wantVal {
				t.Errorf("ValTmpl: got %q, want %q", got.ValTmpl, test.wantVal)
			}
			if got.Secret != test.wantSecret {
				t.Errorf("Secret: got %v, want %v", got.Secret, test.wantSecret)
			}
		})
	}
}
