//go:build linux || darwin

package runner

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"hobnob/internal/scope"
	"hobnob/internal/tui"
)

// terminalStdin makes os.Stdin a terminal for the test and returns the other
// end, where a person's keys would be typed.
func terminalStdin(t *testing.T) (keys *os.File) {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Skipf("no PTY: %v", err)
	}
	orig := os.Stdin
	os.Stdin = slave
	t.Cleanup(func() { os.Stdin = orig; master.Close(); slave.Close() })
	return master
}

func lflag(t *testing.T) uint64 {
	t.Helper()
	termios, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), getTermios)
	if err != nil {
		t.Fatal(err)
	}
	return uint64(termios.Lflag)
}

func TestExecRun_TerminalStdin_ForwardsKeysToTheCommand(t *testing.T) {
	// given hobnob's stdin is a terminal, when keys are typed during a step
	// that reads, then the command gets them and a terminal that cooks input
	// again is left behind (why: read prompts must work, and the terminal is
	// raw only while a command runs)

	// Arrange
	keys := terminalStdin(t)
	dir := t.TempDir()
	cfg := makeRunCfg(t, "[ -t 0 ] && read n && echo \"$n\" > answer", nil)
	want := lflag(t)
	done := make(chan error, 1)

	// Act
	go func() { done <- ExecuteTask(context.Background(), "t", scope.New(), nil, cfg, true, dir, tui.NewRunLog(os.Stdout, os.Stderr)) }()
	time.Sleep(200 * time.Millisecond)
	keys.WriteString("bob\n")
	err := <-done

	// Assert
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "answer"))
	if strings.TrimSpace(string(got)) != "bob" {
		t.Errorf("command read %q, want bob", got)
	}
	if lflag(t) != want {
		t.Errorf("terminal not restored: lflag %x, want %x", lflag(t), want)
	}
}

func TestExecRun_TerminalStdin_CtrlCReachesHobnobNotTheCommand(t *testing.T) {
	// given hobnob's stdin is a terminal, when CTRL+C is typed during a step
	// that traps SIGINT, then hobnob gets a SIGINT and the command doesn't
	// (why: the first CTRL+C must stop and the second force even a command
	// that traps SIGINT)

	// Arrange
	keys := terminalStdin(t)
	dir := t.TempDir()
	cfg := makeRunCfg(t, "trap 'touch gotint' INT; read n", nil)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	defer signal.Stop(sigs)
	done := make(chan error, 1)

	// Act
	go func() { done <- ExecuteTask(context.Background(), "t", scope.New(), nil, cfg, true, dir, tui.NewRunLog(os.Stdout, os.Stderr)) }()
	time.Sleep(200 * time.Millisecond)
	keys.Write([]byte{3})
	var got os.Signal
	select {
	case got = <-sigs:
	case <-time.After(2 * time.Second):
	}
	keys.WriteString("\n")
	<-done

	// Assert
	if got != syscall.SIGINT {
		t.Error("hobnob got no SIGINT for the CTRL+C key")
	}
	if _, err := os.Stat(filepath.Join(dir, "gotint")); err == nil {
		t.Error("the command received the SIGINT too")
	}
}
