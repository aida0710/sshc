package acceptance_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 「この alias に接続すると何が使われるか」に返すものを二つ持たない。
//
// この package には解決器が 2 つある。
//
//   - effective.Resolve。接続に実際に使われる値。Match ブロックを評価する。
//   - effective.Project。どの行がその値を書いたか。Match ブロックを一切適用
//     しない（条件が接続中の状態に依るので、出所としては帰属させられない）。
//
// 出所を並べる用途では Project が正しい。値を取る用途では、常に間違っている。
//
// 実際に起きていたこと: 経路の展開（jump.go）と到達性の宛先と鍵登録の確認画面が
// Project から値を読んでいた。Match の下に HostName を書いているユーザーに対して、
// 画面は違う番号を見せ、到達性は違う機械を叩き、ProxyJump 自体が Match の下に
// あれば経路は丸ごと空。「踏み台を通らない」と言いながら、通っていた。
//
// 散文では守れないので、ここで数える。
func TestOnlyProvenanceUsesTheProjection(t *testing.T) {
	// 許されるのは、結果を出所として使う場所だけである。
	allowed := map[string]int{
		// Inspect が画面へ「どの行が書いたか」を並べる。値はここから取らない。
		filepath.Join("internal", "diagnostics", "service.go"): 1,
	}

	calls := regexp.MustCompile(`effective\.Project\(`)
	found := map[string]int{}

	root := filepath.Join("..", "..")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case "node_modules", ".git", "bin", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if count := len(calls.FindAll(body, -1)); count > 0 {
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			found[relative] = count
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for path, count := range found {
		want, ok := allowed[path]
		if !ok {
			t.Errorf("%s calls effective.Project.\n"+
				"  Project は Match を適用しない。値が要るなら effective.Resolve を使うこと。", path)
			continue
		}
		if count != want {
			t.Errorf("%s calls effective.Project %d times, want %d.\n"+
				"  増やしたのが出所のためなら、この検査の allowed を更新すること。", path, count, want)
		}
	}
	for path := range allowed {
		if _, ok := found[path]; !ok {
			t.Errorf("%s no longer calls effective.Project. この検査の allowed から消すこと。", path)
		}
	}
}
