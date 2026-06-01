package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInternalTaskRejectedByCLI(t *testing.T) {
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		os.Args = []string{"hobnob", "_helper"}
		main()
		return
	}

	// Arrange
	cmd := exec.Command(os.Args[0], "-test.run=TestInternalTaskRejectedByCLI")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1")

	// Act
	err := cmd.Run()

	// Assert
	if err == nil {
		t.Fatal("expected non-zero exit for internal task name, got success")
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got: %v", err)
	}
}

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
		wantNoPrompts bool
		wantVars      map[string]string
		wantErr       bool
	}{
		{
			name:          "given CI env set, when parsed, then noPrompts true (why: CI environments must not block on prompts)",
			args:          []string{},
			ciEnv:         "true",
			wantNoPrompts: true,
			wantVars:      map[string]string{},
		},
		{
			name:          "given CI env empty, when parsed, then noPrompts false (why: interactive mode default outside CI)",
			args:          []string{},
			ciEnv:         "",
			wantNoPrompts: false,
			wantVars:      map[string]string{},
		},
		{
			name:          "given --no-input flag, when parsed, then noPrompts true (why: explicit flag overrides default)",
			args:          []string{"--no-input"},
			ciEnv:         "",
			wantNoPrompts: true,
			wantVars:      map[string]string{},
		},
		{
			name:          "given CI env set and --no-input, when parsed, then noPrompts true (why: both sources agree)",
			args:          []string{"--no-input"},
			ciEnv:         "true",
			wantNoPrompts: true,
			wantVars:      map[string]string{},
		},
		{
			name:          "given KEY=VALUE args, when parsed, then vars populated (why: CLI overrides must be captured)",
			args:          []string{"ENV=prod", "TIMEOUT=60"},
			ciEnv:         "",
			wantNoPrompts: false,
			wantVars:      map[string]string{"ENV": "prod", "TIMEOUT": "60"},
		},
		{
			name:    "given invalid arg, when parsed, then returns error (why: unknown flags must fail fast)",
			args:    []string{"badarg"},
			ciEnv:   "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			t.Setenv("CI", tc.ciEnv)

			// Act
			gotNoPrompts, gotVars, err := parseTaskArgs(tc.args)

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
			if gotNoPrompts != tc.wantNoPrompts {
				t.Errorf("noPrompts: got %v, want %v", gotNoPrompts, tc.wantNoPrompts)
			}
			if !reflect.DeepEqual(gotVars, tc.wantVars) {
				t.Errorf("vars: got %v, want %v", gotVars, tc.wantVars)
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			gotFile, gotArgs, err := extractFileFlag(tc.args)

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
			if gotFile != tc.wantFile {
				t.Errorf("file: got %q, want %q", gotFile, tc.wantFile)
			}
			if !reflect.DeepEqual(gotArgs, tc.wantArgs) {
				t.Errorf("args: got %v, want %v", gotArgs, tc.wantArgs)
			}
		})
	}
}
