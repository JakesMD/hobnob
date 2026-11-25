package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"hobnob/internal/cli"
	"hobnob/internal/tui"
)

// selfUpgrade runs the latest release's install.sh, so an upgrade also gets
// the installer's PATH and completion setup, not just a new binary.
func selfUpgrade(from string) error {
	url := os.Getenv("HOBNOB_UPGRADE_URL")
	if url == "" {
		url = "https://github.com/jakesmd/hobnob/releases/latest/download/install.sh"
	}

	executablePath := os.Getenv("HOBNOB_UPGRADE_EXE")
	if executablePath == "" {
		var err error
		executablePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("locate executable: %w", err)
		}
		executablePath, err = filepath.EvalSymlinks(executablePath)
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}
	}

	if err := runInstaller(url, filepath.Dir(executablePath), os.Stdout, os.Stderr); err != nil {
		return err
	}
	tui.HobnobDone(os.Stdout, upgradedMessage(from, executablePath))
	return nil
}

// upgradedMessage names the versions the upgrade went between. The new one
// is asked of the binary the installer just wrote; when it can't say, the
// message names no versions.
func upgradedMessage(from, executablePath string) string {
	out, err := exec.Command(executablePath, "--version").Output()
	fields := strings.Fields(string(out))
	if err != nil || len(fields) == 0 {
		return "upgraded"
	}
	return "upgraded " + cli.DisplayVersion(from) + " → " + fields[len(fields)-1]
}

func runInstaller(url, installDir string, stdout, stderr io.Writer) error {
	tui.HobnobNote(stdout, "downloading the latest installer…")

	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	workDir, err := os.MkdirTemp("", "hobnob-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	scriptPath := filepath.Join(workDir, "install.sh")
	script, err := os.Create(scriptPath)
	if err != nil {
		return fmt.Errorf("create installer: %w", err)
	}
	if _, err := io.Copy(script, resp.Body); err != nil {
		script.Close()
		return fmt.Errorf("save installer: %w", err)
	}
	script.Close()

	// Run from the temp dir: inside a hobnob checkout the installer would
	// build from source instead of fetching the release.
	cmd := exec.Command("bash", scriptPath)
	cmd.Dir = workDir
	cmd.Env = append(installerEnv(), "HOBNOB_INSTALL_DIR="+installDir)
	out, errOut := tui.HobnobOutput(stdout), tui.HobnobOutput(stderr)
	cmd.Stdout = out
	cmd.Stderr = errOut
	err = cmd.Run()
	out.Flush()
	errOut.Flush()
	if err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	return nil
}

// installerEnv drops HOBNOB_VERSION, which would pin the installer to that
// release instead of the one it was stamped with.
func installerEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HOBNOB_VERSION=") {
			env = append(env, kv)
		}
	}
	return env
}
