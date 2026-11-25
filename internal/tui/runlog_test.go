package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunLogRoutesLinesToTheirStream(t *testing.T) {
	// given a run log over separate buffers, when a task's lines, output and
	// failure are logged, then run lines and stdout go to stdout and stderr
	// and the failure go to stderr (why: the run log must keep today's
	// stream split)
	// Arrange
	var out, errOut bytes.Buffer
	log := NewRunLog(&out, &errOut)

	// Act
	log.Run("t", "echo hi", "")
	stdout, stderr := log.Output("t")
	stdout.Write([]byte("a\n"))
	stderr.Write([]byte("b\n"))
	log.TaskDone("t")
	log.TaskFailed("t", "boom")

	// Assert
	if !strings.Contains(out.String(), "echo hi") || !strings.Contains(out.String(), "a\n") || !strings.Contains(out.String(), "done") {
		t.Errorf("stdout = %q, want run line, output and done", out.String())
	}
	if !strings.Contains(errOut.String(), "b\n") || !strings.Contains(errOut.String(), "boom") || strings.Contains(errOut.String(), "echo hi") {
		t.Errorf("stderr = %q, want only command stderr and failure", errOut.String())
	}
}
