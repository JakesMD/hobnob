package hobnob_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const body = "export PATH=\"$HOME/.local/bin:$PATH\"\neval \"$(hobnob completion bash)\""

// writeBlock runs install.sh's _write_hobnob_block against an rc file
// holding before, and returns the rc's contents afterwards plus stderr.
func writeBlock(t *testing.T, before string) (after, stderr string) {
	t.Helper()
	rc := filepath.Join(t.TempDir(), "rc")
	if err := os.WriteFile(rc, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", `source ./install.sh && _write_hobnob_block "$1" "$2"`, "_", rc, body)
	cmd.Env = append(os.Environ(), "HOBNOB_INSTALL_SOURCE_ONLY=1")
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("install.sh failed: %v\n%s", err, errBuf.String())
	}
	got, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	return string(got), errBuf.String()
}

func assertRC(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rc mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

const block = "### hobnob ###\n" + body + "\n### end hobnob ###\n"

func TestInstall_RC_AppendsBlockAfterUserLines(t *testing.T) {
	// given an rc with no hobnob block, when the installer runs, then the
	// block is appended after a blank line (why: first install must not
	// disturb what the user already has)

	// Arrange
	before := "alias a=1\n"

	// Act
	got, _ := writeBlock(t, before)

	// Assert
	assertRC(t, got, "alias a=1\n\n"+block)
}

func TestInstall_RC_ReplacesExistingBlockInPlace(t *testing.T) {
	// given an rc with an outdated block between user lines, when the
	// installer runs, then only the block's contents change (why: re-runs
	// update the setup without duplicating it or moving user lines)

	// Arrange
	before := "alias a=1\n### hobnob ###\nold line\n### end hobnob ###\nalias b=2\n"

	// Act
	got, _ := writeBlock(t, before)

	// Assert
	assertRC(t, got, "alias a=1\n"+block+"alias b=2\n")
}

func TestInstall_RC_RerunLeavesFileUnchanged(t *testing.T) {
	// given an rc already holding the current block, when the installer
	// runs, then the file is byte-for-byte the same (why: re-runs are safe)

	// Arrange
	before := "alias a=1\n\n" + block

	// Act
	got, _ := writeBlock(t, before)

	// Assert
	assertRC(t, got, before)
}

func TestInstall_RC_UnpairedMarkersLeaveFileUntouched(t *testing.T) {
	// given an rc whose markers are unpaired or out of order, when the
	// installer runs, then the file is untouched and the user is warned
	// (why: guessing where the block ends could delete the user's lines)
	cases := map[string]string{
		"begin without end": "x\n### hobnob ###\nmine\nkeep\n",
		"end before begin":  "keep1\n### end hobnob ###\nkeep2\n### hobnob ###\nkeep3\n",
		"two blocks":        block + "mine\n" + block,
	}
	for name, before := range cases {
		t.Run(name, func(t *testing.T) {
			// Act
			got, stderr := writeBlock(t, before)

			// Assert
			assertRC(t, got, before)
			if !strings.Contains(stderr, "left it unchanged") {
				t.Errorf("want a warning on stderr, got %q", stderr)
			}
		})
	}
}

func TestInstall_RC_RemovesPreMarkerBlocks(t *testing.T) {
	// given an rc set up by an installer from before the markers, when the
	// installer runs, then the old lines are replaced by the block (why:
	// upgrading must not set PATH and completion up twice)
	cases := map[string]string{
		"bash": "alias a=1\n\n# hobnob: path+completion\nexport PATH=\"$HOME/.local/bin:$PATH\"\neval \"$(hobnob completion bash)\"\n",
		"zsh":  "alias a=1\n\n# hobnob: path+completion\nexport PATH=\"$HOME/.local/bin:$PATH\"\ntype compdef &>/dev/null || { autoload -Uz compinit && compinit; }; eval \"$(hobnob completion zsh)\"\n",
		"fish": "alias a=1\n\n# hobnob\nfish_add_path \"$HOME/.local/bin\"\n\n# hobnob completion\nhobnob completion fish | source\n",
	}
	for name, before := range cases {
		t.Run(name, func(t *testing.T) {
			// Act
			got, _ := writeBlock(t, before)

			// Assert
			assertRC(t, got, "alias a=1\n\n"+block)
		})
	}
}

func TestInstall_RC_RecognisesCRLFMarkers(t *testing.T) {
	// given an rc with Windows line endings on its markers, when the
	// installer runs, then the existing block is replaced rather than a
	// second one appended (why: CRLF files are still the user's rc)

	// Arrange
	before := "### hobnob ###\r\nold\r\n### end hobnob ###\r\n"

	// Act
	got, _ := writeBlock(t, before)

	// Assert
	assertRC(t, got, block)
}
