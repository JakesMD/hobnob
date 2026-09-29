package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"hobnob/internal/scope"
	"hobnob/internal/value"
)

func copyVars(src map[string]value.Value) map[string]value.Value {
	out := make(map[string]value.Value, len(src))
	for key, val := range src {
		out[key] = val
	}
	return out
}

func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	f()

	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	return buf.String()
}

func TestDisplayDirPath(t *testing.T) {
	tests := []struct {
		name          string
		dir           string
		invocationDir string
		want          string
	}{
		{
			name:          "given dir is invocation dir, when formatting, then returns ./ (why: dir is the cwd itself, mirror the dir: path style)",
			dir:           "/home/user/project",
			invocationDir: "/home/user/project",
			want:          "./",
		},
		{
			name:          "given dir is subdir of invocation dir, when formatting, then returns relative path with ./ prefix (why: mirror the dir: path style)",
			dir:           "/home/user/project/infra",
			invocationDir: "/home/user/project",
			want:          "./infra",
		},
		{
			name:          "given dir is nested subdir, when formatting, then returns relative path with ./ prefix (why: still inside cwd)",
			dir:           "/home/user/project/infra/staging",
			invocationDir: "/home/user/project",
			want:          "./infra/staging",
		},
		{
			name:          "given dir is outside invocation dir, when formatting, then returns full path (why: relative path with .. is harder to read than absolute)",
			dir:           "/home/user/other",
			invocationDir: "/home/user/project",
			want:          "/home/user/other",
		},
		{
			name:          "given dir is parent of invocation dir, when formatting, then returns full path (why: not within cwd or its subdirs)",
			dir:           "/home/user",
			invocationDir: "/home/user/project",
			want:          "/home/user",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange (test fields are the arrangement)

			// Act
			got := displayDirPath(test.dir, test.invocationDir)

			// Assert
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestExecuteSteps_CtxCancelledBetweenSteps_ReturnsErrInterrupted(t *testing.T) {
	// given ctx already cancelled before a step runs, when executeSteps
	// evaluates the between-step guard, then the error wraps ErrInterrupted
	// (why: must match execRun's wrapping so main.go's errors.Is check catches
	// cancellation that lands between steps, not just mid-command)

	// Arrange
	cfg := makeRunCfg("echo hello", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	err := ExecuteTask(ctx, "t", scope.New(), nil, cfg, true, t.TempDir())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
}
