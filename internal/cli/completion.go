package cli

import (
	_ "embed"
	"fmt"
)

//go:embed completions/hobnob.zsh
var zshCompletion string

//go:embed completions/hobnob.bash
var bashCompletion string

//go:embed completions/hobnob.fish
var fishCompletion string

func CompletionScript(shell string) (string, error) {
	switch shell {
	case "zsh":
		return zshCompletion, nil
	case "bash":
		return bashCompletion, nil
	case "fish":
		return fishCompletion, nil
	default:
		return "", fmt.Errorf("unknown shell %q: supported shells are bash, zsh, fish", shell)
	}
}
