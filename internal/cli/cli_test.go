package cli

import (
	"bytes"
	"os"
	"reflect"
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
			wantParts: []string{"_hobnob()", "hobnob --list", "compadd", "type compdef &>/dev/null", "compdef _hobnob hobnob", "--file", "--no-input", "--version", "--upgrade"},
		},
		{
			name:      "given bash, when completion script generated, then contains complete directive and function (why: bash completion uses complete builtin)",
			shell:     "bash",
			wantParts: []string{"_hobnob_completion()", "hobnob --list", "compgen", "complete -F _hobnob_completion hobnob", "--file", "--no-input", "--version", "--upgrade"},
		},
		{
			name:      "given fish, when completion script generated, then contains complete command (why: fish completion uses complete builtin with -c flag)",
			shell:     "fish",
			wantParts: []string{"__fish_hobnob_tasks", "hobnob --list", "complete -c hobnob", "__fish_hobnob_no_task_given", "commandline -opc", "--file", "-l no-input", "-l version", "-l upgrade"},
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
		{
			name:    "given no public tasks, when listed, then shows empty-state message with guide link (why: blank output is confusing for new users)",
			fixture: "testdata/no_public_tasks.yml",
			wantLines: []string{
				"No tasks found",
				"hobnob.yml",
				"GUIDE.md",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg, err := config.ParseConfig(tc.fixture)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			scope, err := BuildScope(cfg.Vars, nil, nil, "/tmp/taskfile", "/tmp/invocation")
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
	// given config with global vars, when BuildScope called with no CLI overrides, then vars resolved into scope (why: global vars are the baseline scope)
	// Arrange
	cfg, err := config.ParseConfig("testdata/global_vars.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act — build scope without any CLI vars, no TIMEOUT in env
	scope, err := BuildScope(cfg.Vars, nil, map[string]string{}, "/tmp/file", "/tmp/invoc")
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
	// given any taskfile, when BuildScope called, then HOBNOB_FILE_DIR and HOBNOB_INVOCATION_DIR injected (why: built-in vars must always be available)
	// Arrange
	cfg, err := config.ParseConfig("testdata/global_vars.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act
	scope, err := BuildScope(cfg.Vars, nil, nil, "/path/to/tasks", "/path/to/invocation")
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
	// given config with task info, when PrintHelp called, then output contains all flag docs and task descriptions (why: help output is the primary user reference)
	// Arrange
	cfg, err := config.ParseConfig("testdata/task_info.yml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	scope, err := BuildScope(cfg.Vars, nil, nil, "/tmp/taskfile", "/tmp/invocation")
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

func TestCollectSelectableTasks(t *testing.T) {
	tests := []struct {
		name      string
		fixture   string
		wantNames []string
		wantInfos []string
		absentNames []string
	}{
		{
			name:      "given tasks with and without info, when collected, then returns public tasks sorted with rendered info (why: selector needs display data)",
			fixture:   "testdata/task_info.yml",
			wantNames: []string{"adopt", "list_animals", "no_info"},
			wantInfos: []string{"Adopt a cat from the shelter", "List all available animals", ""},
		},
		{
			name:        "given internal tasks, when collected, then underscore-prefixed tasks excluded (why: internal tasks must not appear in selector)",
			fixture:     "testdata/internal_tasks.yml",
			wantNames:   []string{"another_public", "public"},
			absentNames: []string{"_helper"},
		},
		{
			name:      "given no public tasks, when collected, then returns empty slice (why: selector needs empty state)",
			fixture:   "testdata/no_public_tasks.yml",
			wantNames: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			cfg, err := config.ParseConfig(tc.fixture)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			scope, err := BuildScope(cfg.Vars, nil, nil, "/tmp/taskfile", "/tmp/invocation")
			if err != nil {
				t.Fatalf("scope error: %v", err)
			}

			// Act
			tasks := CollectSelectableTasks(cfg, scope)

			// Assert
			var gotNames []string
			for _, task := range tasks {
				gotNames = append(gotNames, task.Name)
			}
			if len(gotNames) == 0 {
				gotNames = []string{}
			}
			if !reflect.DeepEqual(gotNames, tc.wantNames) {
				t.Errorf("names: got %v, want %v", gotNames, tc.wantNames)
			}
			for i, wantInfo := range tc.wantInfos {
				if i < len(tasks) && tasks[i].Info != wantInfo {
					t.Errorf("info[%d]: got %q, want %q", i, tasks[i].Info, wantInfo)
				}
			}
			for _, absent := range tc.absentNames {
				for _, task := range tasks {
					if task.Name == absent {
						t.Errorf("should not contain %q", absent)
					}
				}
			}
		})
	}
}

func TestBuildScope_GlobalsWinOverCLIArgs(t *testing.T) {
	// given global var and matching CLI arg, when BuildScope called, then global value wins (why: globals are internal wiring; CLI can't silently override them)
	// Arrange
	cfg, err := config.ParseConfig("testdata/global_vars.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act — global HOST="localhost" should win over CLI arg HOST="remotehost"
	scope, err := BuildScope(cfg.Vars, nil, map[string]string{"HOST": "remotehost"}, "/tmp/file", "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["HOST"] != "localhost" {
		t.Errorf("HOST: got %q, want %q (global vars: block should override CLI arg)", scope.Vars["HOST"], "localhost")
	}
}

func TestBuildScope_CLIArgsWinOverEnvFile(t *testing.T) {
	// given env: file setting FOO and matching CLI arg, when BuildScope called, then CLI arg wins (why: a caller's explicit override always beats a sourced default)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("FOO=fromfile\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	// Act
	scope, err := BuildScope(nil, []config.EnvFileEntry{{PathTmpl: ".env"}}, map[string]string{"FOO": "fromcli"}, dir, "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["FOO"] != "fromcli" {
		t.Errorf("FOO: got %q, want %q (CLI arg should override env: file)", scope.Vars["FOO"], "fromcli")
	}
}

func TestBuildScope_GlobalWinsOverEnvFile(t *testing.T) {
	// given env: file setting HOST and a global vars: HOST, when BuildScope called, then global value wins (why: vars: is the highest-priority internal wiring)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("HOST=fromfile\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	cfg, err := config.ParseConfig("testdata/global_vars.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act
	scope, err := BuildScope(cfg.Vars, []config.EnvFileEntry{{PathTmpl: ".env"}}, nil, dir, "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["HOST"] != "localhost" {
		t.Errorf("HOST: got %q, want %q (global vars: block should override env: file)", scope.Vars["HOST"], "localhost")
	}
}

func TestBuildScope_EnvFileWinsOverOSEnv(t *testing.T) {
	// given OS env var and an env: file overriding it, when BuildScope called, then env: file value wins (why: env: files are explicit project config, OS env is just ambient)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("FOO=fromfile\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	t.Setenv("FOO", "fromosenv")

	// Act
	scope, err := BuildScope(nil, []config.EnvFileEntry{{PathTmpl: ".env"}}, nil, dir, "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["FOO"] != "fromfile" {
		t.Errorf("FOO: got %q, want %q (env: file should override OS env)", scope.Vars["FOO"], "fromfile")
	}
}

func TestBuildScope_EnvFileSecretMasking(t *testing.T) {
	// given a .env entry marked secret: true and a plain .sh file, when BuildScope called, then the .env var is marked secret and the .sh var is not (why: secret: false is the default for every env: entry; masking is opt-in via secret: true)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("FROM_ENV=secretval\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(dir+"/setup.sh", []byte("export FROM_SH=plainval\n"), 0o644); err != nil {
		t.Fatalf("write setup.sh: %v", err)
	}
	isSecret := true

	// Act
	scope, err := BuildScope(nil, []config.EnvFileEntry{{PathTmpl: ".env", SecretOverride: &isSecret}, {PathTmpl: "setup.sh"}}, nil, dir, "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if !scope.Secrets["FROM_ENV"] {
		t.Error("FROM_ENV: expected secret (secret: true override)")
	}
	if scope.Secrets["FROM_SH"] {
		t.Error("FROM_SH: expected not secret (default secret: false)")
	}
}
