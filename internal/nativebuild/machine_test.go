package nativebuild_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sshc/internal/nativebuild"
)

// buildFor は指定ターゲット用の最小バイナリをビルドする。
//
// 本物のコンパイラに焼かせる。手で組み立てたヘッダを読ませても、
// 確かめられるのは自分の書いた偽物であって、Go が出すものではない。
func buildFor(t *testing.T, goos, goarch string) string {
	t.Helper()
	directory := t.TempDir()
	source := filepath.Join(directory, "main.go")
	if err := os.WriteFile(source, []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	name := "probe"
	if goos == "windows" {
		name += ".exe"
	}
	output := filepath.Join(directory, name)
	build := exec.Command("go", "build", "-o", output, source)
	build.Env = append(os.Environ(),
		"GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOFLAGS=")
	if combined, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build %s/%s: %v\n%s", goos, goarch, err, combined)
	}
	return output
}

// ファイル名だけの検査では、異なるアーキテクチャのバイナリを検出できない。
func TestTheArchitectureOfEachTargetIsRead(t *testing.T) {
	for _, target := range []struct{ goos, goarch string }{
		{"windows", "amd64"},
		{"windows", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"linux", "amd64"},
		{"linux", "arm64"},
	} {
		name := target.goos + "/" + target.goarch
		t.Run(name, func(t *testing.T) {
			path := buildFor(t, target.goos, target.goarch)
			if err := nativebuild.VerifyBinaryArchitecture(path, target.goos, target.goarch); err != nil {
				t.Fatalf("a real %s binary was refused: %v", name, err)
			}
		})
	}
}

// arm64 の成果物に amd64 バイナリが入る誤りを検出する。
func TestABinaryForTheWrongArchitectureIsRefused(t *testing.T) {
	for _, target := range []struct{ goos, built, claimed string }{
		{"windows", "amd64", "arm64"},
		{"windows", "arm64", "amd64"},
		{"darwin", "amd64", "arm64"},
		{"linux", "arm64", "amd64"},
	} {
		name := target.goos + " " + target.built + " as " + target.claimed
		t.Run(name, func(t *testing.T) {
			path := buildFor(t, target.goos, target.built)
			err := nativebuild.VerifyBinaryArchitecture(path, target.goos, target.claimed)
			if err == nil {
				t.Fatalf("a %s binary passed as %s", target.built, target.claimed)
			}
			if !strings.Contains(err.Error(), target.claimed) {
				t.Errorf("the refusal does not name the expected architecture: %v", err)
			}
		})
	}
}

// 別 OS のバイナリも検出する。同じバイナリを再利用すると Linux の
// AppImage に macOS のバイナリが入る。ビルドは通り、配ってから壊れる。
func TestABinaryForTheWrongOperatingSystemIsRefused(t *testing.T) {
	for _, target := range []struct{ built, claimed string }{
		{"darwin", "linux"},
		{"linux", "windows"},
		{"windows", "darwin"},
	} {
		t.Run(target.built+" as "+target.claimed, func(t *testing.T) {
			path := buildFor(t, target.built, runtime.GOARCH)
			if err := nativebuild.VerifyBinaryArchitecture(path, target.claimed, runtime.GOARCH); err == nil {
				t.Fatalf("a %s binary passed as %s", target.built, target.claimed)
			}
		})
	}
}

// 読めないものを通さない。空のファイルは実行ファイルではない。
func TestSomethingThatIsNotABinaryIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.exe")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := nativebuild.VerifyBinaryArchitecture(path, "windows", "amd64"); err == nil {
		t.Fatal("an empty file passed as a Windows binary")
	}
}
