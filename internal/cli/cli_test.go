package cli

import (
	"bytes"
	"strings"
	"testing"

	"hobnob/internal/config"
)

func TestCompletionScript(t *testing.T) {
	tests := []struct {
		name      string
		shell     string
		wantErr   bool
		wantParts []string
	}{
		{
			name:      "given zsh, when completion script generated, then contains compdef and _hobnob with compinit guard (why: zsh completion requires compdef directive, function, and compinit guard for shells that defer compinit)",
			shell:     "zsh",
			wantParts: []string{"_hobnob()", "hobnob --list", "compadd", "type compdef &>/dev/null", "compdef _hobnob hobnob", "CURRENT == 2"},
		},
		{
			name:      "given bash, when completion script generated, then contains complete directive and function (why: bash completion uses complete builtin)",
			shell:     "bash",
			wantParts: []string{"_hobnob_completion()", "hobnob --list", "compgen", "complete -F _hobnob_completion hobnob", "${COMP_CWORD}\" -eq 1"},
		},
		{
			name:      "given fish, when completion script generated, then contains complete command (why: fish completion uses complete builtin with -c flag)",
			shell:     "fish",
			wantParts: []string{"__fish_hobnob_tasks", "hobnob --list", "complete -c hobnob", "__fish_hobnob_no_task_given", "count (commandline -opc)"},
		},
		{
			name:    "given unknown shell, when completion script generated, then returns error (why: unsupported shells must fail fast)",
			shell:   "powershell",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc.shell is the arrangement)

			// Act
			got, err := CompletionScript(tc.shell)

			// Assert
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("script missing %q\ngot:\n%s", part, got)
				}
			}
		})
	}
}

func TestListTasks(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		wantLines  []string
		wantAbsent []string
	}{
		{
			name:    "given tasks with and without info, when listed, then shows names and rendered info templates",
			fixture: "testdata/task_info.yml",
			wantLines: []string{
				"adopt",
				"Adopt a cat from the shelter",
				"list_animals",
				"List all available animals",
				"no_info",
			},
		},
		{
			name:    "given tasks with get: params, when listed, then shows required params without parens and optional params with parens",
			fixture: "testdata/list_params.yml",
			wantLines: []string{
				"deploy",
				"Deploy the service to an environment",
				"ENV",
				"Target environment",
				"(PORT)",
				"Port to bind",
				"(RETRIES)",
				"Retry count",
				"rollback",
				"DEPLOY_ID",
				"ID of deployment to roll back",
				"CONFIRM",
				"no_params",
				"No parameters needed",
			},
			wantAbsent: []string{"required", "(default:"},
		},
		{
			name:    "given tasks with underscore-prefixed names, when listed, then internal tasks are absent (why: internal tasks must not be discoverable via CLI)",
			fixture: "testdata/internal_tasks.yml",
			wantLines: []string{
				"public",
				"another_public",
			},
			wantAbsent: []string{"_helper"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg, err := config.ParseConfig(tc.fixture)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			scope, err := BuildScope(cfg.Vars, nil, "/tmp/taskfile", "/tmp/invocation")
			if err != nil {
				t.Fatalf("scope error: %v", err)
			}
			var buf bytes.Buffer

			// Act
			if err := ListTasks(cfg, scope, &buf); err != nil {
				t.Fatalf("ListTasks error: %v", err)
			}

			// Assert
			output := buf.String()
			for _, want := range tc.wantLines {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, output)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(output, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, output)
				}
			}
		})
	}
}

func TestBuildScope_GlobalVars(t *testing.T) {
	// Arrange
	cfg, err := config.ParseConfig("testdata/global_vars.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act — build scope without any CLI vars, no TIMEOUT in env
	scope, err := BuildScope(cfg.Vars, map[string]string{}, "/tmp/file", "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["HOST"] != "localhost" {
		t.Errorf("HOST: got %q, want %q", scope.Vars["HOST"], "localhost")
	}
	if !strings.Contains(scope.Vars["TIMEOUT"], "30") && scope.Vars["TIMEOUT"] == "" {
		t.Errorf("TIMEOUT: got %q, want default 30", scope.Vars["TIMEOUT"])
	}
}

func TestBuildScope_SystemVars(t *testing.T) {
	// Arrange
	cfg, err := config.ParseConfig("testdata/global_vars.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act
	scope, err := BuildScope(cfg.Vars, nil, "/path/to/tasks", "/path/to/invocation")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["HOBNOB_FILE_DIR"] != "/path/to/tasks" {
		t.Errorf("HOBNOB_FILE_DIR: got %q, want %q", scope.Vars["HOBNOB_FILE_DIR"], "/path/to/tasks")
	}
	if scope.Vars["HOBNOB_INVOCATION_DIR"] != "/path/to/invocation" {
		t.Errorf("HOBNOB_INVOCATION_DIR: got %q, want %q", scope.Vars["HOBNOB_INVOCATION_DIR"], "/path/to/invocation")
	}
}

func TestPrintUsage(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		wantParts []string
		wantAbsent []string
	}{
		{
			name:    "given release version, when usage printed, then shows version and versioned docs URL (why: users should see docs matching their binary)",
			version: "v1.2.3",
			wantParts: []string{
				"hobnob v1.2.3",
				"--version",
				"https://github.com/JakesMD/hobnob/blob/v1.2.3/GUIDE.md",
			},
			wantAbsent: []string{"hobnob dev"},
		},
		{
			name:    "given empty version (dev build), when usage printed, then shows dev and main branch docs URL (why: dev builds fall back to main branch docs)",
			version: "",
			wantParts: []string{
				"hobnob dev",
				"--version",
				"https://github.com/JakesMD/hobnob/blob/main/GUIDE.md",
			},
			wantAbsent: []string{"hobnob v"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var buf bytes.Buffer

			// Act
			PrintUsage(&buf, tc.version)

			// Assert
			output := buf.String()
			for _, want := range tc.wantParts {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, output)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(output, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, output)
				}
			}
		})
	}
}

func TestPrintHelp(t *testing.T) {
	// Arrange
	cfg, err := config.ParseConfig("testdata/task_info.yml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	scope, err := BuildScope(cfg.Vars, nil, "/tmp/taskfile", "/tmp/invocation")
	if err != nil {
		t.Fatalf("scope error: %v", err)
	}
	var buf bytes.Buffer

	// Act
	if err := PrintHelp(cfg, scope, &buf, "v2.0.0"); err != nil {
		t.Fatalf("PrintHelp error: %v", err)
	}

	// Assert
	output := buf.String()
	for _, want := range []string{
		"--file",
		"--list",
		"--help",
		"--no-input",
		"--version",
		"hobnob v2.0.0",
		"https://github.com/JakesMD/hobnob/blob/v2.0.0/GUIDE.md",
		"adopt",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}
}

func TestBuildScope_GlobalsWinOverCLIArgs(t *testing.T) {
	// Arrange
	cfg, err := config.ParseConfig("testdata/global_vars.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act — global HOST="localhost" should win over CLI arg HOST="remotehost"
	scope, err := BuildScope(cfg.Vars, map[string]string{"HOST": "remotehost"}, "/tmp/file", "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["HOST"] != "localhost" {
		t.Errorf("HOST: got %q, want %q (global vars: block should override CLI arg)", scope.Vars["HOST"], "localhost")
	}
}
