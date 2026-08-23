//go:build unix

// 実行ビットで信頼を決めるのは Unix の話である。Windows の `Perm()` は書き込み
// ビットしか運ばないので、ここの二つは向こうでは意味を持たない。`0o755` も
// `0o644` も同じ「実行不可」に見え、片方は理由もなく通ってしまう。Windows で
// 何を実行してよいと見なすかは Windows Task 4 の trusted toolchain が決める。
package process_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/platform/process"
)

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
