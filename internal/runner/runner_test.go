package runner

import (
	"context"
	"errors"
	"os"
	"testing"

	"hobnob/internal/runevents"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
)

func TestDisplayDirPath(t *testing.T) {
	// given dirs inside and outside the invocation dir, when formatted, then
	// inside ones read as ./ paths and outside ones in full (why: mirror the
	// dir: path style where it reads well)
	// Arrange
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
	cfg := makeRunCfg(t, "echo hello", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Act
	err := ExecuteTask(ctx, "t", scope.New(), nil, cfg, true, t.TempDir(), runevents.New(), tui.NewRunLog(os.Stdout, os.Stderr), TerminalPrompts())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
}
