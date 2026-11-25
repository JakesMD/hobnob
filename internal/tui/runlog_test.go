package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunLogWritesEachKindOfLineToItsStream(t *testing.T) {
	// given a run log over separate buffers, when each kind of line is
	// logged, then only the failure and command stderr reach stderr (why:
	// the stream split of every run line must stay as it was)
	tests := []struct {
		name       string
		log        func(l *RunLog)
		wantStdout string
		wantStderr string
	}{
		{"run", func(l *RunLog) { l.Run("t", "echo hi", "") }, "echo hi", ""},
		{"call cached", func(l *RunLog) { l.CallCached("t", "other", "") }, "other", ""},
		{"task skipped", func(l *RunLog) { l.TaskSkipped("t") }, "skipped", ""},
		{"run skipped", func(l *RunLog) { l.RunSkipped("t") }, "skipped (if: false)", ""},
		{"note", func(l *RunLog) { l.Note("t", "msg") }, "msg", ""},
		{"task done", func(l *RunLog) { l.TaskDone("t") }, "done", ""},
		{"task failed", func(l *RunLog) { l.TaskFailed("t", "boom") }, "", "boom"},
		{"command stdout", func(l *RunLog) { o, _ := l.Output("t"); o.Write([]byte("a\n")) }, "a\n", ""},
		{"command stderr", func(l *RunLog) { _, e := l.Output("t"); e.Write([]byte("b\n")) }, "", "b\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			var out, errOut bytes.Buffer
			log := NewRunLog(&out, &errOut)

			// Act
			tt.log(log)

			// Assert
			if tt.wantStdout == "" && out.Len() != 0 || !strings.Contains(out.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want %q", out.String(), tt.wantStdout)
			}
			if tt.wantStderr == "" && errOut.Len() != 0 || !strings.Contains(errOut.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want %q", errOut.String(), tt.wantStderr)
			}
		})
	}
}
