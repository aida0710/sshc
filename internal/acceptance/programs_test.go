package acceptance_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// startsAProcess は、プログラムを起こす書き方である。
//
// **継ぎ目の名前だけを探していた。** かつてここには `RunOutput(ctx` も並んでいた。
// それだけを見ていたころ、os/exec を直接呼ぶ新しい起動はこの見張りの外を素通り
// した——実際に 1 つ増えたのに緑のままだった。継ぎ目を通らない起こし方こそ、
// 気づきたいものである。
//
// その継ぎ目自体は、本番のどこからも呼ばれないまま残っていたので消した。**残った
// 一つで足りる。** プログラムを起こす道は os/exec しかない。
var startsAProcess = []string{"exec.Command"}

// allowedToStartPrograms は、そこからプログラムを起こしてよいファイルである。
//
// **このアプリケーションは OpenSSH のプログラムを自分から選んで実行しない。**
// 認証も、鍵の一覧も、agent への登録も、このプロセスの中で行う。`ssh-keygen` は
// 名前を出すだけで、走らせるのは利用者である——`HardwareCommand` が返すのは
// 画面に表示する引数の並びである。
//
// **接続には例外がひとつある。** ProxyCommand は、利用者が「これで繋げ」と
// 書いた綴りをそのまま起こす設定であり、それを断ることは「その接続先は扱えない」
// と言うことだった。起こす綴りを選ぶのはこのアプリケーションではなく、
// ~/.ssh/config を書いた本人である。
//
// ログイン時起動は OS に任せた。launchd や systemd の unit を書いていたのは
// このアプリケーション自身だったが、その仕組みごと消えた。ここに並ぶものは
// いずれも os/exec を直接呼んでおり、**そうしてよい理由がそれぞれ違う。**
//
// **一覧を持つ形にしてあるのは、増えたときに気づくためである。** 「OpenSSH が
// 無いこと」を検査すると、OpenSSH でない何かが増えても緑のままになる。
var allowedToStartPrograms = []string{
	// 入口をブラウザへ渡す。**出力を取る実行ではない。** 起こしたら手を離すので
	// 継ぎ目（出力を集めて返す道）を通す必要が無く、渡すのは自分で組み立てた
	// loopback の URL ひとつだけである。開けなくても失敗ではない——URL は
	// 標準出力にも出ているので、貼れる綴りは人の手元に残る。
	"cmd/sshc/browser.go",
	// ローカルシェルには擬似端末が要る。継ぎ目は出力を集めて返すものなので、
	// PTY を握って対話し続けるこれは、そもそもあそこを通れない。
	"internal/terminal/pty_unix.go",
	// native build helper は配布される sshc runtime ではない。固定された go/git/npm と
	// artifact verifier の sh/pwsh だけを allowlist 済み argv で起動し、caller input を
	// shell command line として組み立てない。
	"internal/nativebuild/nativebuild.go",
	// ProxyCommand は、接続そのものを外部のプログラムに任せる設定である。
	//
	// **ここだけは、利用者が書いた文字列をシェルに渡す。** ~/.ssh/config の
	// その 1 行を、ssh が読むのと同じように解釈するためであり、綴りを分解して
	// argv を組み立てると、引用もリダイレクトも書けなくなる——それは ssh と
	// 違う意味になる。
	//
	// **黙って起こさない。** 起こす綴りは接続のたびに端末へ 1 行出る
	// （tracer.announce）。既定が無言であることの、ただ一つの例外である。
	"internal/sshclient/proxycommand.go",
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
		if !slices.ContainsFunc(startsAProcess, func(way string) bool {
			return strings.Contains(string(contents), way)
		}) {
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
		t.Fatalf("nothing at all was found that starts a program with %v; this test is looking for the wrong thing", startsAProcess)
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
