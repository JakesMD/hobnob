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

func TestRunDisplayLines(t *testing.T) {
	tests := []struct {
		name          string
		cmd           string
		task          string
		wantFirstHas  string
		wantRestLacks string
	}{
		{
			name:          "given single-line cmd, when rendered, then first line has run: and task prefix (why: standard display)",
			cmd:           "echo hello",
			task:          "mytask",
			wantFirstHas:  "[mytask]",
			wantRestLacks: "",
		},
		{
			name:          "given multi-line cmd, when rendered, then only first line has task prefix (why: continuation lines must not repeat prefix)",
			cmd:           "echo first\necho second\necho third",
			task:          "mytask",
			wantFirstHas:  "[mytask]",
			wantRestLacks: "[mytask]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got := RunDisplayLines(tc.cmd, tc.task)

			// Assert
			if len(got) == 0 {
				t.Fatal("got no display lines")
			}
			if !strings.Contains(got[0], tc.wantFirstHas) {
				t.Errorf("line 0: want %q in %q", tc.wantFirstHas, got[0])
			}
			if tc.wantRestLacks != "" {
				for i, line := range got[1:] {
					if strings.Contains(line, tc.wantRestLacks) {
						t.Errorf("line %d: must not contain %q, got %q", i+1, tc.wantRestLacks, line)
					}
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
			scope, _, err := BuildScope(cfg.Vars, nil, "/tmp/taskfile", "/tmp/invocation")
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

func TestCollectGetParams(t *testing.T) {
	tests := []struct {
		name      string
		steps     []config.Step
		cfg       *config.ConfigFile
		wantNames []string
	}{
		{
			name:      "given steps with no get, when collected, then returns empty (why: no params to surface)",
			steps:     []config.Step{{Kind: config.KindRun, Command: "echo hi"}},
			wantNames: nil,
		},
		{
			name: "given steps with get entries, when collected, then returns all var names (why: params are inputs to the task)",
			steps: []config.Step{{Kind: config.KindGet, GetEntries: []config.GetEntry{
				{VarName: "ENV"},
				{VarName: "PORT", DefaultTmpl: "8080"},
			}}},
			wantNames: []string{"ENV", "PORT"},
		},
		{
			name: "given for loop containing get, when collected, then includes nested entries (why: params inside loops are still task inputs)",
			steps: []config.Step{{Kind: config.KindFor, ForSteps: []config.Step{{Kind: config.KindGet, GetEntries: []config.GetEntry{
				{VarName: "ITEM_CONFIRM"},
			}}}}},
			wantNames: []string{"ITEM_CONFIRM"},
		},
		{
			name: "given mixed steps with get and for-containing-get, when collected, then returns all (why: full param surface)",
			steps: []config.Step{
				{Kind: config.KindSet, SetEntries: []config.SetEntry{{Key: "X", ValTmpl: "1"}}},
				{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "REGION"}}},
				{Kind: config.KindFor, ForSteps: []config.Step{{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "ZONE"}}}}},
				{Kind: config.KindRun, Command: "echo done"},
			},
			wantNames: []string{"REGION", "ZONE"},
		},
		{
			name: "given call step referencing sub-task with get, when collected, then includes sub-task params (why: --list misleads when called sub-task params are hidden)",
			steps: []config.Step{
				{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "TOP"}}},
				{Kind: config.KindCall, CallTarget: "_helper"},
			},
			cfg: &config.ConfigFile{
				Tasks: map[string]config.Task{
					"_helper": {Steps: []config.Step{
						{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "HELPER_VAR"}}},
					}},
				},
			},
			wantNames: []string{"TOP", "HELPER_VAR"},
		},
		{
			name: "given call step with template target, when collected, then skips dynamic call (why: cannot statically resolve template dispatch)",
			steps: []config.Step{
				{Kind: config.KindCall, CallTarget: "{{.PROFILE}}_setup"},
			},
			wantNames: nil,
		},
		{
			name: "given mutually recursive call tasks, when collected, then does not infinite loop (why: cycle guard prevents stack overflow)",
			steps: []config.Step{
				{Kind: config.KindCall, CallTarget: "b"},
			},
			cfg: &config.ConfigFile{
				Tasks: map[string]config.Task{
					"b": {Steps: []config.Step{
						{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "FROM_B"}}},
						{Kind: config.KindCall, CallTarget: "a"},
					}},
					"a": {Steps: []config.Step{
						{Kind: config.KindCall, CallTarget: "b"},
					}},
				},
			},
			wantNames: []string{"FROM_B"},
		},
		{
			name: "given set before get for same var, when collected, then omits already-set var (why: set satisfies the var so no prompt needed)",
			steps: []config.Step{
				{Kind: config.KindSet, SetEntries: []config.SetEntry{{Key: "ENV", ValTmpl: "production"}}},
				{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "ENV"}, {VarName: "PORT"}}},
			},
			wantNames: []string{"PORT"},
		},
		{
			name: "given call with: passes var, when sub-task has get for that var, then omits it (why: with: satisfies the sub-task var so no prompt needed)",
			steps: []config.Step{
				{Kind: config.KindCall, CallTarget: "_sub", CallVars: []config.SetEntry{{Key: "ENV", ValTmpl: "staging"}}},
			},
			cfg: &config.ConfigFile{
				Tasks: map[string]config.Task{
					"_sub": {Steps: []config.Step{
						{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "ENV"}, {VarName: "REGION"}}},
					}},
				},
			},
			wantNames: []string{"REGION"},
		},
		{
			name: "given run into: captures var, when later sub-task has get for that var, then omits it (why: run output satisfies the var so no prompt needed)",
			steps: []config.Step{
				{Kind: config.KindRun, Command: "git rev-parse HEAD", IntoEntries: []config.IntoEntry{{ParentKey: "SHA", ValueTmpl: "stdout | trim"}}},
				{Kind: config.KindCall, CallTarget: "_sub"},
			},
			cfg: &config.ConfigFile{
				Tasks: map[string]config.Task{
					"_sub": {Steps: []config.Step{
						{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "SHA"}, {VarName: "REGION"}}},
					}},
				},
			},
			wantNames: []string{"REGION"},
		},
		{
			name: "given get in parent and same var in sub-task get, when collected, then deduplicates (why: already surfaced once so no double prompt)",
			steps: []config.Step{
				{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "ENV"}}},
				{Kind: config.KindCall, CallTarget: "_sub"},
			},
			cfg: &config.ConfigFile{
				Tasks: map[string]config.Task{
					"_sub": {Steps: []config.Step{
						{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "ENV"}, {VarName: "REGION"}}},
					}},
				},
			},
			wantNames: []string{"ENV", "REGION"},
		},
		{
			name: "given for loop default ITEM iterator, when loop body has get for ITEM, then omits it (why: iterator is injected by loop engine not user input)",
			steps: []config.Step{
				{Kind: config.KindFor, ForSteps: []config.Step{
					{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "ITEM"}, {VarName: "CONFIRM"}}},
				}},
			},
			wantNames: []string{"CONFIRM"},
		},
		{
			name: "given for matrix loop, when loop body has get for matrix vars, then omits them (why: matrix vars are injected by loop engine not user input)",
			steps: []config.Step{
				{Kind: config.KindFor, ForMatrix: []config.ForMatrixEntry{{VarName: "OS"}, {VarName: "ARCH"}}, ForSteps: []config.Step{
					{Kind: config.KindGet, GetEntries: []config.GetEntry{{VarName: "OS"}, {VarName: "ARCH"}, {VarName: "CONFIRM"}}},
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

func TestBuildScope_GlobalVars(t *testing.T) {
	// Arrange
	cfg, err := config.ParseConfig("testdata/global_vars.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act — build scope without any CLI vars, no TIMEOUT in env
	scope, _, err := BuildScope(cfg.Vars, map[string]string{}, "/tmp/file", "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope["HOST"] != "localhost" {
		t.Errorf("HOST: got %q, want %q", scope["HOST"], "localhost")
	}
	if !strings.Contains(scope["TIMEOUT"], "30") && scope["TIMEOUT"] == "" {
		t.Errorf("TIMEOUT: got %q, want default 30", scope["TIMEOUT"])
	}
}

func TestBuildScope_SystemVars(t *testing.T) {
	// Arrange
	cfg, err := config.ParseConfig("testdata/global_vars.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act
	scope, _, err := BuildScope(cfg.Vars, nil, "/path/to/tasks", "/path/to/invocation")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope["HOBNOB_FILE_DIR"] != "/path/to/tasks" {
		t.Errorf("HOBNOB_FILE_DIR: got %q, want %q", scope["HOBNOB_FILE_DIR"], "/path/to/tasks")
	}
	if scope["HOBNOB_INVOCATION_DIR"] != "/path/to/invocation" {
		t.Errorf("HOBNOB_INVOCATION_DIR: got %q, want %q", scope["HOBNOB_INVOCATION_DIR"], "/path/to/invocation")
	}
}

func TestPrintHelp(t *testing.T) {
	// Arrange
	cfg, err := config.ParseConfig("testdata/task_info.yml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	scope, _, err := BuildScope(cfg.Vars, nil, "/tmp/taskfile", "/tmp/invocation")
	if err != nil {
		t.Fatalf("scope error: %v", err)
	}
	var buf bytes.Buffer

	// Act
	if err := PrintHelp(cfg, scope, &buf); err != nil {
		t.Fatalf("PrintHelp error: %v", err)
	}

	// Assert
	output := buf.String()
	for _, want := range []string{
		"--file",
		"--list",
		"--help",
		"--no-input",
		"https://github.com/JakesMD/hobnob/GUIDE.md",
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
	scope, _, err := BuildScope(cfg.Vars, map[string]string{"HOST": "remotehost"}, "/tmp/file", "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope["HOST"] != "localhost" {
		t.Errorf("HOST: got %q, want %q (global vars: block should override CLI arg)", scope["HOST"], "localhost")
	}
}
