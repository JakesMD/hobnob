package cli

import (
	"fmt"
	"io"

	"hobnob/internal/config"
	"hobnob/internal/scope"
	"hobnob/internal/tui"

	"github.com/charmbracelet/lipgloss"
)

// DisplayVersion returns version, or "dev" when version is empty (unset by
// the release build's -ldflags, e.g. a local `go build`/`go run`) — the one
// place that fallback is decided, shared by --version and usage output so
// they can't drift out of sync with each other.
func DisplayVersion(version string) string {
	if version == "" {
		return "dev"
	}
	return version
}

// GuideURL points at the guide on the docs site.
const GuideURL = docsBase + "guide/"

// ReferenceURL is the reference's equivalent of GuideURL. The guide teaches;
// the reference is what someone reaching for --help mid-task usually wants, so
// usage names both rather than making them click through.
const ReferenceURL = docsBase + "reference/"

const docsBase = "https://jakesmd.github.io/hobnob/docs/"

func PrintUsage(out io.Writer, version string) {
	heading := lipgloss.NewStyle().Bold(true).Render
	grey := tui.SInfo.Render
	fmt.Fprintf(out, "%s %s\n\n", heading("hobnob"), grey(DisplayVersion(version)))

	fmt.Fprintln(out, heading("Usage"))
	fmt.Fprintln(out, "  hobnob [--file <path>] <task> [--no-input] [KEY=VALUE ...]")
	fmt.Fprintln(out, "  hobnob [--file <path>] --list | --select | --help")
	fmt.Fprintln(out, "  hobnob --demo | --version | --upgrade")

	fmt.Fprintf(out, "\n%s\n", heading("Flags"))
	for _, flag := range [][2]string{
		{"--file <path>", "Hobnob file to use instead of auto-discovery"},
		{"--demo", "Run the built-in demo taskfile instead of one of yours"},
		{"--list", "List all available tasks"},
		{"--select", "Interactively select a task to run"},
		{"--help", "Show this help"},
		{"--no-input", "Skip interactive prompts; fail if a required variable is missing"},
		{"--quiet, -q", "Print only each command and each task's end (or set HOBNOB_QUIET=1)"},
		{"--version", "Print version and exit"},
		{"--upgrade", "Upgrade hobnob to the latest release"},
	} {
		fmt.Fprintf(out, "  %-15s %s\n", flag[0], grey(flag[1]))
	}

	fmt.Fprintf(out, "\n%s\n", heading("Docs"))
	fmt.Fprintf(out, "  Guide      %s\n", grey(GuideURL))
	fmt.Fprintf(out, "  Reference  %s\n\n", grey(ReferenceURL))
}

func PrintHelp(cfg *config.ConfigFile, scope *scope.Scope, out io.Writer, version string) error {
	PrintUsage(out, version)
	if rows, _ := buildTaskRows(cfg, scope); len(rows) == 0 {
		return ListTasks(cfg, scope, out)
	}
	fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).Render("Tasks"))
	return listTasks(cfg, scope, out, "  ")
}
