package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// selfUpgrade runs the latest release's install.sh, so an upgrade also gets
// the installer's PATH and completion setup, not just a new binary.
func selfUpgrade() error {
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

	return runInstaller(url, filepath.Dir(executablePath), os.Stdout, os.Stderr)
}

func runInstaller(url, installDir string, stdout, stderr io.Writer) error {
	fmt.Fprintln(stdout, "Downloading latest hobnob installer...")

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
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
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
