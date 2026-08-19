package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFindTaskfile(t *testing.T) {
	t.Run("given hobnob.yml in start dir, when searching, then returns it (why: nearest file wins)", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		expected := filepath.Join(dir, "hobnob.yml")
		os.WriteFile(expected, []byte("tasks: {}"), 0644)

		// Act
		got, err := findTaskfile(dir)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("given hobnob.yaml in start dir, when searching, then returns it (why: both extensions supported)", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		expected := filepath.Join(dir, "hobnob.yaml")
		os.WriteFile(expected, []byte("tasks: {}"), 0644)

		// Act
		got, err := findTaskfile(dir)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("given hobnob.yml and hobnob.yaml in same dir, when searching, then returns .yml first (why: .yml takes priority over .yaml)", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()
		expected := filepath.Join(dir, "hobnob.yml")
		os.WriteFile(expected, []byte("tasks: {}"), 0644)
		os.WriteFile(filepath.Join(dir, "hobnob.yaml"), []byte("tasks: {}"), 0644)

		// Act
		got, err := findTaskfile(dir)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("given taskfile only in parent dir, when searching from child, then finds parent file (why: walks up directory tree)", func(t *testing.T) {
		// Arrange
		parent := t.TempDir()
		expected := filepath.Join(parent, "hobnob.yml")
		os.WriteFile(expected, []byte("tasks: {}"), 0644)
		child := filepath.Join(parent, "subdir")
		os.Mkdir(child, 0755)

		// Act
		got, err := findTaskfile(child)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("given taskfile in both start dir and parent, when searching, then returns start dir file (why: nearest file takes priority)", func(t *testing.T) {
		// Arrange
		parent := t.TempDir()
		os.WriteFile(filepath.Join(parent, "hobnob.yml"), []byte("tasks: {}"), 0644)
		child := filepath.Join(parent, "subdir")
		os.Mkdir(child, 0755)
		expected := filepath.Join(child, "hobnob.yml")
		os.WriteFile(expected, []byte("tasks: {}"), 0644)

		// Act
		got, err := findTaskfile(child)

		// Assert
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("given no taskfile anywhere, when searching, then returns error (why: explicit error prevents silent failure)", func(t *testing.T) {
		// Arrange
		dir := t.TempDir()

		// Act
		_, err := findTaskfile(dir)

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestParseTaskArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		ciEnv         string
		isTerminal    bool
		wantNoPrompts bool
		wantVars      map[string]string
		wantErr       bool
	}{
		{
			name:          "given CI env set, when parsed, then noPrompts true (why: CI environments must not block on prompts)",
			args:          []string{},
			ciEnv:         "true",
			isTerminal:    true,
			wantNoPrompts: true,
			wantVars:      map[string]string{},
		},
		{
			name:          "given CI env empty and stdin is a terminal, when parsed, then noPrompts false (why: interactive mode default for humans)",
			args:          []string{},
			ciEnv:         "",
			isTerminal:    true,
			wantNoPrompts: false,
			wantVars:      map[string]string{},
		},
		{
			name:          "given --no-input flag, when parsed, then noPrompts true (why: explicit flag overrides default)",
			args:          []string{"--no-input"},
			ciEnv:         "",
			isTerminal:    true,
			wantNoPrompts: true,
			wantVars:      map[string]string{},
		},
		{
			name:          "given CI env set and --no-input, when parsed, then noPrompts true (why: both sources agree)",
			args:          []string{"--no-input"},
			ciEnv:         "true",
			isTerminal:    true,
			wantNoPrompts: true,
			wantVars:      map[string]string{},
		},
		{
			name:          "given KEY=VALUE args, when parsed, then vars populated (why: CLI overrides must be captured)",
			args:          []string{"ENV=prod", "TIMEOUT=60"},
			ciEnv:         "",
			isTerminal:    true,
			wantNoPrompts: false,
			wantVars:      map[string]string{"ENV": "prod", "TIMEOUT": "60"},
		},
		{
			name:    "given invalid arg, when parsed, then returns error (why: unknown flags must fail fast)",
			args:    []string{"badarg"},
			ciEnv:   "",
			wantErr: true,
		},
		{
			name:          "given CI env empty and stdin is not a terminal, when parsed, then noPrompts true (why: AI agents and other non-interactive callers pipe stdin and can't answer prompts)",
			args:          []string{},
			ciEnv:         "",
			isTerminal:    false,
			wantNoPrompts: true,
			wantVars:      map[string]string{},
		},
		{
			name:          "given stdin is not a terminal but --no-input is not passed, when parsed, then noPrompts still true (why: non-tty detection alone is sufficient, no flag required)",
			args:          []string{"ENV=prod"},
			ciEnv:         "",
			isTerminal:    false,
			wantNoPrompts: true,
			wantVars:      map[string]string{"ENV": "prod"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			t.Setenv("CI", test.ciEnv)
			a := &App{IsTerminal: func() bool { return test.isTerminal }}

			// Act
			gotNoPrompts, gotVars, err := a.parseTaskArgs(test.args)

			// Assert
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotNoPrompts != test.wantNoPrompts {
				t.Errorf("noPrompts: got %v, want %v", gotNoPrompts, test.wantNoPrompts)
			}
			if !reflect.DeepEqual(gotVars, test.wantVars) {
				t.Errorf("vars: got %v, want %v", gotVars, test.wantVars)
			}
		})
	}
}

// TestDefaultNoPrompts covers the CI/terminal decision shared by every code
// path that computes noPrompts (no-args default task, --select, and
// parseTaskArgs via defaultNoPrompts). Regression for a bug where two of the
// three call sites checked CI but not IsTerminal, so piped/non-terminal
// stdin still tried to open a prompt instead of failing fast.
func TestDefaultNoPrompts(t *testing.T) {
	tests := []struct {
		name       string
		ciEnv      string
		isTerminal bool
		want       bool
	}{
		{
			name:       "given CI set, when checked, then true regardless of terminal (why: CI runners are never interactive even if stdin looks like a tty)",
			ciEnv:      "true",
			isTerminal: true,
			want:       true,
		},
		{
			name:       "given CI empty and stdin is not a terminal, when checked, then true (why: non-interactive callers like AI agents can't answer a prompt)",
			ciEnv:      "",
			isTerminal: false,
			want:       true,
		},
		{
			name:       "given CI empty and stdin is a terminal, when checked, then false (why: a human at a real terminal should still see prompts)",
			ciEnv:      "",
			isTerminal: true,
			want:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			t.Setenv("CI", test.ciEnv)
			a := &App{IsTerminal: func() bool { return test.isTerminal }}

			// Act
			got := a.defaultNoPrompts()

			// Assert
			if got != test.want {
				t.Errorf("defaultNoPrompts(): got %v, want %v", got, test.want)
			}
		})
	}
}

func TestExtractFileFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantFile string
		wantArgs []string
		wantErr  bool
	}{
		{
			name:     "given --file at start, when extracted, then returns value and remaining args (why: flag must be consumed)",
			args:     []string{"--file", "custom.yml", "mytask"},
			wantFile: "custom.yml",
			wantArgs: []string{"mytask"},
		},
		{
			name:     "given --file in middle of args, when extracted, then removes both tokens (why: order-independent flag)",
			args:     []string{"mytask", "--file", "custom.yml", "KEY=VAL"},
			wantFile: "custom.yml",
			wantArgs: []string{"mytask", "KEY=VAL"},
		},
		{
			name:     "given no --file flag, when extracted, then returns empty string and all args unchanged (why: optional flag)",
			args:     []string{"mytask", "KEY=VAL"},
			wantFile: "",
			wantArgs: []string{"mytask", "KEY=VAL"},
		},
		{
			name:    "given --file with no value, when extracted, then returns error (why: flag requires an argument)",
			args:    []string{"--file"},
			wantErr: true,
		},
		{
			name:    "given --file as last arg before nothing, when extracted, then returns error (why: value cannot be omitted)",
			args:    []string{"mytask", "--file"},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			gotFile, gotArgs, err := extractFileFlag(test.args)

			// Assert
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotFile != test.wantFile {
				t.Errorf("file: got %q, want %q", gotFile, test.wantFile)
			}
			if !reflect.DeepEqual(gotArgs, test.wantArgs) {
				t.Errorf("args: got %v, want %v", gotArgs, test.wantArgs)
			}
		})
	}
}
