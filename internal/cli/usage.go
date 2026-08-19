package cli

import (
	"fmt"
	"io"

	"hobnob/internal/config"
)

// DisplayVersion returns version, or "dev" when version is empty (unset by
// the release build's -ldflags, e.g. a local `go build`/`go run`) — the one
// place that fallback is decided, shared by --version and usage/docs-link
// output so they can't drift out of sync with each other.
func DisplayVersion(version string) string {
	if version == "" {
		return "dev"
	}
	return version
}

func PrintUsage(out io.Writer, version string) {
	displayVersion := DisplayVersion(version)
	docsURL := "https://github.com/JakesMD/hobnob/blob/main/GUIDE.md"
	if version != "" {
		docsURL = "https://github.com/JakesMD/hobnob/blob/" + version + "/GUIDE.md"
	}
	fmt.Fprintf(out, `hobnob %s

Usage:
  hobnob [--file <path>] <task> [--no-input] [KEY=VALUE ...]
  hobnob [--file <path>] --list
  hobnob [--file <path>] --select
  hobnob [--file <path>] --help
  hobnob --version
  hobnob --upgrade

Flags:
  --file <path>   Hobnob file to use instead of auto-discovery
  --list          List all available tasks
  --select        Interactively select a task to run
  --help          Show this help
  --no-input      Skip interactive prompts; fail if a required variable is missing
  --version       Print version and exit
  --upgrade       Upgrade hobnob to the latest release

Docs:
  %s

`, displayVersion, docsURL)
}

func PrintHelp(cfg *config.ConfigFile, scope *Scope, out io.Writer, version string) error {
	PrintUsage(out, version)
	return ListTasks(cfg, scope, out)
}
