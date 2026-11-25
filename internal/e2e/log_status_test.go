//go:build unix

package e2e

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestE2E_Status_RuleShowsUnderTheRunningCommandAndIsGoneAfter(t *testing.T) {
	// given a run watched on a terminal, when a command is running, then the
	// bottom row is the rule with the clocks and a blank row above it, and
	// when the run ends no rule is left (why: the person watching sees the
	// time pass, and the shell prompt must not land under a stale clock)

	// Arrange
	screen, _ := screenOf(60, 24)
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          touch ready
		          while [ ! -f go ]; do :; done
		          echo done
	`
	var during []string

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"t"}, Screen: screen, WhileRunning: func(rows func() []string) {
		waitForFile(t, "ready")
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			during = rows()
			if len(during) > 0 && strings.Contains(during[len(during)-1], "step ") {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err := os.WriteFile("go", nil, 0o644); err != nil {
			t.Error(err)
		}
	}})

	// Assert
	res.OK(t)
	n := len(during)
	if n < 2 || during[n-2] != "" || !strings.HasPrefix(during[n-1], "─") || !strings.Contains(during[n-1], "step ") || !strings.Contains(during[n-1], "total ") {
		t.Errorf("rows while running: %q", during)
	}
	res.NotOut(t, "──")
}

func TestE2E_Status_NeverAppearsInPipedOutput(t *testing.T) {
	// given a run whose output is piped, when it runs, then neither stream
	// holds a rule or an erase code (why: logs and CI must stay plain text)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: echo hi
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	if strings.Contains(res.Raw, "──") || strings.ContainsAny(res.Raw, "\x1b\r") {
		t.Errorf("piped stdout: %q", res.Raw)
	}
}
