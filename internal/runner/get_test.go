package runner

import (
	"context"
	"strings"
	"testing"

	"hobnob/internal/cli"
	"hobnob/internal/config"
)

func makeGetCfg(entries []config.GetEntry) *config.ConfigFile {
	return &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{{Kind: config.KindGet, GetEntries: entries}}},
		},
	}
}

func TestNoInput_GetStep(t *testing.T) {
	tests := []struct {
		name     string
		entries  []config.GetEntry
		vars     map[string]string
		wantErr  string
		wantVars map[string]string
	}{
		{
			name: "given no-input and var not set and no default, when executed, then fails (why: no way to satisfy the prompt without interaction)",
			entries: []config.GetEntry{
				{VarName: "FOO", Info: "Enter foo"},
			},
			vars:    map[string]string{},
			wantErr: "--no-input: FOO requires input",
		},
		{
			name: "given no-input and var not set and default provided, when executed, then uses default (why: default removes need for interaction)",
			entries: []config.GetEntry{
				{VarName: "FOO", Info: "Enter foo", DefaultTmpl: "bar"},
			},
			vars:     map[string]string{},
			wantVars: map[string]string{"FOO": "bar"},
		},
		{
			name: "given no-input and var preset, when executed, then uses preset value (why: skip-if-set rule applies)",
			entries: []config.GetEntry{
				{VarName: "FOO", Info: "Enter foo"},
			},
			vars:     map[string]string{"FOO": "preset"},
			wantVars: map[string]string{"FOO": "preset"},
		},
		{
			name: "given no-input and var preset and value in options, when executed, then succeeds (why: value satisfies the option constraint)",
			entries: []config.GetEntry{
				{VarName: "FOO", Info: "Pick", FromList: []string{"a", "b", "c"}},
			},
			vars:     map[string]string{"FOO": "b"},
			wantVars: map[string]string{"FOO": "b"},
		},
		{
			name: "given no-input and var preset and value not in options, when executed, then fails (why: preset value violates declared options constraint)",
			entries: []config.GetEntry{
				{VarName: "FOO", Info: "Pick", FromList: []string{"a", "b", "c"}},
			},
			vars:    map[string]string{"FOO": "x"},
			wantErr: "--no-input: FOO value \"x\" not in options",
		},
		{
			name: "given no-input and default in options, when executed, then uses default and succeeds (why: default satisfies option constraint)",
			entries: []config.GetEntry{
				{VarName: "FOO", Info: "Pick", DefaultTmpl: "a", FromList: []string{"a", "b"}},
			},
			vars:     map[string]string{},
			wantVars: map[string]string{"FOO": "a"},
		},
		{
			name: "given no-input and default not in options, when executed, then fails (why: default violates declared options constraint)",
			entries: []config.GetEntry{
				{VarName: "FOO", Info: "Pick", DefaultTmpl: "z", FromList: []string{"a", "b"}},
			},
			vars:    map[string]string{},
			wantErr: "--no-input: FOO value \"z\" not in options",
		},
		{
			name: "given no-input and preset value passes check, when executed, then succeeds (why: check validates the value)",
			entries: []config.GetEntry{
				{VarName: "SIZE", Info: "Enter size", Check: "[ {{.SIZE}} -le 100 ]"},
			},
			vars:     map[string]string{"SIZE": "50"},
			wantVars: map[string]string{"SIZE": "50"},
		},
		{
			name: "given no-input and preset value fails check, when executed, then fails (why: check rejects invalid value)",
			entries: []config.GetEntry{
				{VarName: "SIZE", Info: "Enter size", Check: "[ {{.SIZE}} -le 100 ]"},
			},
			vars:    map[string]string{"SIZE": "200"},
			wantErr: "validation failed",
		},
		{
			name: "given no-input and default value fails check, when executed, then fails (why: check applies to default values too)",
			entries: []config.GetEntry{
				{VarName: "SIZE", Info: "Enter size", DefaultTmpl: "999", Check: "[ {{.SIZE}} -le 100 ]"},
			},
			vars:    map[string]string{},
			wantErr: "validation failed",
		},
		{
			name: "given no-input and preset value with malformed check template, when executed, then errors with template error not validation failed (why: template errors must be distinguished from check failures)",
			entries: []config.GetEntry{
				{VarName: "SIZE", Info: "Enter size", Check: "[ {{ .Unclosed "},
			},
			vars:    map[string]string{"SIZE": "50"},
			wantErr: "get SIZE check:",
		},
		{
			name: "given no-input and multi-select var preset with all values in options, when executed, then succeeds (why: each selected element validated against options list)",
			entries: []config.GetEntry{
				{VarName: "TAGS", Info: "Pick", Multi: true, FromList: []string{"a", "b", "c"}},
			},
			vars:     map[string]string{"TAGS": `["a","c"]`},
			wantVars: map[string]string{"TAGS": `["a","c"]`},
		},
		{
			name: "given no-input and multi-select var preset with value not in options, when executed, then fails (why: invalid selection must be rejected even for multi)",
			entries: []config.GetEntry{
				{VarName: "TAGS", Info: "Pick", Multi: true, FromList: []string{"a", "b"}},
			},
			vars:    map[string]string{"TAGS": `["x"]`},
			wantErr: `--no-input: TAGS value "x" not in options`,
		},
		{
			name: "given no-input and multi-select var preset with invalid JSON, when executed, then fails (why: stored value must be a valid JSON array)",
			entries: []config.GetEntry{
				{VarName: "TAGS", Info: "Pick", Multi: true, FromList: []string{"a", "b"}},
			},
			vars:    map[string]string{"TAGS": `not-json`},
			wantErr: "--no-input: TAGS value is not a valid JSON array",
		},
		{
			name: "given no-input and two vars, first preset second uses default, when executed, then both resolved (why: each entry evaluated independently)",
			entries: []config.GetEntry{
				{VarName: "A", Info: "A"},
				{VarName: "B", Info: "B", DefaultTmpl: "bval"},
			},
			vars:     map[string]string{"A": "aval"},
			wantVars: map[string]string{"A": "aval", "B": "bval"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg := makeGetCfg(test.entries)
			vars := copyVars(test.vars)

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

func TestNoInput_GetStep_Optional(t *testing.T) {
	tests := []struct {
		name     string
		entries  []config.GetEntry
		vars     map[string]string
		wantErr  string
		wantVars map[string]string
	}{
		{
			name: "given optional and no-input and var not set, when executed, then succeeds with empty string (why: optional means empty is valid without interaction)",
			entries: []config.GetEntry{
				{VarName: "NOTE", Info: "Enter note", Optional: true},
			},
			vars:     map[string]string{},
			wantVars: map[string]string{"NOTE": ""},
		},
		{
			name: "given optional and no-input and var not set with check, when executed, then empty bypasses check (why: check skipped when optional and value is empty)",
			entries: []config.GetEntry{
				{VarName: "SIZE", Info: "Enter size", Check: "[ {{.SIZE}} -le 100 ]", Optional: true},
			},
			vars:     map[string]string{},
			wantVars: map[string]string{"SIZE": ""},
		},
		{
			name: "given optional and no-input and var preset as empty string, when executed, then empty bypasses check (why: explicit empty must not run check for optional)",
			entries: []config.GetEntry{
				{VarName: "SIZE", Info: "Enter size", Check: "[ {{.SIZE}} -le 100 ]", Optional: true},
			},
			vars:     map[string]string{"SIZE": ""},
			wantVars: map[string]string{"SIZE": ""},
		},
		{
			name: "given optional and no-input and var preset as non-empty passing check, when executed, then succeeds (why: non-empty value still validated)",
			entries: []config.GetEntry{
				{VarName: "SIZE", Info: "Enter size", Check: "[ {{.SIZE}} -le 100 ]", Optional: true},
			},
			vars:     map[string]string{"SIZE": "50"},
			wantVars: map[string]string{"SIZE": "50"},
		},
		{
			name: "given optional and no-input and var preset as non-empty failing check, when executed, then fails (why: check applies to non-empty values even for optional)",
			entries: []config.GetEntry{
				{VarName: "SIZE", Info: "Enter size", Check: "[ {{.SIZE}} -le 100 ]", Optional: true},
			},
			vars:    map[string]string{"SIZE": "999"},
			wantErr: "validation failed",
		},
		{
			name: "given optional false and no-input and var not set, when executed, then fails (why: non-optional still requires input without a default)",
			entries: []config.GetEntry{
				{VarName: "NOTE", Info: "Enter note", Optional: false},
			},
			vars:    map[string]string{},
			wantErr: "--no-input: NOTE requires input",
		},
		{
			name: "given optional and no-input and var not set, when executed, then does not use default (why: optional uses empty not default when unset)",
			entries: []config.GetEntry{
				{VarName: "NOTE", Info: "Enter note", DefaultTmpl: "fallback", Optional: true},
			},
			vars:     map[string]string{},
			wantVars: map[string]string{"NOTE": ""},
		},
		{
			name: "given multi optional and no-input and var not set, when executed, then succeeds with empty JSON array (why: multi vars must stay valid JSON arrays for loop/options consumers, not empty string or null)",
			entries: []config.GetEntry{
				{VarName: "TAGS", Info: "Pick tags", Multi: true, Optional: true, FromList: []string{"a", "b"}},
			},
			vars:     map[string]string{},
			wantVars: map[string]string{"TAGS": "[]"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg := makeGetCfg(test.entries)
			vars := copyVars(test.vars)

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

func TestExecGet_SecretFlag_NoInput(t *testing.T) {
	tests := []struct {
		name       string
		entry      config.GetEntry
		preset     map[string]string
		wantSecret bool
	}{
		{
			name:       "given secret get and default used, when no-input executed, then var marked in secrets (why: masking required even when default fills the value)",
			entry:      config.GetEntry{VarName: "PASS", DefaultTmpl: "fallback", Secret: true},
			preset:     map[string]string{},
			wantSecret: true,
		},
		{
			name:       "given secret get and var preset, when no-input executed, then var still marked in secrets (why: skip-if-set must not bypass masking)",
			entry:      config.GetEntry{VarName: "PASS", Secret: true},
			preset:     map[string]string{"PASS": "preset"},
			wantSecret: true,
		},
		{
			name:       "given non-secret get and var preset, when no-input executed, then var not in secrets (why: non-secret vars must not be masked)",
			entry:      config.GetEntry{VarName: "ENV", DefaultTmpl: "prod"},
			preset:     map[string]string{},
			wantSecret: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			cfg := &config.ConfigFile{
				Tasks: map[string]config.Task{
					"t": {Steps: []config.Step{{Kind: config.KindGet, GetEntries: []config.GetEntry{test.entry}}}},
				},
			}
			vars := copyVars(test.preset)
			secrets := make(map[string]bool)

			// Act
			err := ExecuteTask(context.Background(), "t", &cli.Scope{Vars: vars, Secrets: secrets}, cfg, true, "")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if secrets[test.entry.VarName] != test.wantSecret {
				t.Errorf("secrets[%q]: got %v, want %v", test.entry.VarName, secrets[test.entry.VarName], test.wantSecret)
			}
		})
	}
}

func TestExecGet_TextPrompt_SetsVar(t *testing.T) {
	// given get step with text prompt, when executed, then var set to prompted value in scope (why: get is the primary mechanism for user input into scope)
	// Arrange
	orig := promptTextFn
	defer func() { promptTextFn = orig }()
	promptTextFn = func(ctx context.Context, info, check, varName string, vars map[string]string, defaultVal, task string, secret, optional bool) (string, error) {
		return "typed-value", nil
	}
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{Kind: config.KindGet, GetEntries: []config.GetEntry{
					{VarName: "NAME", Info: "Enter name"},
				}},
			}},
		},
	}
	vars := map[string]string{}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, false, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["NAME"] != "typed-value" {
		t.Errorf("NAME: got %q, want typed-value", vars["NAME"])
	}
}

func TestExecGet_SelectPrompt_SetsVar(t *testing.T) {
	// given get step with options list, when executed, then var set to selected value in scope (why: select prompt routes through promptSelectFn distinct from text prompt)
	// Arrange
	orig := promptSelectFn
	defer func() { promptSelectFn = orig }()
	promptSelectFn = func(ctx context.Context, varName, info string, items []string, multi bool, defaultVal, task string, secret bool) (string, error) {
		return "beta", nil
	}
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{Kind: config.KindGet, GetEntries: []config.GetEntry{
					{VarName: "ENV", Info: "Pick env", FromList: []string{"alpha", "beta", "gamma"}},
				}},
			}},
		},
	}
	vars := map[string]string{}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, false, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["ENV"] != "beta" {
		t.Errorf("ENV: got %q, want beta", vars["ENV"])
	}
}

func TestExecGet_SelectWithCheck_RepromptsUntilValid(t *testing.T) {
	// given check fails on first selection, when select re-prompts, then accepts second valid value
	// (why: select re-prompt loop must not stop on first failed check)
	orig := promptSelectFn
	defer func() { promptSelectFn = orig }()
	calls := 0
	promptSelectFn = func(ctx context.Context, varName, info string, items []string, multi bool, defaultVal, task string, secret bool) (string, error) {
		calls++
		if calls == 1 {
			return "bad", nil
		}
		return "good", nil
	}

	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{Kind: config.KindGet, GetEntries: []config.GetEntry{
					{
						VarName:  "CHOICE",
						FromList: []string{"bad", "good"},
						Check:    `[ "{{.CHOICE}}" = "good" ]`,
					},
				}},
			}},
		},
	}
	vars := map[string]string{}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, false, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["CHOICE"] != "good" {
		t.Errorf("CHOICE: got %q, want good", vars["CHOICE"])
	}
	if calls != 2 {
		t.Errorf("promptSelectFn called %d times, want 2", calls)
	}
}

func TestExecGet_SelectCheckErrors_NotDoublePrefixed(t *testing.T) {
	// given a check: that fails to evaluate during a select re-prompt, when the error
	// surfaces, then it carries exactly one "get VAR" prefix (why: prompt errors and
	// check errors take different wrapping paths — wrapping both at the call site
	// produced "get X: get X check: ...")
	orig := promptSelectFn
	defer func() { promptSelectFn = orig }()
	promptSelectFn = func(ctx context.Context, varName, info string, items []string, multi bool, defaultVal, task string, secret bool) (string, error) {
		return "a", nil
	}

	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{Kind: config.KindGet, GetEntries: []config.GetEntry{
					{
						VarName:  "CHOICE",
						FromList: []string{"a"},
						// unterminated quote — renders fine, fails to evaluate as a template
						Check: `{{ .MISSING | pluck "x" }}`,
					},
				}},
			}},
		},
	}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(map[string]string{}), cfg, false, t.TempDir())

	// Assert
	if err == nil {
		t.Fatal("expected error from unevaluatable check, got nil")
	}
	if got := strings.Count(err.Error(), "get CHOICE"); got != 1 {
		t.Errorf("error should carry exactly one \"get CHOICE\" prefix, got %d: %v", got, err)
	}
}

func TestExecGet_SkipIfSet_StillRunsCheck(t *testing.T) {
	// given PORT already in scope with value failing check, when get step reached,
	// then check runs and errors (why: skip-if-set must still validate via check:)
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{Kind: config.KindGet, GetEntries: []config.GetEntry{
					{
						VarName:  "PORT",
						Check:    `[ {{.PORT}} -gt 1000 ]`,
						FromList: []string{"80", "8080"},
					},
				}},
			}},
		},
	}
	vars := map[string]string{"PORT": "80"}

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(vars), cfg, true, t.TempDir())

	// Assert
	if err == nil {
		t.Fatal("expected error from failed check on preset value, got nil")
	}
}
