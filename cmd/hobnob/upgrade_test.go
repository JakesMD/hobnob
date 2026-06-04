package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func TestUpgradeFlagPrintsVersion(t *testing.T) {
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		os.Args = []string{"hobnob", "--upgrade"}
		main()
		return
	}

	// Arrange
	newBinary := []byte("new-binary-content")
	archive := makeTarGz(t, map[string][]byte{"hobnob": newBinary})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	}))
	defer srv.Close()
	exe := filepath.Join(t.TempDir(), "hobnob")
	if err := os.WriteFile(exe, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	upgradeURL := srv.URL + "/jakesmd/hobnob/releases/download/v9.9.9/hobnob_os_arch.tar.gz"

	cmd := exec.Command(os.Args[0], "-test.run=TestUpgradeFlagPrintsVersion")
	cmd.Env = append(os.Environ(),
		"hobnob_SUBPROCESS=1",
		"HOBNOB_UPGRADE_URL="+upgradeURL,
		"HOBNOB_UPGRADE_EXE="+exe,
	)

	// Act
	out, err := cmd.CombinedOutput()

	// Assert
	if err != nil {
		t.Fatalf("expected success, got: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "v9.9.9") {
		t.Errorf("expected output to contain 'v9.9.9', got: %s", out)
	}
}

func TestDownloadAndInstall(t *testing.T) {
	t.Run("given HTTP 404, when upgrading, then returns error (why: no release for this platform)", func(t *testing.T) {
		// Arrange
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()
		exe := filepath.Join(t.TempDir(), "hobnob")
		os.WriteFile(exe, []byte("old"), 0755)

		// Act
		err := downloadAndInstall(srv.URL+"/hobnob.tar.gz", exe, &bytes.Buffer{})

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected 404 in error, got: %v", err)
		}
	})

	t.Run("given corrupt gzip body, when upgrading, then returns decompress error (why: malformed download must not replace binary)", func(t *testing.T) {
		// Arrange
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not a gzip"))
		}))
		defer srv.Close()
		exe := filepath.Join(t.TempDir(), "hobnob")
		os.WriteFile(exe, []byte("old"), 0755)

		// Act
		err := downloadAndInstall(srv.URL+"/hobnob.tar.gz", exe, &bytes.Buffer{})

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "decompress failed") {
			t.Errorf("expected decompress error, got: %v", err)
		}
	})

	t.Run("given tarball without hobnob binary, when upgrading, then returns error and leaves original intact (why: incomplete release must not silently succeed)", func(t *testing.T) {
		// Arrange
		archive := makeTarGz(t, map[string][]byte{"README.md": []byte("readme")})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(archive)
		}))
		defer srv.Close()
		exe := filepath.Join(t.TempDir(), "hobnob")
		os.WriteFile(exe, []byte("old"), 0755)

		// Act
		err := downloadAndInstall(srv.URL+"/hobnob.tar.gz", exe, &bytes.Buffer{})

		// Assert
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not found in release archive") {
			t.Errorf("expected 'not found in release archive', got: %v", err)
		}
		content, _ := os.ReadFile(exe)
		if string(content) != "old" {
			t.Errorf("original binary must be untouched, got: %s", content)
		}
	})

	t.Run("given valid tarball with hobnob binary served at versioned URL, when upgrading, then replaces binary and prints new version (why: successful upgrade should confirm with version)", func(t *testing.T) {
		// Arrange
		newBinary := []byte("new-binary-content")
		archive := makeTarGz(t, map[string][]byte{"hobnob": newBinary})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(archive)
		}))
		defer srv.Close()
		exe := filepath.Join(t.TempDir(), "hobnob")
		os.WriteFile(exe, []byte("old"), 0755)

		var out bytes.Buffer

		// Act
		err := downloadAndInstall(srv.URL+"/jakesmd/hobnob/releases/download/v9.9.9/hobnob_linux_amd64.tar.gz", exe, &out)

		// Assert
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		content, _ := os.ReadFile(exe)
		if !bytes.Equal(content, newBinary) {
			t.Errorf("expected binary to be replaced with new content")
		}
		if !strings.Contains(out.String(), "v9.9.9") {
			t.Errorf("expected output to contain 'v9.9.9', got: %s", out.String())
		}
	})

	t.Run("given GitHub-style redirect through versioned URL to CDN, when upgrading, then prints new version (why: real GitHub redirects end at CDN so version must be captured from intermediate redirect)", func(t *testing.T) {
		// Arrange
		newBinary := []byte("new-binary-content")
		archive := makeTarGz(t, map[string][]byte{"hobnob": newBinary})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "latest") {
				http.Redirect(w, r, "/jakesmd/hobnob/releases/download/v9.9.9/hobnob_linux_amd64.tar.gz", http.StatusFound)
				return
			}
			w.Write(archive)
		}))
		defer srv.Close()
		exe := filepath.Join(t.TempDir(), "hobnob")
		os.WriteFile(exe, []byte("old"), 0755)

		var out bytes.Buffer

		// Act
		err := downloadAndInstall(srv.URL+"/jakesmd/hobnob/releases/latest/download/hobnob_linux_amd64.tar.gz", exe, &out)

		// Assert
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if !strings.Contains(out.String(), "v9.9.9") {
			t.Errorf("expected output to contain 'v9.9.9', got: %s", out.String())
		}
	})
}
