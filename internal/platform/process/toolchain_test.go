package process_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"sshc/internal/platform"
	"sshc/internal/platform/process"
)

var _ platform.Toolchain = process.Toolchain{}

func writeProgram(t *testing.T, directory, name string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte("#!/usr/bin/true\n"), mode); err != nil {
		t.Fatal(err)
	}
}

func TestToolchainPrefersTheFirstDirectoryThatHoldsAnExecutable(t *testing.T) {
	preferred := t.TempDir()
	fallback := t.TempDir()
	writeProgram(t, fallback, "ssh-keygen", 0o755)
	writeProgram(t, preferred, "ssh-keygen", 0o755)

	toolchain := process.Toolchain{Directories: []string{preferred, fallback}}

	keygenPath, err := toolchain.KeyGen()
	if err != nil {
		t.Fatalf("KeyGen() = %v", err)
	}
	if want := filepath.Join(preferred, "ssh-keygen"); keygenPath != want {
		t.Errorf("KeyGen() = %q, want %q", keygenPath, want)
	}
}

func TestToolchainIgnoresMissingAndNonExecutableFiles(t *testing.T) {
	directory := t.TempDir()
	writeProgram(t, directory, "ssh-keygen", 0o644)

	toolchain := process.Toolchain{Directories: []string{directory}}
	if _, err := toolchain.KeyGen(); !errors.Is(err, process.ErrProgramNotFound) {
		t.Errorf("KeyGen() = %v, want ErrProgramNotFound", err)
	}
}

func TestToolchainResolvesEveryProgramThroughTheInjectedStat(t *testing.T) {
	installed := fstest.MapFS{"sandbox/ssh-keygen": &fstest.MapFile{Mode: 0o755}}
	var asked []string
	toolchain := process.Toolchain{
		Directories: []string{"/sandbox"},
		Stat: func(name string) (fs.FileInfo, error) {
			asked = append(asked, name)
			return installed.Stat(strings.TrimPrefix(name, "/"))
		},
	}

	resolvers := map[string]func() (string, error){"ssh-keygen": toolchain.KeyGen}
	for program, resolve := range resolvers {
		path, err := resolve()
		if err != nil {
			t.Fatalf("resolving %s = %v", program, err)
		}
		if want := filepath.Join("/sandbox", program); path != want {
			t.Errorf("resolving %s = %q, want %q", program, path, want)
		}
	}
	if len(asked) != len(resolvers) {
		t.Errorf("injected Stat saw %#v, want one lookup per program", asked)
	}
}
