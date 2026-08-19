package runner

import (
	"context"
	"testing"

	"hobnob/internal/cli"
	"hobnob/internal/config"
)

func TestExecSet_JSONNode(t *testing.T) {
	tests := []struct {
		name      string
		entry     config.SetEntry
		initVars  map[string]string
		wantValue string
	}{
		{
			name: "given a map literal with no template leaves, when executed, then marshaled to the same JSON object string as before (why: pins output for the common case across the marshal-after-eval refactor)",
			entry: config.SetEntry{
				Key: "REGION_MAP",
				ValNode: &config.JSONNode{
					Kind: config.JSONObject,
					Fields: []config.JSONField{
						{Key: "us", Node: config.JSONNode{Kind: config.JSONString, Tmpl: "us-east-1"}},
						{Key: "eu", Node: config.JSONNode{Kind: config.JSONString, Tmpl: "eu-west-1"}},
					},
				},
			},
			wantValue: `{"eu":"eu-west-1","us":"us-east-1"}`,
		},
		{
			name: "given a captured value containing a double quote used as a map-literal template leaf, when executed, then the quote is escaped by json.Marshal rather than corrupting the JSON (why: regression for the injection bug — marshaling now happens once, after leaf evaluation, not before it)",
			entry: config.SetEntry{
				Key: "OBJ",
				ValNode: &config.JSONNode{
					Kind: config.JSONObject,
					Fields: []config.JSONField{
						{Key: "name", Node: config.JSONNode{Kind: config.JSONString, Tmpl: "{{.NAME}}"}},
					},
				},
			},
			initVars:  map[string]string{"NAME": `he said "hi"`},
			wantValue: `{"name":"he said \"hi\""}`,
		},
		{
			name: "given a nested mapping with typed literals and a template leaf, when executed, then non-string types round-trip as native JSON and the template leaf still evaluates (why: JSONLiteral leaves bypass template eval entirely, so they can't be corrupted by it, while JSONString leaves still resolve against scope)",
			entry: config.SetEntry{
				Key: "NESTED",
				ValNode: &config.JSONNode{
					Kind: config.JSONObject,
					Fields: []config.JSONField{
						{Key: "count", Node: config.JSONNode{Kind: config.JSONLiteral, Literal: int(3)}},
						{Key: "active", Node: config.JSONNode{Kind: config.JSONLiteral, Literal: true}},
						{Key: "tags", Node: config.JSONNode{Kind: config.JSONArray, Elements: []config.JSONNode{
							{Kind: config.JSONString, Tmpl: "a"},
							{Kind: config.JSONString, Tmpl: "b"},
						}}},
						{Key: "env", Node: config.JSONNode{Kind: config.JSONString, Tmpl: "{{.ENV}}"}},
					},
				},
			},
			initVars:  map[string]string{"ENV": "prod"},
			wantValue: `{"active":true,"count":3,"env":"prod","tags":["a","b"]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg := &config.ConfigFile{
				Tasks: map[string]config.Task{
					"t": {Steps: []config.Step{{Kind: config.KindSet, SetEntries: []config.SetEntry{test.entry}}}},
				},
			}
			vars := map[string]string{}
			for k, v := range test.initVars {
				vars[k] = v
			}

			// Act
			err := ExecuteTask(context.Background(), "t", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, true, "")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := vars[test.entry.Key]; got != test.wantValue {
				t.Errorf("%s: got %q, want %q", test.entry.Key, got, test.wantValue)
			}
		})
	}
}

func TestExecSet_JSONNode_EndToEnd(t *testing.T) {
	// given a real hobnob.yml with map/list literals — a bare .VAR shorthand
	// leaf, a {{}} template leaf, typed scalar leaves, and a YAML anchor/alias
	// pair inside a literal — when parsed via config.ParseConfig and run
	// through ExecuteTask, then the full parse->eval pipeline (not just
	// hand-built config.JSONNode trees) produces the expected JSON (why:
	// pins the two behavior changes from the JSONNode refactor — literal
	// leaves are now templated, and non-string YAML scalars keep their
	// native JSON type — plus alias resolution inside a literal)
	cfg, err := config.ParseConfig("testdata/literal_eval.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	vars := map[string]string{}

	// Act
	err = ExecuteTask(context.Background(), "literal-eval", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, true, "")

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := vars["PORTS"], `[8080,9090]`; got != want {
		t.Errorf("PORTS: got %q, want %q (typed list literal must keep native JSON types)", got, want)
	}
	if got, want := vars["CONFIG"], `{"enabled":true,"endpoint":"prod.example.com/v1","host":"prod.example.com","replicas":3}`; got != want {
		t.Errorf("CONFIG: got %q, want %q (map literal leaves must be templated)", got, want)
	}
	if got, want := vars["ALIASED"], `{"a":"hello","b":"hello"}`; got != want {
		t.Errorf("ALIASED: got %q, want %q (YAML alias inside a literal must resolve)", got, want)
	}
}

func TestExecSet_SecretFlag(t *testing.T) {
	tests := []struct {
		name          string
		entries       []config.SetEntry
		wantSecrets   map[string]bool
		wantNoSecrets []string
	}{
		{
			name: "given secret:true set entry, when executed, then var marked in secrets (why: must be masked in run display)",
			entries: []config.SetEntry{
				{Key: "API_KEY", ValTmpl: "abc123", Secret: true},
			},
			wantSecrets: map[string]bool{"API_KEY": true},
		},
		{
			name: "given non-secret set entry, when executed, then var not in secrets (why: only explicitly marked vars masked)",
			entries: []config.SetEntry{
				{Key: "HOST", ValTmpl: "localhost"},
			},
			wantNoSecrets: []string{"HOST"},
		},
		{
			name: "given mixed set entries, when executed, then only secret ones marked (why: selective masking)",
			entries: []config.SetEntry{
				{Key: "TOKEN", ValTmpl: "s3cr3t", Secret: true},
				{Key: "ENV", ValTmpl: "prod"},
			},
			wantSecrets:   map[string]bool{"TOKEN": true},
			wantNoSecrets: []string{"ENV"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg := &config.ConfigFile{
				Tasks: map[string]config.Task{
					"t": {Steps: []config.Step{{Kind: config.KindSet, SetEntries: test.entries}}},
				},
			}
			secrets := make(map[string]bool)

			// Act
			err := ExecuteTask(context.Background(), "t", &cli.Scope{Vars: map[string]string{}, Secrets: secrets}, cfg, true, "")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for key := range test.wantSecrets {
				if !secrets[key] {
					t.Errorf("secrets[%q]: want true, got false", key)
				}
			}
			for _, key := range test.wantNoSecrets {
				if secrets[key] {
					t.Errorf("secrets[%q]: want false, got true", key)
				}
			}
		})
	}
}
