package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func selfUpgrade() error {
	url := os.Getenv("HOBNOB_UPGRADE_URL")
	if url == "" {
		url = fmt.Sprintf(
			"https://github.com/jakesmd/hobnob/releases/latest/download/hobnob_%s_%s.tar.gz",
			runtime.GOOS, runtime.GOARCH,
		)
	}

	exe := os.Getenv("HOBNOB_UPGRADE_EXE")
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return fmt.Errorf("locate executable: %w", err)
		}
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}
	}

	return downloadAndInstall(url, exe, os.Stdout)
}

func downloadAndInstall(url, exe string, w io.Writer) error {
	fmt.Fprintln(w, "Downloading latest hobnob...")

	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d (no release for %s/%s?)", resp.StatusCode, runtime.GOOS, runtime.GOARCH)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("decompress failed: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("extract failed: %w", err)
		}
		if filepath.Base(hdr.Name) != "hobnob" {
			continue
		}

		tmp, err := os.CreateTemp(filepath.Dir(exe), "hobnob-upgrade-*")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tmpName := tmp.Name()

		if _, err := io.Copy(tmp, tr); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return fmt.Errorf("write upgrade: %w", err)
		}
		tmp.Close()

		if err := os.Chmod(tmpName, 0755); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("chmod: %w", err)
		}

		if err := os.Rename(tmpName, exe); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("replace binary: %w", err)
		}

		// GitHub redirects latest → /releases/download/v1.2.3/...; extract tag from final URL.
		newVersion := ""
		parts := strings.Split(resp.Request.URL.Path, "/")
		for i, part := range parts {
			if part == "download" && i+1 < len(parts) {
				newVersion = parts[i+1]
				break
			}
		}
		if newVersion != "" {
			fmt.Fprintf(w, "Upgraded to hobnob %s\n", newVersion)
		} else {
			fmt.Fprintln(w, "Upgraded successfully.")
		}
		return nil
	}

	return fmt.Errorf("hobnob binary not found in release archive")
}
