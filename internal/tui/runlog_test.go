package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"
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
		{"step done", func(l *RunLog) { l.StepDone("t", time.Second) }, "1.0s", ""},
		{"step soft failed", func(l *RunLog) { l.StepSoftFailed("t", time.Second, "go vet", "exit status 1") }, "exit status 1", ""},
		{"step failed", func(l *RunLog) { l.StepFailed("t", time.Second, "go vet", "exit status 1") }, "", "exit status 1"},
		{"task done", func(l *RunLog) { l.TaskDone("t", time.Second, 0) }, "done", ""},
		{"task failed", func(l *RunLog) { l.TaskFailed("t", time.Second, false) }, "", "failed"},
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
		{"variable", func(l *RunLog) { l.Var("deploy", "ENV", "prod", "") }, "deploy » ENV=prod\n"},
		{"variable with a source", func(l *RunLog) { l.Var("deploy", "ENV", "prod", "from the environment") }, "deploy » ENV=prod # from the environment\n"},
		{"skipped variable", func(l *RunLog) { l.VarSkipped("deploy", "REPLICAS") }, "deploy ⊘ REPLICAS\n"},
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

func TestRunLogEndLines(t *testing.T) {
	// given a run log, when a step or task ends, then a step's line carries
	// its mark and time, a task's says "done after" or "failed after", and a
	// soft failure is a "!" with no ✗ (why: a step's end and a task's end
	// must not be confusable)
	tests := []struct {
		name    string
		log     func(l *RunLog)
		wantOut string
		wantErr string
	}{
		{"step done", func(l *RunLog) { l.StepDone("deploy", 1600*time.Millisecond) }, "deploy ✓ 1.6s\n", ""},
		{"step failed", func(l *RunLog) { l.StepFailed("deploy", 500*time.Millisecond, "go test -v ./...", "exit status 1") }, "", "deploy ✗ 0.5s go test -v ./...: exit status 1\n"},
		{"step soft failed", func(l *RunLog) {
			l.StepSoftFailed("deploy", 600*time.Millisecond, "golangci-lint run", "exit status 1")
		}, "deploy ! 0.6s golangci-lint run: exit status 1\n", ""},
		{"step error without a time", func(l *RunLog) { l.StepError("deploy", false, "no task \"nope\"") }, "", "deploy ✗ no task \"nope\"\n"},
		{"task done", func(l *RunLog) { l.TaskDone("setup", 1600*time.Millisecond, 0) }, "setup ✓ done after 1.6s\n", ""},
		{"task done with one soft failure", func(l *RunLog) { l.TaskDone("deploy", 8200*time.Millisecond, 1) }, "deploy ✓ done after 8.2s • 1 soft failure\n", ""},
		{"task done with soft failures", func(l *RunLog) { l.TaskDone("deploy", 8200*time.Millisecond, 2) }, "deploy ✓ done after 8.2s • 2 soft failures\n", ""},
		{"task failed", func(l *RunLog) { l.TaskFailed("test", 1900*time.Millisecond, false) }, "", "test ✗ failed after 1.9s\n"},
		{"task stopped", func(l *RunLog) { l.TaskFailed("test", 1900*time.Millisecond, true) }, "", "test ✗ stopped after 1.9s\n"},
		{"minutes", func(l *RunLog) { l.TaskDone("t", 75*time.Second, 0) }, "t ✓ done after 1m15s\n", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			var out, errOut bytes.Buffer
			log := NewRunLog(&out, &errOut)

			// Act
			test.log(log)

			// Assert
			if out.String() != test.wantOut || errOut.String() != test.wantErr {
				t.Errorf("got out %q err %q, want out %q err %q", out.String(), errOut.String(), test.wantOut, test.wantErr)
			}
		})
	}
}

func TestRunLog_Stopping_NamesTheInnermostTaskAndPrintsOnce(t *testing.T) {
	// given a log inside a called task, when stopping is logged twice, then
	// one yellow line names the innermost task (why: the first CTRL+C is
	// reported once, on the task that was running)
	// Arrange
	var out, errOut bytes.Buffer
	log := NewRunLog(&out, &errOut)
	defer log.Enter("deploy")()
	defer log.Enter("test")()

	// Act
	log.Stopping()
	log.Stopping()

	// Assert
	want := "test ! stopping, ctrl+c again to force\n"
	if out.String() != want || errOut.String() != "" {
		t.Errorf("got out %q err %q, want out %q", out.String(), errOut.String(), want)
	}
}
