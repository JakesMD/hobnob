package cli

import (
	"strings"
	"testing"
)

func TestCompletionScript(t *testing.T) {
	tests := []struct {
		name      string
		shell     string
		wantErr   bool
		wantParts []string
	}{
		{
			name:      "given zsh, when completion script generated, then contains compdef and _hobnob with compinit guard (why: zsh completion requires compdef directive, function, and compinit guard for shells that defer compinit)",
			shell:     "zsh",
			wantParts: []string{"_hobnob()", "hobnob --list", "compadd", "type compdef &>/dev/null", "compdef _hobnob hobnob", "--file", "--no-input", "--version", "--upgrade"},
		},
		{
			name:      "given bash, when completion script generated, then contains complete directive and function (why: bash completion uses complete builtin)",
			shell:     "bash",
			wantParts: []string{"_hobnob_completion()", "hobnob --list", "compgen", "complete -F _hobnob_completion hobnob", "--file", "--no-input", "--version", "--upgrade"},
		},
		{
			name:      "given fish, when completion script generated, then contains complete command (why: fish completion uses complete builtin with -c flag)",
			shell:     "fish",
			wantParts: []string{"__fish_hobnob_tasks", "hobnob --list", "complete -c hobnob", "__fish_hobnob_no_task_given", "commandline -opc", "--file", "-l no-input", "-l version", "-l upgrade"},
		},
		{
			name:    "given unknown shell, when completion script generated, then returns error (why: unsupported shells must fail fast)",
			shell:   "powershell",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test.shell is the arrangement)

			// Act
			got, err := CompletionScript(test.shell)

			// Assert
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, part := range test.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("script missing %q\ngot:\n%s", part, got)
				}
			}
		})
	}
}
