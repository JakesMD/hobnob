package config

import (
	"os"
	"testing"
)

func TestLoadEnvFiles_DotenvParsing(t *testing.T) {
	// given a .env file with comments, blank lines, export prefix, and quoted values, when loaded, then only real KEY=VALUE lines are parsed (why: dotenv files commonly mix comments and export-style declarations)
	// Arrange
	dir := t.TempDir()
	content := "# a comment\n\nFOO=bar\nexport BAZ=qux\nQUOTED=\"hello world\"\nSINGLE='single quoted'\n"
	if err := os.WriteFile(dir+"/.env", []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	// Act
	got, _, err := LoadEnvFiles([]EnvFileEntry{{PathTmpl: ".env"}}, dir, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	want := map[string]string{
		"FOO":    "bar",
		"BAZ":    "qux",
		"QUOTED": "hello world",
		"SINGLE": "single quoted",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("len: got %d, want %d (got=%v)", len(got), len(want), got)
	}
}

func TestLoadEnvFiles_MissingFile(t *testing.T) {
	// given an env: entry pointing at a nonexistent file, when loaded, then no error and no vars are produced for that entry (why: a typo'd or optional env file should warn, not abort every task)
	// Arrange
	dir := t.TempDir()

	// Act
	got, _, err := LoadEnvFiles([]EnvFileEntry{{PathTmpl: "does-not-exist.env"}}, dir, map[string]string{})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want no vars from a missing file", got)
	}
}

func TestLoadEnvFiles_MissingFileWarnsButLaterEntriesStillLoad(t *testing.T) {
	// given a missing env: entry followed by a real one, when loaded, then the missing entry is skipped and the later entry still loads (why: one typo'd path shouldn't block the rest of the list)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/present.env", []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatalf("write present.env: %v", err)
	}

	// Act
	got, _, err := LoadEnvFiles([]EnvFileEntry{{PathTmpl: "does-not-exist.env"}, {PathTmpl: "present.env"}}, dir, map[string]string{})

	// Assert
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["FOO"] != "bar" {
		t.Errorf("FOO: got %q, want %q", got["FOO"], "bar")
	}
}

func TestLoadEnvFiles_RelativePathResolvesAgainstTaskfileDir(t *testing.T) {
	// given a relative env: path, when loaded, then resolved against taskfileDir rather than the process cwd (why: all relative paths in a hobnob file resolve against the taskfile's directory)
	// Arrange
	dir := t.TempDir()
	if err := os.Mkdir(dir+"/sub", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(dir+"/sub/.env", []byte("FOO=nested\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	// Act
	got, _, err := LoadEnvFiles([]EnvFileEntry{{PathTmpl: "sub/.env"}}, dir, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	if got["FOO"] != "nested" {
		t.Errorf("FOO: got %q, want %q", got["FOO"], "nested")
	}
}

func TestLoadEnvFiles_LaterEntryWinsOverEarlier(t *testing.T) {
	// given two env: files that both set FOO, when loaded, then the later file's value wins (why: env: is an ordered list, matching how later steps override earlier scope)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/first.env", []byte("FOO=first\n"), 0o644); err != nil {
		t.Fatalf("write first.env: %v", err)
	}
	if err := os.WriteFile(dir+"/second.env", []byte("FOO=second\n"), 0o644); err != nil {
		t.Fatalf("write second.env: %v", err)
	}

	// Act
	got, _, err := LoadEnvFiles([]EnvFileEntry{{PathTmpl: "first.env"}, {PathTmpl: "second.env"}}, dir, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	if got["FOO"] != "second" {
		t.Errorf("FOO: got %q, want %q", got["FOO"], "second")
	}
}

func TestLoadEnvFiles_NotSecretByDefault(t *testing.T) {
	// given a .env file with no explicit secret: setting, when loaded, then vars are not marked secret (why: secret: false is the default for every env: entry regardless of filename; masking is opt-in)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	// Act
	got, secrets, err := LoadEnvFiles([]EnvFileEntry{{PathTmpl: ".env"}}, dir, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	if got["FOO"] != "bar" {
		t.Errorf("FOO: got %q, want %q", got["FOO"], "bar")
	}
	if secrets["FOO"] {
		t.Error("FOO: expected not secret, secret: false is the default for all env: entries")
	}
}

func TestLoadEnvFiles_ShFilesNotSecretByDefault(t *testing.T) {
	// given a .sh file that exports a var, when loaded, then the var is not marked secret (why: default is secret: false for every entry, .sh included)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/setup.sh", []byte("export FOO=bar\n"), 0o644); err != nil {
		t.Fatalf("write setup.sh: %v", err)
	}

	// Act
	got, secrets, err := LoadEnvFiles([]EnvFileEntry{{PathTmpl: "setup.sh"}}, dir, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	if got["FOO"] != "bar" {
		t.Errorf("FOO: got %q, want %q", got["FOO"], "bar")
	}
	if secrets["FOO"] {
		t.Error("FOO: expected not secret, .sh files default to secret: false")
	}
}

func TestLoadEnvFiles_LaterFileSecretOverrideDeterminesStatus(t *testing.T) {
	// given a secret: true .env file setting FOO followed by a plain .sh file overriding FOO, when loaded, then FOO is not secret (why: secret status should reflect the entry the final value actually came from)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("FOO=fromenv\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(dir+"/setup.sh", []byte("export FOO=fromsh\n"), 0o644); err != nil {
		t.Fatalf("write setup.sh: %v", err)
	}
	isSecret := true

	// Act
	got, secrets, err := LoadEnvFiles([]EnvFileEntry{{PathTmpl: ".env", SecretOverride: &isSecret}, {PathTmpl: "setup.sh"}}, dir, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	if got["FOO"] != "fromsh" {
		t.Errorf("FOO: got %q, want %q", got["FOO"], "fromsh")
	}
	if secrets["FOO"] {
		t.Error("FOO: expected not secret, final value came from the non-secret .sh entry")
	}
}

func TestLoadEnvFiles_ExplicitSecretTrueOverridesDefault(t *testing.T) {
	// given an env: entry with an explicit secret: true override, when loaded, then vars are marked secret (why: an entry's own secret: setting always wins over the secret: false default)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	isSecret := true

	// Act
	got, secrets, err := LoadEnvFiles([]EnvFileEntry{{PathTmpl: ".env", SecretOverride: &isSecret}}, dir, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	if got["FOO"] != "bar" {
		t.Errorf("FOO: got %q, want %q", got["FOO"], "bar")
	}
	if !secrets["FOO"] {
		t.Error("FOO: expected secret, explicit secret: true should override the default")
	}
}

func TestLoadEnvFiles_ExplicitSecretFalseStaysNotSecret(t *testing.T) {
	// given an env: entry with an explicit secret: false override, when loaded, then vars are not marked secret (why: explicit false is a no-op against the false default, but must still be accepted)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.env", []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	notSecret := false

	// Act
	got, secrets, err := LoadEnvFiles([]EnvFileEntry{{PathTmpl: ".env", SecretOverride: &notSecret}}, dir, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	if got["FOO"] != "bar" {
		t.Errorf("FOO: got %q, want %q", got["FOO"], "bar")
	}
	if secrets["FOO"] {
		t.Error("FOO: expected not secret, explicit secret: false")
	}
}

func TestLoadEnvFiles_LaterEntryPathReferencesEarlierEntryVar(t *testing.T) {
	// given an earlier env: entry sets a var and a later entry's path template references it, when loaded, then the later path resolves against the accumulated vars (why: env: is documented to follow the same top-to-bottom resolution as vars:)
	// Arrange
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/first.env", []byte("STAGE=prod\n"), 0o644); err != nil {
		t.Fatalf("write first.env: %v", err)
	}
	if err := os.WriteFile(dir+"/prod.env", []byte("DEPLOY_TARGET=prod-cluster\n"), 0o644); err != nil {
		t.Fatalf("write prod.env: %v", err)
	}

	// Act
	got, _, err := LoadEnvFiles([]EnvFileEntry{
		{PathTmpl: "first.env"},
		{PathTmpl: "{{.STAGE}}.env"},
	}, dir, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	if got["DEPLOY_TARGET"] != "prod-cluster" {
		t.Errorf("DEPLOY_TARGET: got %q, want %q (STAGE=%q)", got["DEPLOY_TARGET"], "prod-cluster", got["STAGE"])
	}
}
