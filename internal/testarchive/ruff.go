package testarchive

import (
	"fmt"
	"os"
	"path/filepath"
)

// ruffVersion is pinned to match the version required by the acceptance harness
// (acceptance/internal/ruff.go) and the repo-wide pin in python/pyproject.toml.
const ruffVersion = "0.9.1"

// RuffDownloader handles downloading and extracting ruff releases.
type RuffDownloader struct {
	BinDir string
	Arch   string
}

func (r RuffDownloader) mapArchitecture(arch string) (string, error) {
	switch arch {
	case "arm64":
		return "aarch64", nil
	case "amd64":
		return "x86_64", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s (supported: arm64, amd64)", arch)
	}
}

// Download downloads and extracts ruff for Linux.
func (r RuffDownloader) Download() error {
	ruffArch, err := r.mapArchitecture(r.Arch)
	if err != nil {
		return err
	}

	dir := filepath.Join(r.BinDir, r.Arch)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	ruffTarName := fmt.Sprintf("ruff-%s-unknown-linux-gnu", ruffArch)
	url := fmt.Sprintf("https://github.com/astral-sh/ruff/releases/download/%s/%s.tar.gz", ruffVersion, ruffTarName)

	tempFile := filepath.Join(dir, "ruff.tar.gz")
	if err := downloadFile(url, tempFile); err != nil {
		return err
	}

	if err := ExtractTarGz(tempFile, dir); err != nil {
		return err
	}

	if err := os.Remove(tempFile); err != nil {
		return err
	}

	// The ruff binary is extracted into a directory like
	// ruff-x86_64-unknown-linux-gnu; move it one level up to match the
	// bin/<arch> layout the runner adds to PATH.
	if err := os.Rename(filepath.Join(dir, ruffTarName, "ruff"), filepath.Join(dir, "ruff")); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(dir, ruffTarName))
}
