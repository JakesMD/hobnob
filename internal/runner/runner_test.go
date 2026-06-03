package runner

import (
	"fmt"
	"os"
	"path/filepath"
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
		name      string
		entries   []config.GetEntry
		vars      map[string]string
		wantErr   string
		wantVars  map[string]string
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := makeGetCfg(tc.entries)
			vars := make(map[string]string)
			for k, v := range tc.vars {
				vars[k] = v
			}

			// Act
			err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: make(map[string]bool)}, cfg, true, "")

			// Assert
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error: got %q, want it to contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, want := range tc.wantVars {
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := makeGetCfg(tc.entries)
			vars := make(map[string]string)
			for k, v := range tc.vars {
				vars[k] = v
			}

			// Act
			err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: make(map[string]bool)}, cfg, true, "")

			// Assert
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error: got %q, want it to contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, want := range tc.wantVars {
				if got := vars[k]; got != want {
					t.Errorf("vars[%s]: got %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestMaskSecrets(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		vars    map[string]string
		secrets map[string]bool
		want    string
	}{
		{
			name:    "given no secrets, when masking, then string unchanged (why: nothing to mask)",
			input:   "deploy --user=alice --pass=hunter2",
			vars:    map[string]string{"PASS": "hunter2"},
			secrets: map[string]bool{},
			want:    "deploy --user=alice --pass=hunter2",
		},
		{
			name:    "given secret var in command, when masking, then value replaced with **** (why: secret must not appear in logs)",
			input:   "deploy --user=alice --pass=hunter2",
			vars:    map[string]string{"PASS": "hunter2"},
			secrets: map[string]bool{"PASS": true},
			want:    "deploy --user=alice --pass=****",
		},
		{
			name:    "given secret value appears multiple times, when masking, then all replaced (why: full redaction required)",
			input:   "echo hunter2 && login --pass=hunter2",
			vars:    map[string]string{"PASS": "hunter2"},
			secrets: map[string]bool{"PASS": true},
			want:    "echo **** && login --pass=****",
		},
		{
			name:    "given secret var with empty value, when masking, then string unchanged (why: empty string replacement would corrupt output)",
			input:   "deploy --pass=",
			vars:    map[string]string{"PASS": ""},
			secrets: map[string]bool{"PASS": true},
			want:    "deploy --pass=",
		},
		{
			name:    "given multiple secret vars, when masking, then all replaced (why: each secret must be redacted)",
			input:   "connect --user=root --pass=s3cr3t --token=abc123",
			vars:    map[string]string{"PASS": "s3cr3t", "TOKEN": "abc123"},
			secrets: map[string]bool{"PASS": true, "TOKEN": true},
			want:    "connect --user=root --pass=**** --token=****",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange (tc fields are the arrangement)

			// Act
			got := maskSecrets(tc.input, &cli.Scope{Vars: tc.vars, Secrets: tc.secrets})

			// Assert
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := &config.ConfigFile{
				Tasks: map[string]config.Task{
					"t": {Steps: []config.Step{{Kind: config.KindSet, SetEntries: tc.entries}}},
				},
			}
			secrets := make(map[string]bool)

			// Act
			err := ExecuteTask("t", &cli.Scope{Vars: map[string]string{}, Secrets: secrets}, cfg, true, "")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k := range tc.wantSecrets {
				if !secrets[k] {
					t.Errorf("secrets[%q]: want true, got false", k)
				}
			}
			for _, k := range tc.wantNoSecrets {
				if secrets[k] {
					t.Errorf("secrets[%q]: want false, got true", k)
				}
			}
		})
	}
}

func TestExecGet_SecretFlag_NoInput(t *testing.T) {
	tests := []struct {
		name        string
		entry       config.GetEntry
		preset      map[string]string
		wantSecret  bool
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := &config.ConfigFile{
				Tasks: map[string]config.Task{
					"t": {Steps: []config.Step{{Kind: config.KindGet, GetEntries: []config.GetEntry{tc.entry}}}},
				},
			}
			vars := make(map[string]string)
			for k, v := range tc.preset {
				vars[k] = v
			}
			secrets := make(map[string]bool)

			// Act
			err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: secrets}, cfg, true, "")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if secrets[tc.entry.VarName] != tc.wantSecret {
				t.Errorf("secrets[%q]: got %v, want %v", tc.entry.VarName, secrets[tc.entry.VarName], tc.wantSecret)
			}
		})
	}
}

func makeForMatrixCfg(matrix []config.ForMatrixEntry, innerTmpl string) *config.ConfigFile {
	return &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{{
				Kind:      config.KindFor,
				ForMatrix: matrix,
				ForSteps: []config.Step{{
					Kind:       config.KindSet,
					SetEntries: []config.SetEntry{{Key: "RESULT", ValTmpl: innerTmpl}},
				}},
			}}},
		},
	}
}

func TestExecFor_Matrix(t *testing.T) {
	tests := []struct {
		name       string
		matrix     []config.ForMatrixEntry
		innerTmpl  string
		initVars   map[string]string
		wantResult string
	}{
		{
			name: "given single-var map form, when executed, then iterates using named variable (why: map form names iterator without as:)",
			matrix: []config.ForMatrixEntry{
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos", "windows"}},
			},
			innerTmpl:  "{{.RESULT}} {{.PLATFORM}}",
			initVars:   map[string]string{"RESULT": ""},
			wantResult: " ubuntu macos windows",
		},
		{
			name: "given two-var matrix, when executed, then runs full cartesian product",
			matrix: []config.ForMatrixEntry{
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos"}},
				{VarName: "DB", List: []string{"postgres", "sqlite"}},
			},
			innerTmpl:  "{{.RESULT}} {{.PLATFORM}}/{{.DB}}",
			initVars:   map[string]string{"RESULT": ""},
			wantResult: " ubuntu/postgres ubuntu/sqlite macos/postgres macos/sqlite",
		},
		{
			name: "given two-var matrix with reversed key order, when executed, then second key becomes inner loop (why: first key is outermost per spec)",
			matrix: []config.ForMatrixEntry{
				{VarName: "DB", List: []string{"postgres", "sqlite"}},
				{VarName: "PLATFORM", List: []string{"ubuntu", "macos"}},
			},
			innerTmpl:  "{{.RESULT}} {{.DB}}/{{.PLATFORM}}",
			initVars:   map[string]string{"RESULT": ""},
			wantResult: " postgres/ubuntu postgres/macos sqlite/ubuntu sqlite/macos",
		},
		{
			name: "given three-var matrix, when executed, then runs all combinations in correct nesting order",
			matrix: []config.ForMatrixEntry{
				{VarName: "OS", List: []string{"linux", "macos"}},
				{VarName: "ARCH", List: []string{"amd64", "arm64"}},
				{VarName: "DB", List: []string{"pg"}},
			},
			innerTmpl:  "{{.RESULT}} {{.OS}}/{{.ARCH}}/{{.DB}}",
			initVars:   map[string]string{"RESULT": ""},
			wantResult: " linux/amd64/pg linux/arm64/pg macos/amd64/pg macos/arm64/pg",
		},
		{
			name: "given single-var matrix with one item, when executed, then runs exactly once",
			matrix: []config.ForMatrixEntry{
				{VarName: "ENV", List: []string{"prod"}},
			},
			innerTmpl:  "{{.RESULT}}{{.ENV}}",
			initVars:   map[string]string{"RESULT": ""},
			wantResult: "prod",
		},
		{
			name: "given matrix with dynamic list template, when executed, then resolves list from vars",
			matrix: []config.ForMatrixEntry{
				{VarName: "NODE", ListTmpl: `{{.SERVERS}}`},
			},
			innerTmpl:  "{{.RESULT}} {{.NODE}}",
			initVars:   map[string]string{"RESULT": "", "SERVERS": `["web1","web2"]`},
			wantResult: " web1 web2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := makeForMatrixCfg(tc.matrix, tc.innerTmpl)
			vars := make(map[string]string)
			for k, v := range tc.initVars {
				vars[k] = v
			}

			// Act
			err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: make(map[string]bool)}, cfg, true, "")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if vars["RESULT"] != tc.wantResult {
				t.Errorf("RESULT: got %q, want %q", vars["RESULT"], tc.wantResult)
			}
		})
	}
}

func makeForStringCfg(forList []string, forTarget, innerTmpl string) *config.ConfigFile {
	return &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{{
				Kind:      config.KindFor,
				ForList:   forList,
				ForTarget: forTarget,
				ForSteps: []config.Step{{
					Kind:       config.KindSet,
					SetEntries: []config.SetEntry{{Key: "RESULT", ValTmpl: innerTmpl}},
				}},
			}}},
		},
	}
}

func TestExecFor_String(t *testing.T) {
	tests := []struct {
		name       string
		forList    []string
		forTarget  string
		innerTmpl  string
		initVars   map[string]string
		wantResult string
	}{
		{
			name:       "given literal list, when executed, then iterates binding each item to ITEM (why: string form uses ITEM as default iterator)",
			forList:    []string{"alpha", "beta", "gamma"},
			innerTmpl:  "{{.RESULT}} {{.ITEM}}",
			initVars:   map[string]string{"RESULT": ""},
			wantResult: " alpha beta gamma",
		},
		{
			name:       "given template target resolving to JSON array, when executed, then iterates resolved items (why: dynamic lists evaluated at runtime)",
			forTarget:  "{{.FILES}}",
			innerTmpl:  "{{.RESULT}} {{.ITEM}}",
			initVars:   map[string]string{"RESULT": "", "FILES": `["x","y","z"]`},
			wantResult: " x y z",
		},
		{
			name:       "given empty list, when executed, then runs zero iterations (why: empty source must not error)",
			forList:    []string{},
			innerTmpl:  "{{.RESULT}} {{.ITEM}}",
			initVars:   map[string]string{"RESULT": "initial"},
			wantResult: "initial",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := makeForStringCfg(tc.forList, tc.forTarget, tc.innerTmpl)
			vars := make(map[string]string)
			for k, v := range tc.initVars {
				vars[k] = v
			}

			// Act
			err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: make(map[string]bool)}, cfg, true, "")

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if vars["RESULT"] != tc.wantResult {
				t.Errorf("RESULT: got %q, want %q", vars["RESULT"], tc.wantResult)
			}
		})
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
	if err := ExecuteTask("t", &cli.Scope{Vars: map[string]string{}, Secrets: make(map[string]bool)}, cfg, true, parentDir); err != nil {
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			step := config.Step{Kind: config.KindRun, DirTmpl: tc.stepDirTmpl}

			// Act
			got := captureRunDir(t, step, taskfileDir, parentDir)

			// Assert
			if got != tc.wantDir {
				t.Errorf("run dir: got %q, want %q", got, tc.wantDir)
			}
		})
	}
}

func TestRunInto(t *testing.T) {
	makeRunCfg := func(command string, into []config.IntoEntry) *config.ConfigFile {
		return &config.ConfigFile{
			Tasks: map[string]config.Task{
				"t": {Steps: []config.Step{{Kind: config.KindRun, Command: command, IntoEntries: into}}},
			},
		}
	}

	tests := []struct {
		name     string
		command  string
		into     []config.IntoEntry
		wantVars map[string]string
		wantErr  string
	}{
		{
			name:    "given stdout into, when command prints to stdout, then captures raw stdout (why: basic capture without any pipe transform)",
			command: "printf 'hello'",
			into:    []config.IntoEntry{{ParentKey: "OUT", ValueTmpl: "stdout"}},
			wantVars: map[string]string{"OUT": "hello"},
		},
		{
			name:    "given stderr into, when command prints to stderr, then captures raw stderr (why: error output captured separately from stdout)",
			command: "printf 'err msg' >&2",
			into:    []config.IntoEntry{{ParentKey: "ERR", ValueTmpl: "stderr"}},
			wantVars: map[string]string{"ERR": "err msg"},
		},
		{
			name:    "given stdout | trim into, when command echoes with newline, then trims whitespace (why: common single-value capture pattern)",
			command: "printf 'hello\n'",
			into:    []config.IntoEntry{{ParentKey: "OUT", ValueTmpl: "stdout | trim"}},
			wantVars: map[string]string{"OUT": "hello"},
		},
		{
			name:    "given stdout | lines into, when command outputs multiple lines, then returns JSON array (why: captures list output into loop-ready var)",
			command: "printf 'alpha\nbeta\ngamma'",
			into:    []config.IntoEntry{{ParentKey: "ITEMS", ValueTmpl: "stdout | lines"}},
			wantVars: map[string]string{"ITEMS": `["alpha","beta","gamma"]`},
		},
		{
			name:    "given stdout | upper into, when command outputs lowercase, then stores uppercase (why: normalise captured output)",
			command: "printf 'hello'",
			into:    []config.IntoEntry{{ParentKey: "OUT", ValueTmpl: "stdout | upper"}},
			wantVars: map[string]string{"OUT": "HELLO"},
		},
		{
			name:    "given stdout | lower into, when command outputs uppercase, then stores lowercase (why: normalise captured output)",
			command: "printf 'HELLO'",
			into:    []config.IntoEntry{{ParentKey: "OUT", ValueTmpl: "stdout | lower"}},
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
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := makeRunCfg(tc.command, tc.into)
			vars := map[string]string{}

			// Act
			err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: make(map[string]bool)}, cfg, true, "")

			// Assert
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error: got %q, want it to contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, want := range tc.wantVars {
				if got := vars[k]; got != want {
					t.Errorf("vars[%s]: got %q, want %q", k, got, want)
				}
			}
		})
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
		name         string
		callDirTmpl  string
		taskDirTmpl  string
		wantDir      string
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg := &config.ConfigFile{
				Tasks: map[string]config.Task{
					"parent": {Steps: []config.Step{
						{Kind: config.KindCall, CallTarget: "child", DirTmpl: tc.callDirTmpl},
					}},
					"child": {Dir: tc.taskDirTmpl, Steps: childSteps},
				},
				TaskfileDir: t.TempDir(),
			}

			// Act
			if err := ExecuteTask("parent", &cli.Scope{Vars: map[string]string{}, Secrets: make(map[string]bool)}, cfg, true, parentDir); err != nil {
				t.Fatalf("ExecuteTask: %v", err)
			}
			data, err := os.ReadFile(outFile)
			if err != nil {
				t.Fatalf("read out file: %v", err)
			}

			// Assert
			got := strings.TrimRight(string(data), "\n")
			if got != tc.wantDir {
				t.Errorf("child run dir: got %q, want %q", got, tc.wantDir)
			}
		})
	}
}

func TestExecFor_IteratorVarRemovedAfterLoop(t *testing.T) {
	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{
					Kind:    config.KindFor,
					ForList: []string{"a", "b", "c"},
					ForSteps: []config.Step{
						{Kind: config.KindSet, SetEntries: []config.SetEntry{
							{Key: "RESULT", ValTmpl: "{{.RESULT}}{{.ITEM}}"},
						}},
					},
				},
			}},
		},
	}
	vars := map[string]string{"RESULT": ""}

	// Act
	err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["RESULT"] != "abc" {
		t.Errorf("RESULT: got %q, want abc", vars["RESULT"])
	}
	if _, exists := vars["ITEM"]; exists {
		t.Errorf("ITEM should not exist after loop, got %q", vars["ITEM"])
	}
}

func TestExecFor_IteratorVarRestoredIfPreexisting(t *testing.T) {
	// given ITEM exists before loop, when loop ends, then ITEM restored to prior value
	// (why: loop must not clobber caller's variable with the same name)
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{
					Kind:    config.KindFor,
					ForList: []string{"x"},
					ForSteps: []config.Step{
						{Kind: config.KindSet, SetEntries: []config.SetEntry{
							{Key: "RESULT", ValTmpl: "{{.ITEM}}"},
						}},
					},
				},
			}},
		},
	}
	vars := map[string]string{"ITEM": "original", "RESULT": ""}

	// Act
	err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["ITEM"] != "original" {
		t.Errorf("ITEM: got %q, want original (should be restored)", vars["ITEM"])
	}
}

func TestExecForMatrix_IteratorVarsRemovedAfterLoop(t *testing.T) {
	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{
					Kind: config.KindFor,
					ForMatrix: []config.ForMatrixEntry{
						{VarName: "OS", List: []string{"linux", "mac"}},
						{VarName: "ARCH", List: []string{"amd64"}},
					},
					ForSteps: []config.Step{
						{Kind: config.KindSet, SetEntries: []config.SetEntry{
							{Key: "RESULT", ValTmpl: "{{.RESULT}} {{.OS}}/{{.ARCH}}"},
						}},
					},
				},
			}},
		},
	}
	vars := map[string]string{"RESULT": ""}

	// Act
	err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := vars["OS"]; exists {
		t.Errorf("OS should not exist after matrix loop, got %q", vars["OS"])
	}
	if _, exists := vars["ARCH"]; exists {
		t.Errorf("ARCH should not exist after matrix loop, got %q", vars["ARCH"])
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
	err := ExecuteTask("parent", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, true, t.TempDir())

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
	err := ExecuteTask("parent", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, true, t.TempDir())

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
	err := ExecuteTask("parent", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, true, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["RESULT"] != "ok" {
		t.Errorf("RESULT: got %q, want ok", vars["RESULT"])
	}
}

func TestExecGet_TextPrompt_SetsVar(t *testing.T) {
	// Arrange
	orig := promptTextFn
	defer func() { promptTextFn = orig }()
	promptTextFn = func(info, check, varName string, vars map[string]string, defaultVal, task string, secret, optional bool) (string, error) {
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
	err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, false, t.TempDir())

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["NAME"] != "typed-value" {
		t.Errorf("NAME: got %q, want typed-value", vars["NAME"])
	}
}

func TestExecGet_SelectPrompt_SetsVar(t *testing.T) {
	// Arrange
	orig := promptSelectFn
	defer func() { promptSelectFn = orig }()
	promptSelectFn = func(varName, info string, items []string, multi bool, defaultVal, task string, secret bool) (string, error) {
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
	err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, false, t.TempDir())

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
	promptSelectFn = func(varName, info string, items []string, multi bool, defaultVal, task string, secret bool) (string, error) {
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
	err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, false, t.TempDir())

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
	err := ExecuteTask("t", &cli.Scope{Vars: vars, Secrets: map[string]bool{}}, cfg, true, t.TempDir())

	// Assert
	if err == nil {
		t.Fatal("expected error from failed check on preset value, got nil")
	}
}
