package app

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/tui"
)

// demoTaskfile is the taskfile hobnob runs when auto-discovery finds no
// hobnob.yml anywhere — so `hobnob`, straight after install and before the
// user has written anything, opens the picker with something in it instead of
// printing usage at them.
//
//go:embed demo.yml
var demoTaskfile []byte

// demoFileName is what the built-in taskfile calls itself in error messages.
// It's deliberately not a real path: nothing on disk backs it, and a reader
// who sees it in an error should be able to tell that immediately.
const demoFileName = "<built-in demo>"

// loadDemoConfig parses the embedded demo taskfile as though it had been
// discovered in invDir, so relative paths and HOBNOB_FILE_DIR behave the way
// they would for a real file sitting in the directory the user ran from.
func loadDemoConfig(ctx context.Context, cliVars map[string]string, invDir string) (*config.ConfigFile, *cli.Scope, error) {
	cfg, err := config.ParseConfigData(demoTaskfile, filepath.Join(invDir, demoFileName), invDir)
	if err != nil {
		return nil, nil, err
	}
	scope, err := buildScopeFor(ctx, cfg, cliVars, invDir)
	if err != nil {
		return nil, nil, err
	}
	return cfg, scope, nil
}

// announceDemo marks the output as the built-in demo's rather than anything
// in the user's own directory, and points at the guide — --demo is most of the
// time the first hobnob command someone runs. Goes to stderr so it can't
// contaminate a piped task's output.
func announceDemo() {
	fmt.Fprintln(os.Stderr, tui.SInfo.Render("Running the built-in demo. Write your own: "+cli.GuideURL))
}
