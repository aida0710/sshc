package terminal

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLocalWorkingDirectoryPreservesLiteralShellCharacters(t *testing.T) {
	t.Parallel()
	directory := filepath.Join(t.TempDir(), "project's $(whoami); & work")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{directory, filepath.ToSlash(directory), filepath.ToSlash(directory) + "/"} {
		resolved, err := ResolveLocalWorkingDirectory(candidate)
		if err != nil || resolved != directory {
			t.Fatalf("ResolveLocalWorkingDirectory(%q) = %q, %v; want %q", candidate, resolved, err, directory)
		}
	}
}

func TestLocalWorkingDirectoryRefusesMissingFilesAndAmbiguousPaths(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	file := filepath.Join(directory, "notes.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	candidates := []string{
		"", "relative", "~", file, filepath.Join(directory, "missing"),
		directory + "/\x00", directory + "/../" + filepath.Base(directory),
		strings.Repeat("/", localWorkingDirectoryLimit+1),
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, "/srv/remote", "C:relative")
	} else {
		candidates = append(candidates, "C:/Users/engine", `\\server\share`)
	}
	for _, candidate := range candidates {
		if _, err := ResolveLocalWorkingDirectory(candidate); !errors.Is(err, ErrInvalidLocalWorkingDirectory) {
			t.Errorf("ResolveLocalWorkingDirectory(%q) = %v; want refusal", candidate, err)
		}
	}
}
