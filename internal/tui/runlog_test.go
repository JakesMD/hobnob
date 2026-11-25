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
		{"run", func(l *RunLog) { l.Run("t", "echo hi") }, "echo hi", ""},
		{"call", func(l *RunLog) { l.Call("t", "other") }, "other", ""},
		{"done earlier", func(l *RunLog) { l.DoneEarlier("t") }, "earlier", ""},
		{"task skipped", func(l *RunLog) { l.TaskSkipped("t") }, "skipped", ""},
		{"run skipped", func(l *RunLog) { l.RunSkipped("t", "go vet") }, "go vet", ""},
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

func TestRunLogLines(t *testing.T) {
	// given a run log, when a run, call, skip, once: call or output line is
	// logged, then each reads "task SEP text" with no step word, bracket or
	// padding, and a multi-line command's continuation lines sit under its
	// first (why: every line of the log names its task and says what it is)
	tests := []struct {
		name string
		log  func(l *RunLog)
		want string
	}{
		{"run", func(l *RunLog) { l.Run("deploy", "go build") }, "deploy $ go build\n"},
		{"run with notes", func(l *RunLog) { l.Run("deploy", "rsync .", "in ./deploy/", "output hidden") }, "deploy $ rsync . # in ./deploy/ # output hidden\n"},
		{"multi-line run", func(l *RunLog) { l.Run("deploy", "a \\\n  b\nc") }, "deploy $ a \\\n           b\n         c\n"},
		{"call", func(l *RunLog) { l.Call("deploy", "setup") }, "deploy → setup\n"},
		{"step skip", func(l *RunLog) { l.RunSkipped("deploy", "go vet ./...") }, "deploy ⊘ go vet ./...\n"},
		{"task skip", func(l *RunLog) { l.TaskSkipped("lint") }, "lint ⊘ skipped\n"},
		{"done earlier", func(l *RunLog) { l.DoneEarlier("setup") }, "setup ✓ done earlier\n"},
		{"output", func(l *RunLog) {
			o, _ := l.Output("deploy")
			o.Write([]byte("hi\n"))
		}, "deploy │ hi\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			var out, errOut bytes.Buffer
			log := NewRunLog(&out, &errOut)

			// Act
			test.log(log)

			// Assert
			if out.String() != test.want {
				t.Errorf("got %q, want %q", out.String(), test.want)
			}
		})
	}
}
