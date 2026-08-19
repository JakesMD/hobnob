package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"hobnob/internal/config"
)

func TestBuildScope_GlobalVars(t *testing.T) {
	// given config with global vars, when BuildScope called with no CLI overrides, then vars resolved into scope (why: global vars are the baseline scope)
	// Arrange
	cfg, err := config.ParseConfig("testdata/global_vars.yml")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Act — build scope without any CLI vars, no TIMEOUT in env
	scope, err := BuildScope(context.Background(), cfg.Vars, nil, map[string]string{}, "/tmp/file", "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["HOST"].String() != "localhost" {
		t.Errorf("HOST: got %q, want %q", scope.Vars["HOST"].String(), "localhost")
	}
	if !strings.Contains(scope.Vars["TIMEOUT"].String(), "30") && scope.Vars["TIMEOUT"].IsEmpty() {
		t.Errorf("TIMEOUT: got %q, want default 30", scope.Vars["TIMEOUT"].String())
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
	scope, err := BuildScope(context.Background(), cfg.Vars, nil, nil, "/path/to/tasks", "/path/to/invocation")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["HOBNOB_FILE_DIR"].String() != "/path/to/tasks" {
		t.Errorf("HOBNOB_FILE_DIR: got %q, want %q", scope.Vars["HOBNOB_FILE_DIR"].String(), "/path/to/tasks")
	}
	if scope.Vars["HOBNOB_INVOCATION_DIR"].String() != "/path/to/invocation" {
		t.Errorf("HOBNOB_INVOCATION_DIR: got %q, want %q", scope.Vars["HOBNOB_INVOCATION_DIR"].String(), "/path/to/invocation")
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
	scope, err := BuildScope(context.Background(), cfg.Vars, nil, map[string]string{"HOST": "remotehost"}, "/tmp/file", "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["HOST"].String() != "localhost" {
		t.Errorf("HOST: got %q, want %q (global vars: block should override CLI arg)", scope.Vars["HOST"].String(), "localhost")
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
	scope, err := BuildScope(context.Background(), nil, []config.EnvFileEntry{{PathTmpl: ".env"}}, map[string]string{"FOO": "fromcli"}, dir, "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["FOO"].String() != "fromcli" {
		t.Errorf("FOO: got %q, want %q (CLI arg should override env: file)", scope.Vars["FOO"].String(), "fromcli")
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
	scope, err := BuildScope(context.Background(), cfg.Vars, []config.EnvFileEntry{{PathTmpl: ".env"}}, nil, dir, "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["HOST"].String() != "localhost" {
		t.Errorf("HOST: got %q, want %q (global vars: block should override env: file)", scope.Vars["HOST"].String(), "localhost")
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
	scope, err := BuildScope(context.Background(), nil, []config.EnvFileEntry{{PathTmpl: ".env"}}, nil, dir, "/tmp/invoc")
	if err != nil {
		t.Fatalf("unexpected BuildScope error: %v", err)
	}

	// Assert
	if scope.Vars["FOO"].String() != "fromfile" {
		t.Errorf("FOO: got %q, want %q (env: file should override OS env)", scope.Vars["FOO"].String(), "fromfile")
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
	scope, err := BuildScope(context.Background(), nil, []config.EnvFileEntry{{PathTmpl: ".env", SecretOverride: &isSecret}, {PathTmpl: "setup.sh"}}, nil, dir, "/tmp/invoc")
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
