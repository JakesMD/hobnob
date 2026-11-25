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
	log.Run("t", "echo hi")
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
