package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"hobnob/internal/config"
)

func TestPrintUsage(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		wantParts  []string
		wantAbsent []string
	}{
		{
			name:    "given release version, when usage printed, then shows version and versioned docs URL (why: users should see docs matching their binary)",
			version: "v1.2.3",
			wantParts: []string{
				"hobnob v1.2.3",
				"--version",
				"https://github.com/JakesMD/hobnob/blob/v1.2.3/GUIDE.md",
			},
			wantAbsent: []string{"hobnob dev"},
		},
		{
			name:    "given empty version (dev build), when usage printed, then shows dev and main branch docs URL (why: dev builds fall back to main branch docs)",
			version: "",
			wantParts: []string{
				"hobnob dev",
				"--version",
				"https://github.com/JakesMD/hobnob/blob/main/GUIDE.md",
			},
			wantAbsent: []string{"hobnob v"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			var buf bytes.Buffer

			// Act
			PrintUsage(&buf, test.version)

			// Assert
			output := buf.String()
			for _, want := range test.wantParts {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, output)
				}
			}
			for _, absent := range test.wantAbsent {
				if strings.Contains(output, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, output)
				}
			}
		})
	}
}

func TestPrintHelp(t *testing.T) {
	// given config with task info, when PrintHelp called, then output contains all flag docs and task descriptions (why: help output is the primary user reference)
	// Arrange
	cfg, err := config.ParseConfig("testdata/task_info.yml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	scope, err := BuildScope(context.Background(), cfg.Vars, nil, nil, "/tmp/taskfile", "/tmp/invocation")
	if err != nil {
		t.Fatalf("scope error: %v", err)
	}
	var buf bytes.Buffer

	// Act
	if err := PrintHelp(cfg, scope, &buf, "v2.0.0"); err != nil {
		t.Fatalf("PrintHelp error: %v", err)
	}

	// Assert
	output := buf.String()
	for _, want := range []string{
		"--file",
		"--list",
		"--help",
		"--no-input",
		"--version",
		"hobnob v2.0.0",
		"https://github.com/JakesMD/hobnob/blob/v2.0.0/GUIDE.md",
		"adopt",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}
}
