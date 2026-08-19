package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"hobnob/internal/config"
	"hobnob/internal/value"
)

func TestExecCall_Soft_DoesNotSwallowInterrupt(t *testing.T) {
	// given a soft: true call step whose child task is interrupted by ctx
	// cancellation, when executed, then the interrupt still propagates (why:
	// soft: true is meant to tolerate ordinary command failures, not mask a
	// user-requested shutdown as false success)

	// Arrange
	cfg := &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{
				{Kind: config.KindCall, CallTarget: "child", Soft: true},
			}},
			"child": {Steps: []config.Step{
				{Kind: config.KindRun, Command: "sleep 5"},
			}},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Act
	err := ExecuteTask(ctx, "t", makeScope(map[string]value.Value{}), cfg, true, t.TempDir())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted to survive soft:true, got: %v", err)
	}
}
