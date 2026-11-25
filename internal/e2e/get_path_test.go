package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pathFiles is a taskfile plus a few real files for a type: file|dir entry
// to point at.
func pathFiles(yml string) Files {
	return Files{
		"hobnob.yml":    yml,
		"sub/f.txt":     "x",
		"sub/g.md":      "x",
		"other/h.txt":   "x",
		"docs/guide.md": "x",
	}
}

func TestE2E_GetPath_PresetRelativeBecomesAbsolute(t *testing.T) {
	// given a vars: value written relative for a type: file entry, when the
	// task runs with dir:, then it is stored absolute against that dir: (why:
	// a path written in the taskfile belongs to the task's working directory)
	// Arrange
	yml := `
		vars:
		  - CONF: f.txt
		tasks:
		  t:
		    dir: sub
		    steps:
		      - get:
		          - CONF:
		              type: file
		      - run: echo conf={{.CONF}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t"}})

	// Assert
	res.OK(t)
	res.Lines(t, "conf="+filepath.Join(res.Dir, "sub", "f.txt"))
}

func TestE2E_GetPath_CLIArgResolvesAgainstInvocationDir(t *testing.T) {
	// given a relative CLI argument for a type: file entry, when the task has
	// a different dir:, then it resolves against where hobnob was run (why:
	// what a person types at a shell is relative to their shell)
	// Arrange
	yml := `
		tasks:
		  t:
		    dir: other
		    steps:
		      - get:
		          - CONF:
		              type: file
		      - run: echo conf={{.CONF}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Cwd: "sub", Args: []string{"t", "CONF=f.txt"}})

	// Assert
	res.OK(t)
	res.Lines(t, "conf="+filepath.Join(res.Dir, "sub", "f.txt"))
}

func TestE2E_GetPath_ShellEnvResolvesAgainstInvocationDir(t *testing.T) {
	// given a relative value from the shell's environment, when the task has
	// a different dir:, then it resolves against where hobnob was run (why:
	// the shell environment is the user's, like a CLI argument)
	// Arrange
	yml := `
		tasks:
		  t:
		    dir: other
		    steps:
		      - get:
		          - CONF:
		              type: file
		      - run: echo conf={{.CONF}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Cwd: "sub", Env: map[string]string{"CONF": "f.txt"}, Args: []string{"t"}})

	// Assert
	res.OK(t)
	res.Lines(t, "conf="+filepath.Join(res.Dir, "sub", "f.txt"))
}

func TestE2E_GetPath_PathIsCleanedAndHomeExpanded(t *testing.T) {
	// given a messy path and a ~ path, when the task runs, then both are
	// cleaned and ~ is the home directory (why: one tidy form for later steps)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		          - HOMEDIR:
		              type: dir
		      - run: echo conf={{.CONF}} home={{.HOMEDIR}}
	`
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "CONF=./sub//../sub/f.txt", "HOMEDIR=~/"}})

	// Assert
	res.OK(t)
	res.Lines(t, "conf="+filepath.Join(res.Dir, "sub", "f.txt")+" home="+filepath.Clean(home))
}

func TestE2E_GetPath_SymlinkKeptAsGiven(t *testing.T) {
	// given a symlink to a file, when it is the value of a type: file entry,
	// then the stored path is the link itself (why: the link name may matter
	// to the tool that gets it; only its kind is judged by the target)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: ln -s sub/f.txt link.txt
		      - get:
		          - CONF:
		              type: file
		      - run: echo conf={{.CONF}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "CONF=link.txt"}})

	// Assert
	res.OK(t)
	res.Out(t, "conf="+filepath.Join(res.Dir, "link.txt"))
}

func TestE2E_GetPath_SymlinkToDirectoryFailsAsAFile(t *testing.T) {
	// given a symlink to a directory, when it is the value of a type: file
	// entry, then it fails as a directory (why: a link's kind is its target's)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: ln -s sub link
		      - get:
		          - CONF:
		              type: file
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "CONF=link"}})

	// Assert
	res.Fails(t)
	res.Err(t, "get CONF:", "link is a directory; CONF needs a file")
}

func TestE2E_GetPath_FilePresetThatIsADirectoryFails(t *testing.T) {
	// given a type: file entry preset to a directory, when the task runs,
	// then it fails naming the variable (why: the kind check guards the
	// command from the wrong sort of path)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "CONF=sub"}})

	// Assert
	res.Fails(t)
	res.Err(t, "get CONF: "+filepath.Join(res.Dir, "sub")+" is a directory; CONF needs a file")
}

func TestE2E_GetPath_DirPresetThatIsAFileFails(t *testing.T) {
	// given a type: dir entry preset to a file, when the task runs, then it
	// fails naming the variable (why: the kind check works both ways)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - OUT:
		              type: dir
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "OUT=sub/f.txt"}})

	// Assert
	res.Fails(t)
	res.Err(t, "get OUT:", "is a file; OUT needs a directory")
}

func TestE2E_GetPath_MissingPresetFails(t *testing.T) {
	// given a type: file entry preset to a missing path, when the task runs,
	// then it fails saying it doesn't exist (why: pickers name existing
	// paths only)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "CONF=nope.yml"}})

	// Assert
	res.Fails(t)
	res.Err(t, "get CONF: "+filepath.Join(res.Dir, "nope.yml")+" doesn't exist")
}

func TestE2E_GetPath_MatchMissFails(t *testing.T) {
	// given a type: file entry whose preset doesn't match match:, when the
	// task runs, then it fails naming the patterns (why: match: applies to
	// values nobody picked too)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              match: ["*.yml", "*.yaml"]
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "CONF=sub/f.txt"}})

	// Assert
	res.Fails(t)
	res.Err(t, "get CONF:", "f.txt doesn't match *.yml, *.yaml")
}

func TestE2E_GetPath_EachMultiItemMustMatch(t *testing.T) {
	// given a multi: type: file preset where one item misses match:, when
	// the task runs, then it fails naming that item (why: each item is
	// judged on its own, not the list as a whole)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - SRCS:
		              type: file
		              multi: true
		              match: "*.txt"
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", `SRCS=["sub/g.md","sub/f.txt"]`}})

	// Assert
	res.Fails(t)
	res.Err(t, "get SRCS:", "g.md doesn't match *.txt")
}

func TestE2E_GetPath_MultiItemsBecomeAbsoluteAndCheckSeesTheList(t *testing.T) {
	// given a multi: type: file preset with a check: on the list, when the
	// task runs, then every item is absolute and check: saw all of them (why:
	// check: validates the whole answer)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - SRCS:
		              type: file
		              multi: true
		              check: "echo '{{.SRCS}}' | grep f.txt | grep -q h.txt"
		      - loop: .SRCS
		        steps:
		          - run: echo src={{.ITEM}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", `SRCS=["sub/f.txt","other/h.txt"]`}})

	// Assert
	res.OK(t)
	res.Lines(t, "src="+filepath.Join(res.Dir, "sub", "f.txt"), "src="+filepath.Join(res.Dir, "other", "h.txt"))
}

func TestE2E_GetPath_CheckRunsLastAndFails(t *testing.T) {
	// given a path that passes every path check but fails check:, when the
	// task runs, then it fails with the check (why: check: is the last word)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - SRCS:
		              type: file
		              multi: true
		              check: "echo '{{.SRCS}}' | grep f.txt | grep -q h.txt"
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", `SRCS=["sub/g.md","sub/f.txt"]`}})

	// Assert
	res.Fails(t)
	res.Err(t, "get SRCS:", "validation failed")
}

func TestE2E_GetPath_EmptyPresetOptionalIsEmptyAndSkipsCheck(t *testing.T) {
	// given an optional type: file entry preset empty with a check: that
	// would fail, when the task runs, then it is empty and check: is skipped
	// (why: empty is no answer, never the working directory)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              optional: true
		              check: false
		      - run: echo conf=[{{.CONF}}]
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "CONF="}})

	// Assert
	res.OK(t)
	res.Lines(t, "conf=[]")
}

func TestE2E_GetPath_EmptyPresetNotOptionalFails(t *testing.T) {
	// given a type: dir entry preset empty and not optional, when the task
	// runs, then it fails naming the variable (why: empty must never turn
	// into the working directory)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - OUT:
		              type: dir
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "OUT="}})

	// Assert
	res.Fails(t)
	res.Err(t, "get OUT: no directory given")
}

func TestE2E_GetPath_EmptyMultiPresetNotOptionalFails(t *testing.T) {
	// given a multi: path entry preset to [], when the task runs without
	// optional:, then it fails naming the variable (why: an empty list is no
	// answer either)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - SRCS:
		              type: file
		              multi: true
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "SRCS=[]"}})

	// Assert
	res.Fails(t)
	res.Err(t, "get SRCS: no file given")
}

func TestE2E_GetPath_DirectoryDefaultIsNoAnswerWithPromptsOff(t *testing.T) {
	// given a type: file entry whose default: is a directory, when run
	// --no-input, then it fails naming the variable (why: a directory only
	// says where browsing would start)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              default: sub/
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "--no-input"}})

	// Assert
	res.Fails(t)
	res.Err(t, "CONF requires input")
}

func TestE2E_GetPath_DirectoryDefaultOptionalIsEmptyWithPromptsOff(t *testing.T) {
	// given an optional type: file entry whose default: is a directory, when
	// run --no-input, then the variable is empty (why: no default means an
	// optional entry is left empty)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              optional: true
		              default: sub/
		      - run: echo conf=[{{.CONF}}]
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "--no-input"}})

	// Assert
	res.OK(t)
	res.Lines(t, "conf=[]")
}

func TestE2E_GetPath_MultiDirectoryDefaultIsNoAnswerWithPromptsOff(t *testing.T) {
	// given a multi: type: file entry whose default: is a directory, when run
	// --no-input, then it fails like the single entry does (why: a directory
	// only says where browsing would start, however many files are wanted)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              multi: true
		              default: sub/
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "--no-input"}})

	// Assert
	res.Fails(t)
	res.Err(t, "CONF requires input", "its default is a directory")
}

func TestE2E_GetPath_MultiDirectoryDefaultOptionalIsEmptyWithPromptsOff(t *testing.T) {
	// given an optional multi: type: file entry whose default: is a
	// directory, when run --no-input, then the variable is empty (why: no
	// default means an optional entry is left empty)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              multi: true
		              optional: true
		              default: sub/
		      - run: echo conf={{.CONF}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "--no-input"}})

	// Assert
	res.OK(t)
	res.Lines(t, "conf=[]")
}

func TestE2E_GetPath_DirDefaultIsTheAnswerWithPromptsOff(t *testing.T) {
	// given a type: dir entry with a directory default:, when run --no-input,
	// then the default is the answer, stored absolute (why: prompts off takes
	// the defaults)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - OUT:
		              type: dir
		              default: build/../sub/
		      - run: echo out={{.OUT}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "--no-input"}})

	// Assert
	res.OK(t)
	res.Lines(t, "out="+filepath.Join(res.Dir, "sub"))
}

func TestE2E_GetPath_BadDefaultFailsWithPromptsOff(t *testing.T) {
	// given a type: dir entry whose default: is missing, when run
	// --no-input, then it fails saying it doesn't exist (why: a bad default
	// must not slip through unchecked)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - OUT:
		              type: dir
		              default: nowhere
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "--no-input"}})

	// Assert
	res.Fails(t)
	res.Err(t, "get OUT:", "nowhere doesn't exist")
}

func TestE2E_GetPath_OptionalWithDefaultUsesDefaultWithPromptsOff(t *testing.T) {
	// given an optional type: dir entry with a default:, when run
	// --no-input, then the default is used (why: optional means skippable,
	// not "ignore the default")
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - OUT:
		              type: dir
		              optional: true
		              default: sub
		      - run: echo out={{.OUT}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t", "--no-input"}})

	// Assert
	res.OK(t)
	res.Lines(t, "out="+filepath.Join(res.Dir, "sub"))
}

func TestE2E_GetPath_PickerAnswerIsStoredAbsolute(t *testing.T) {
	// given a type: file entry with prompts on, when the picker answers, then
	// the picker was opened with its type and the answer is stored
	// absolute (why: a picked path must read the same as a preset one)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              match: "*.txt"
		      - run: echo conf={{.CONF}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), TTY: true, Cwd: "sub", Args: []string{"t"},
		Answers: map[string][]string{"CONF": {"f.txt"}}})

	// Assert
	res.OK(t)
	res.Lines(t, "conf="+filepath.Join(res.Dir, "sub", "f.txt"))
	res.Prompted(t, "CONF")
	if got := res.Prompts[0]; got.Type != "file" {
		t.Errorf("picker opened with Type %q, want file", got.Type)
	}
}

func TestE2E_GetPath_FailingCheckReopensThePicker(t *testing.T) {
	// given a type: file entry with a check: the first pick fails, when the
	// second pick passes, then the picker was opened twice and the second
	// pick is stored (why: check: re-opens a picker the way it re-opens a menu)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              check: "case {{.CONF}} in *.md) true;; *) false;; esac"
		      - run: echo conf={{.CONF}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), TTY: true, Args: []string{"t"},
		Answers: map[string][]string{"CONF": {"sub/f.txt", "sub/g.md"}}})

	// Assert
	res.OK(t)
	res.Prompted(t, "CONF", "CONF")
	res.Lines(t, "conf="+filepath.Join(res.Dir, "sub", "g.md"))
}

func TestE2E_GetPath_SecretPickerIsMaskedInCommands(t *testing.T) {
	// given a secret: type: file entry picked at a prompt, when the path is
	// used in a command, then it is masked (why: secret: applies to a picked
	// path as to any value)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              secret: true
		      - run: ": {{.CONF}}"
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), TTY: true, Args: []string{"t"},
		Answers: map[string][]string{"CONF": {"sub/f.txt"}}})

	// Assert
	res.OK(t)
	res.Masked(t, filepath.Join(res.Dir, "sub", "f.txt"))
}

func TestE2E_GetPath_OptionsWithTypeFileRejectedAtLoad(t *testing.T) {
	// given a get: entry with options: and type: file, when loaded, then it
	// fails with the hint (why: a menu of paths is a plain menu)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONFIG:
		              type: file
		              options: [a, b]
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t"}})

	// Assert
	res.Fails(t)
	res.Err(t, `get entry "CONFIG": options: can't be used with type: file; leave out type: for a menu of paths`)
}

func TestE2E_GetPath_UnknownTypeRejectedAtLoad(t *testing.T) {
	// given a get: entry with type: folder, when loaded, then it fails naming
	// the entry and the three values (why: a typo must not silently be text)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONFIG:
		              type: folder
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), Args: []string{"t"}})

	// Assert
	res.Fails(t)
	res.Err(t, `get entry "CONFIG"`, "text, file or dir", `"folder"`)
}

func TestE2E_GetPath_PickerGetsMatchAsPredicates(t *testing.T) {
	// given a type: file entry whose match: has a plain and a "!" pattern, when the picker opens, then its predicates include the matching file, drop the non-matching one and hide the "!" directory
	// (why: the picker never sees match: itself, only these two predicates)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              match: ["*.txt", "!other"]
		      - run: echo conf={{.CONF}}
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), TTY: true, Args: []string{"t"}, Probe: []string{"sub/f.txt", "sub/g.md", "other"},
		Answers: map[string][]string{"CONF": {"sub/f.txt"}}})

	// Assert
	res.OK(t)
	res.Prompted(t, "CONF")
	got := res.Prompts[0]
	if !got.Included["sub/f.txt"] || got.Included["sub/g.md"] {
		t.Errorf("Included = %v, want f.txt true and g.md false", got.Included)
	}
	if !got.Excluded["other"] || got.Excluded["sub/f.txt"] {
		t.Errorf("Excluded = %v, want other true and f.txt false", got.Excluded)
	}
}

func TestE2E_GetPath_ErroringCheckReopensThePickerShowingItsError(t *testing.T) {
	// given a type: file entry whose check: cannot be evaluated, when a path is picked, then the picker reopens with the evaluation error on its error line
	// (why: a broken check must explain itself instead of looping silently)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CONF:
		              type: file
		              optional: true
		              check: "{{ .CONF | keys }}"
	`

	// Act
	res := Run(t, Case{Files: pathFiles(yml), TTY: true, Args: []string{"t"},
		Answers: map[string][]string{"CONF": {"sub/f.txt", ""}}})

	// Assert
	if len(res.Prompts) < 2 || !strings.HasPrefix(res.Prompts[1].Error, "check failed:") {
		t.Errorf("prompts %+v, want the second picker opened with a check failed error", res.Prompts)
	}
}
