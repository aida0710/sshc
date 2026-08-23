package windows_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/windows"
)

// Task 6 の配線はこれを platform.Toolchain として渡す。合わないなら、配線では
// なくここで壊れる。
var _ platform.Toolchain = windows.Toolchain{}

// openSSHDirectory は、Windows に同梱される OpenSSH の在り処を作る。
//
// 表記は filepath.Join に任せる。この検査はどの OS でも走るので、区切り文字を
// テストが決めると、macOS 上の期待値だけが Windows の実装と食い違う。
func openSSHDirectory(t *testing.T, windowsDirectory string) string {
	t.Helper()
	directory := filepath.Join(windowsDirectory, "System32", "OpenSSH")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	return directory
}

func writeProgram(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// PATH に置かれた ssh-keygen.exe は決して勝たない。
//
// Windows の PATH には利用者が書き込めるディレクトリが並ぶ。そこの一本が鍵の
// 生成を引き受ければ、生成された鍵はもう利用者のものではない。Toolchain が見る
// のは同梱された OpenSSH の絶対パスだけであり、無ければ「無い」と返す。
func TestTheToolchainNeverTakesAKeygenFromThePath(t *testing.T) {
	windowsDirectory := t.TempDir()
	openSSHDirectory(t, windowsDirectory)
	poisoned := t.TempDir()
	writeProgram(t, poisoned, "ssh-keygen.exe")
	writeProgram(t, poisoned, "ssh-keygen")
	t.Setenv("PATH", poisoned)

	toolchain := windows.NewToolchain(windowsDirectory)
	path, err := toolchain.KeyGen()
	if !errors.Is(err, windows.ErrProgramNotFound) {
		t.Fatalf("KeyGen() = %q, %v; want ErrProgramNotFound", path, err)
	}
}

func TestTheToolchainTakesKeygenFromTheSystemOpenSSHDirectory(t *testing.T) {
	windowsDirectory := t.TempDir()
	want := writeProgram(t, openSSHDirectory(t, windowsDirectory), "ssh-keygen.exe")

	path, err := windows.NewToolchain(windowsDirectory).KeyGen()
	if err != nil {
		t.Fatalf("KeyGen() = %v", err)
	}
	if path != want {
		t.Errorf("KeyGen() = %q, want %q", path, want)
	}
}

// 名前が一致するディレクトリは、プログラムではない。
func TestTheToolchainRefusesADirectoryNamedLikeTheProgram(t *testing.T) {
	windowsDirectory := t.TempDir()
	directory := openSSHDirectory(t, windowsDirectory)
	if err := os.Mkdir(filepath.Join(directory, "ssh-keygen.exe"), 0o755); err != nil {
		t.Fatal(err)
	}

	if path, err := windows.NewToolchain(windowsDirectory).KeyGen(); !errors.Is(err, windows.ErrProgramNotFound) {
		t.Fatalf("KeyGen() = %q, %v; want ErrProgramNotFound", path, err)
	}
}

// `.exe` が付いていないものは Windows のプログラムではない。Unix 側の
// 表記をそのまま探すと、同梱の OpenSSH が無いマシンで、たまたま同じ名前の
// ファイルが選ばれうる。
func TestTheToolchainDoesNotSettleForTheUnixSpelling(t *testing.T) {
	windowsDirectory := t.TempDir()
	writeProgram(t, openSSHDirectory(t, windowsDirectory), "ssh-keygen")

	if path, err := windows.NewToolchain(windowsDirectory).KeyGen(); !errors.Is(err, windows.ErrProgramNotFound) {
		t.Fatalf("KeyGen() = %q, %v; want ErrProgramNotFound", path, err)
	}
}

// 空の Windows ディレクトリからは、相対パスを組み立てない。組み立てれば、
// それを stat するのはこのプロセスの作業ディレクトリの中である。
func TestTheZeroToolchainResolvesNothing(t *testing.T) {
	if path, err := (windows.Toolchain{}).KeyGen(); !errors.Is(err, windows.ErrProgramNotFound) {
		t.Fatalf("KeyGen() = %q, %v; want ErrProgramNotFound", path, err)
	}
	if path, err := windows.NewToolchain("").KeyGen(); !errors.Is(err, windows.ErrProgramNotFound) {
		t.Fatalf("KeyGen() = %q, %v; want ErrProgramNotFound", path, err)
	}
}
