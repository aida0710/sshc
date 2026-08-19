package acceptance_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// productionGoFiles は、配布物に載る Go ファイルを走査する。
//
// テストは数えない——検査は自分の都合で継ぎ目を組み立ててよく、そこを縛ると、
// 縛りたい本番の性質ではなく検査の書き方を縛ることになる。
func productionGoFiles(t *testing.T, visit func(relative, contents string)) {
	t.Helper()
	repository := filepath.Join("..", "..")
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
		relative, err := filepath.Rel(repository, path)
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(relative), string(contents))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// buildsTheSSHDialer は、プロセス内 SSH の部品一式を組み立ててよい場所である。
//
// **一箇所しかない。** internal/app の sshParts の doc は「組み立てる場所はここ
// ひとつである……二箇所で組み立てると、片方だけが vault を見る日が来る」と書いて
// いるが、長いあいだ同じファイルの中に 2 つ目があった——engine 用と
// `sshc <接続先>` 用で、`Stored` と `Password` の差し方だけが違っていた。散文は
// それを止められない。
var buildsTheSSHDialer = []string{"internal/app/ssh.go"}

func TestOnlyOnePlaceAssemblesTheSSHDialer(t *testing.T) {
	var found []string
	productionGoFiles(t, func(relative, contents string) {
		if strings.Contains(contents, "sshclient.Dialer{") {
			found = append(found, relative)
		}
	})
	slices.Sort(found)
	if !slices.Equal(found, buildsTheSSHDialer) {
		t.Errorf("sshclient.Dialer を組み立てる本番ファイル = %v, want %v", found, buildsTheSSHDialer)
	}
}

// TestOnlyTheCompositionRootOpensTheWorkspace は、~/.ssh を開く場所を internal/app に
// 閉じる。
//
// engine と `sshc <接続先>` は別のプロセスなので、根そのものは 2 つある——**片方が
// もう片方のオブジェクトを借りることはできない。** 縛れるのは「どのパッケージが
// 開いてよいか」の方である。cmd/sshc が自分で開いていた頃、一覧と TUI は engine とは
// 別の解決器を通っており、Match ブロックの下に書かれた HostName は画面に出なかった
// ——選んだ先と繋がる先が食い違っていた。
func TestOnlyTheCompositionRootOpensTheWorkspace(t *testing.T) {
	var found []string
	productionGoFiles(t, func(relative, contents string) {
		if strings.Contains(contents, "storage.NewWorkspace(") &&
			!strings.HasPrefix(relative, "internal/app/") {
			found = append(found, relative)
		}
	})
	slices.Sort(found)
	if len(found) != 0 {
		t.Errorf("internal/app の外で ~/.ssh を開いている: %v", found)
	}
}

// TestEveryManagerInTheEngineIsSealed は、封をされないマネージャが生まれないよう
// 見張る。
//
// **封をされないマネージャがひとつでもあると、置き換えられたファイルの以前の
// 内容が平文で残る。** 鍵 vault のマネージャがそうなっていた期間があり、その間、
// パスフレーズの変更は以前の平文の秘密鍵をバックアップに残していた。
func TestEveryManagerInTheEngineIsSealed(t *testing.T) {
	var creates, seals []string
	productionGoFiles(t, func(relative, contents string) {
		if strings.Contains(contents, "storage.NewManager(") {
			creates = append(creates, relative)
		}
		if strings.Contains(contents, ".Seal = ") {
			seals = append(seals, relative)
		}
	})
	slices.Sort(creates)
	for _, path := range creates {
		if !strings.HasPrefix(path, "internal/app/") {
			t.Errorf("internal/app の外がトランザクションマネージャを作っている: %s", path)
		}
	}
	if !slices.Contains(seals, "internal/app/run.go") {
		t.Errorf("engine の合成の根が封をしていない: %v", seals)
	}
}
