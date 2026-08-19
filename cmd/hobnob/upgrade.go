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

	return downloadAndInstall(url, executablePath, os.Stdout)
}

func versionFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "download" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func downloadAndInstall(url, executablePath string, out io.Writer) error {
	fmt.Fprintln(out, "Downloading latest hobnob...")

	// GitHub redirects latest → /releases/download/v1.2.3/... → CDN.
	// Capture version from the intermediate GitHub-domain redirect.
	var newVersion string
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if newVersion == "" {
				newVersion = versionFromPath(req.URL.Path)
			}
			return nil
		},
	}

	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d (no release for %s/%s?)", resp.StatusCode, runtime.GOOS, runtime.GOARCH)
	}

	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("decompress failed: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("extract failed: %w", err)
		}
		if filepath.Base(header.Name) != "hobnob" {
			continue
		}

		tempFile, err := os.CreateTemp(filepath.Dir(executablePath), "hobnob-upgrade-*")
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tempFileName := tempFile.Name()

		if _, err := io.Copy(tempFile, tarReader); err != nil {
			tempFile.Close()
			os.Remove(tempFileName)
			return fmt.Errorf("write upgrade: %w", err)
		}
		tempFile.Close()

		if err := os.Chmod(tempFileName, 0755); err != nil {
			os.Remove(tempFileName)
			return fmt.Errorf("chmod: %w", err)
		}

		if err := os.Rename(tempFileName, executablePath); err != nil {
			os.Remove(tempFileName)
			return fmt.Errorf("replace binary: %w", err)
		}

		if newVersion == "" {
			newVersion = versionFromPath(resp.Request.URL.Path)
		}
		if newVersion != "" {
			fmt.Fprintf(out, "Upgraded to hobnob %s\n", newVersion)
		} else {
			fmt.Fprintln(out, "Upgraded successfully.")
		}
		return nil
	}

	return fmt.Errorf("hobnob binary not found in release archive")
}
