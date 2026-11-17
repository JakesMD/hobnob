package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout swaps os.Stdout for the duration of f and returns everything
// written to it. Unlike the old internal/runner helper this drains
// concurrently, so it can't deadlock once output exceeds the pipe buffer.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	f()

	os.Stdout = orig
	w.Close()
	return <-done
}

func serveScript(t *testing.T, status int, script string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(script))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/install.sh"
}

func TestUpgradeFlagRunsInstaller(t *testing.T) {
	// given --upgrade flag, when Run called, then runs the release installer
	// against the running binary's directory (why: upgrading must also set up
	// PATH and completion, which only the installer does)
	// Arrange
	exe := filepath.Join(t.TempDir(), "hobnob")
	if err := os.WriteFile(exe, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOBNOB_UPGRADE_URL", serveScript(t, http.StatusOK, `echo "install dir: $HOBNOB_INSTALL_DIR"`))
	t.Setenv("HOBNOB_UPGRADE_EXE", exe)

	// Act
	var runErr error
	out := captureStdout(t, func() {
		runErr = New("v0.0.0").Run(context.Background(), []string{"--upgrade"})
	})

	// Assert
	if runErr != nil {
		t.Fatalf("expected success, got: %v", runErr)
	}
	if !strings.Contains(out, "install dir: "+filepath.Dir(exe)) {
		t.Errorf("expected installer to target %s, got: %s", filepath.Dir(exe), out)
	}
}

func TestRunInstaller(t *testing.T) {
	t.Run("given HTTP 404, when upgrading, then returns error (why: a missing installer must not look like success)", func(t *testing.T) {
		// Arrange
		url := serveScript(t, http.StatusNotFound, "")

		// Act
		err := runInstaller(url, t.TempDir(), &bytes.Buffer{}, &bytes.Buffer{})

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected 404 in error, got: %v", err)
		}
	})

	t.Run("given installer exits non-zero, when upgrading, then returns error with its stderr passed through (why: a failed install must fail the upgrade)", func(t *testing.T) {
		// Arrange
		url := serveScript(t, http.StatusOK, "echo 'error: unsupported arch' >&2; exit 1")
		var stderr bytes.Buffer

		// Act
		err := runInstaller(url, t.TempDir(), &bytes.Buffer{}, &stderr)

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(stderr.String(), "unsupported arch") {
			t.Errorf("expected installer stderr passed through, got: %s", stderr.String())
		}
	})

	t.Run("given upgrade run inside a hobnob checkout, when upgrading, then installer runs outside it (why: the installer builds from source when run in a checkout)", func(t *testing.T) {
		// Arrange
		checkout := t.TempDir()
		os.WriteFile(filepath.Join(checkout, "go.mod"), []byte("module hobnob\n"), 0o644)
		os.MkdirAll(filepath.Join(checkout, "cmd", "hobnob"), 0o755)
		os.WriteFile(filepath.Join(checkout, "cmd", "hobnob", "main.go"), []byte("package main\n"), 0o644)
		t.Chdir(checkout)
		url := serveScript(t, http.StatusOK, `[[ -f go.mod ]] && echo "in checkout" || echo "outside checkout"`)
		var out bytes.Buffer

		// Act
		err := runInstaller(url, t.TempDir(), &out, &bytes.Buffer{})

		// Assert
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if !strings.Contains(out.String(), "outside checkout") {
			t.Errorf("expected installer to run outside the checkout, got: %s", out.String())
		}
	})

	t.Run("given HOBNOB_VERSION set, when upgrading, then installer does not see it (why: it would pin the upgrade to that version instead of the latest)", func(t *testing.T) {
		// Arrange
		t.Setenv("HOBNOB_VERSION", "v0.1.0")
		url := serveScript(t, http.StatusOK, `echo "version: ${HOBNOB_VERSION:-unset}"`)
		var out bytes.Buffer

		// Act
		err := runInstaller(url, t.TempDir(), &out, &bytes.Buffer{})

		// Assert
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if !strings.Contains(out.String(), "version: unset") {
			t.Errorf("expected HOBNOB_VERSION hidden from installer, got: %s", out.String())
		}
	})
}
