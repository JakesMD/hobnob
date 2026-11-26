package e2e

import "testing"

func TestE2E_Say_ShowsTheFilledInMessageAndLogsIt(t *testing.T) {
	// given a say: naming a variable and a secret, when run at a terminal,
	// then the person is shown the message filled in with the secret masked,
	// and the log keeps it (why: the message is for the person, and a secret
	// must not leak through it)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - TOKEN:
		              value: hunter2
		              secret: true
		      - say: "Hi {{.WHO}}, using {{.TOKEN}}"
		      - run: echo after
	`

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"t", "WHO=Ada"}, TTY: true})

	// Assert
	res.OK(t)
	if len(res.Prompts) != 1 || res.Prompts[0].Type != "say" || res.Prompts[0].Info != "Hi Ada, using ****" || res.Prompts[0].Task != "t" {
		t.Errorf("prompts = %+v", res.Prompts)
	}
	res.Out(t, "t » Hi Ada, using ****")
	res.NotOut(t, "hunter2")
	res.Out(t, "t │ after")
}

func TestE2E_Say_WithoutPromptsOnlyLogsTheMessage(t *testing.T) {
	// given a say:, when run with --no-input, then it logs the message and
	// goes on without waiting (why: nobody is there to press Enter in a script)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - say: Hello
		      - run: echo after
	`

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"t", "--no-input"}, TTY: true})

	// Assert
	res.OK(t)
	if len(res.Prompts) != 0 {
		t.Errorf("prompts = %+v, want none", res.Prompts)
	}
	res.Out(t, "t » Hello")
	res.Out(t, "t │ after")
}

func TestE2E_Say_ShowsUnderQuiet(t *testing.T) {
	// given a say:, when run with --quiet, then the message is still logged
	// (why: --quiet trims hobnob's own lines, not what the task says to you)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - say: Hello
	`

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"t", "--quiet", "--no-input"}})

	// Assert
	res.OK(t)
	res.Out(t, "t » Hello")
}

func TestE2E_Say_IfFalseShowsNothing(t *testing.T) {
	// given a say: whose if: is false, when run, then it neither asks nor
	// logs (why: the demo picks its "Correct!" or "Not quite" by if:)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - say: Correct!
		        if: "{{ eq .GUESS \"a\" }}"
		      - say: Not quite
		        if: "{{ ne .GUESS \"a\" }}"
	`

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"t", "GUESS=b"}, TTY: true})

	// Assert
	res.OK(t)
	if len(res.Prompts) != 1 || res.Prompts[0].Info != "Not quite" {
		t.Errorf("prompts = %+v", res.Prompts)
	}
	res.NotOut(t, "Correct!")
}

func TestE2E_Say_RejectsWhatIsNotAMessage(t *testing.T) {
	// given a say: that is a mapping, empty, or carries a key it doesn't
	// take, when loaded, then it fails saying what's wrong (why: a typo
	// shouldn't quietly show nothing)
	tests := []struct {
		name string
		step string
		want string
	}{
		{"given a mapping, then it asks for a message", "- say: {text: hi}", "say: expected a message"},
		{"given an empty say:, then it asks for a message", `- say: ""`, "say: expected a message"},
		{"given into:, then it names the keys say: takes", "- say: hi\n        into: [X]", "say: step doesn't take into:"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			yml := "tasks:\n  t:\n    steps:\n      " + test.step

			// Act
			res := Yml(t, yml, "t")

			// Assert
			res.Fails(t)
			res.Err(t, test.want)
		})
	}
}
