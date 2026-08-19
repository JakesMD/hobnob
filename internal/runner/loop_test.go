package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"hobnob/internal/config"
	"hobnob/internal/value"
)

func TestExecFor_CtxCancelledMidLoop_ReturnsErrInterrupted(t *testing.T) {
	// given a loop: step with multiple slow iterations, when ctx is cancelled
	// partway through, then the loop stops early and the error wraps
	// ErrInterrupted (why: executeSteps' between-step guard must be reached
	// again on each loop iteration, not just checked once before the loop
	// starts)

	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{
					Kind:    config.KindFor,
					ForList: []string{"a", "b", "c", "d", "e"},
					ForSteps: []config.Step{
						{Kind: config.KindRun, Command: "sleep 0.1"},
					},
				},
			}},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Act
	err := ExecuteTask(ctx, "t", makeScope(map[string]value.Value{}), cfg, true, t.TempDir())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
}
