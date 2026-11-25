package app

import (
	"strings"
	"testing"
)

func TestWebFlagFailsOnWindows(t *testing.T) {
	// given --web with a task, when checked on Windows, then it fails saying
	// it is not yet supported there, and elsewhere it passes (why: browser
	// mode has no Windows support yet)
	// Arrange
	args := []string{"deploy"}

	// Act
	windows := checkWebFlag("windows", args)
	linux := checkWebFlag("linux", args)

	// Assert
	if windows == nil || !strings.Contains(windows.Error(), "not yet supported on Windows") {
		t.Errorf("windows error = %v", windows)
	}
	if linux != nil {
		t.Errorf("linux error = %v", linux)
	}
}
