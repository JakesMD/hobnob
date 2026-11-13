package cli

import (
	"fmt"
	"io"

	"hobnob/internal/config"
	"hobnob/internal/scope"
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
	displayVersion := DisplayVersion(version)
	fmt.Fprintf(out, `hobnob %s

Usage:
  hobnob [--file <path>] <task> [--no-input] [KEY=VALUE ...]
  hobnob [--file <path>] --list
  hobnob [--file <path>] --select
  hobnob [--file <path>] --help
  hobnob --demo
  hobnob --version
  hobnob --upgrade

Flags:
  --file <path>   Hobnob file to use instead of auto-discovery
  --demo          Run the built-in demo taskfile instead of one of yours
  --list          List all available tasks
  --select        Interactively select a task to run
  --help          Show this help
  --no-input      Skip interactive prompts; fail if a required variable is missing
  --version       Print version and exit
  --upgrade       Upgrade hobnob to the latest release

Docs:
  Guide      %s
  Reference  %s

`, displayVersion, GuideURL, ReferenceURL)
}

func PrintHelp(cfg *config.ConfigFile, scope *scope.Scope, out io.Writer, version string) error {
	PrintUsage(out, version)
	return ListTasks(cfg, scope, out)
}
