package acceptance_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// startsAProcess は、外部プログラムを起こす唯一の継ぎ目である。
const startsAProcess = "RunOutput(ctx"

// allowedToStartPrograms は、そこからプログラムを起こしてよいファイルである。
//
// **このアプリケーションは OpenSSH のプログラムを一つも実行しない。** 接続も、
// 認証も、鍵の一覧も、agent への登録も、このプロセスの中で行う。`ssh-keygen` は
// 名前を出すだけで、走らせるのは利用者である——`HardwareCommand` が返すのは
// 画面に表示する引数の並びである。
//
// **一覧を持つ形にしてあるのは、増えたときに気づくためである。** 「OpenSSH が
// 無いこと」を検査すると、OpenSSH でない何かが増えても緑のままになる。
var allowedToStartPrograms = []string{
	// 既定ブラウザを開く。
	"internal/platform/macos/browser.go",
	"internal/platform/linux/browser.go",
	// ログイン時起動を登録する。
	"internal/platform/macos/loginitem.go",
	"internal/platform/linux/loginitem.go",
}

// TestOnlyTheNamedSubsystemsStartAProgram は、プロセスを起こす場所を固定する。
//
// ここが赤くなったら、外部プログラムを起こす場所が増えたということである。
// それ自体は悪いことではないが、**気づかずに増えることは悪い。**
func TestOnlyTheNamedSubsystemsStartAProgram(t *testing.T) {
	repository := filepath.Join("..", "..")
	var found []string

	err := filepath.WalkDir(repository, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "bin", ".claude", ".worktrees", "dist", ".playwright-browsers", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(contents), startsAProcess) {
			return nil
		}
		relative, err := filepath.Rel(repository, path)
		if err != nil {
			return err
		}
		found = append(found, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// 正のコントロール: 一覧そのものが古くなっていないこと。**一つも見つからない
	// なら、この検査は探し方を間違えている。**
	if len(found) == 0 {
		t.Fatalf("no caller of %q was found at all; this test is looking for the wrong thing", startsAProcess)
	}
	slices.Sort(found)

	for _, path := range found {
		// 継ぎ目そのものの宣言と実装は数えない。**起こす場所ではなく、
		// 起こし方を定めている場所である。**
		switch path {
		case "internal/platform/command.go", "internal/platform/process/command.go":
			continue
		}
		if !slices.Contains(allowedToStartPrograms, path) {
			t.Errorf("%s starts a program but is not on the list; add it and say why", path)
		}
	}
	for _, allowed := range allowedToStartPrograms {
		if !slices.Contains(found, allowed) {
			t.Errorf("%s is on the list but starts no program any more; take it off", allowed)
		}
	}
}

// **OpenSSH のプログラムの名前が、起動される側に現れないこと。**
//
// 上の検査は「どこから起こすか」を固定する。こちらは「何を起こすか」を見る。
// 片方だけでは、許された場所から ssh を起こす変更が通ってしまう。
func TestNoSubsystemNamesAnOpenSSHProgramToRun(t *testing.T) {
	repository := filepath.Join("..", "..")
	programs := []string{`"ssh"`, `"ssh-add"`, `"ssh-keyscan"`, `"ssh-agent"`}

	for _, path := range allowedToStartPrograms {
		contents, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		for _, program := range programs {
			if strings.Contains(string(contents), program) {
				t.Errorf("%s names %s as a program to run", path, program)
			}
		}
	}
}
