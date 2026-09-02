package acceptance_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"sshc/internal/remotesync"
)

// productionGoFiles は、配布物に載る Go ファイルを走査する。
//
// テストは数えない。検査は自分の都合でインターフェースを組み立ててよく、そこを縛ると、
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

// TestWebPasswordsUseTheSharedControl は、表示切替やアクセシビリティが画面ごとに
// 分岐しないよう、秘密入力の実装場所を PasswordField に限定する。
func TestWebPasswordsUseTheSharedControl(t *testing.T) {
	root := filepath.Join("..", "..", "web", "src")
	passwordInput := regexp.MustCompile(`(?s)type\s*=\s*(?:["']password["']|\{.{0,160}?["']password["'].{0,160}?\})`)
	var found []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tsx") || entry.Name() == "PasswordField.tsx" {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if passwordInput.Match(contents) {
			relative, err := filepath.Rel(filepath.Join("..", ".."), path)
			if err != nil {
				return err
			}
			found = append(found, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(found)
	if len(found) != 0 {
		t.Errorf("PasswordField を介さず秘密入力を実装している: %v", found)
	}
}

// TestWebCardsUseTheSharedControl は、製品画面がカードの見た目を複製しないようにする。
// UIプリミティブはこのclassを組み合わせてよいが、機能側はCardなどの共通部品を通し、
// 角丸・背景・overflowが画面ごとに再び分岐しないようにする。
func TestWebCardsUseTheSharedControl(t *testing.T) {
	root := filepath.Join("..", "..", "web", "src")
	var found []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && entry.Name() == "ui" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".tsx") || strings.HasSuffix(entry.Name(), ".test.tsx") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(contents), "sshc-card") {
			relative, err := filepath.Rel(filepath.Join("..", ".."), path)
			if err != nil {
				return err
			}
			found = append(found, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(found)
	if len(found) != 0 {
		t.Errorf("Card を介さず製品画面に sshc-card を実装している: %v", found)
	}
}

// TestRemoteSyncDoesNotExposeStorageTransactions keeps the transport-facing
// pull preview in remotesync vocabulary. A direct import check cannot detect a
// storage.Request smuggled through an exported service result.
func TestRemoteSyncDoesNotExposeStorageTransactions(t *testing.T) {
	result := reflect.TypeOf(remotesync.PullResult{})
	for index := range result.NumField() {
		field := result.Field(index)
		if !field.IsExported() {
			continue
		}
		if strings.HasPrefix(field.Type.PkgPath(), "sshc/internal/storage") ||
			strings.Contains(field.Type.String(), "storage.") {
			t.Errorf("remotesync.PullResult.%s exposes persistence type %s", field.Name, field.Type)
		}
	}
}

// buildsTheSSHDialer は、プロセス内 SSH の部品一式を組み立ててよい場所である。
//
// 一箇所しかない。internal/app の sshParts の doc は「組み立てる場所はここ
// ひとつである……二箇所で組み立てると、片方だけが vault を見る日が来る」と書いて
// いるが、長いあいだ同じファイルの中に 2 つ目があった。engine 用と
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
// engine と `sshc <接続先>` は別のプロセスなので、根そのものは 2 つある。片方が
// もう片方のオブジェクトを借りることはできない。縛れるのは「どのパッケージが
// 開いてよいか」の方である。cmd/sshc が自分で開いていた頃、一覧と TUI は engine とは
// 別の解決器を通っており、Match ブロックの下に書かれた HostName は画面に出なかった
// 選んだ先と繋がる先が食い違っていた。
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
// 監視する。
//
// 封をされないマネージャがひとつでもあると、置き換えられたファイルの以前の
// 内容が平文で残る。鍵 vault のマネージャがそうなっていた期間があり、その間、
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
	// どのファイルかは縛らない。縛りたいのは「engine の合成の根が暗号化する」で
	// あって、それがどの表記のファイルに書かれているかではない。
	sealedInsideApp := false
	for _, path := range seals {
		if strings.HasPrefix(path, "internal/app/") {
			sealedInsideApp = true
		}
	}
	if !sealedInsideApp {
		t.Errorf("engine の合成の根が封をしていない: %v", seals)
	}
}

// persistenceLayer は、ディスクとネットワークの原始操作を持つパッケージである。
//
// トランザクション・ジャーナル・世代バックアップ・封・条件付き PUT。外向きの
// 応答がここの用語で組まれてはならない。永続化の都合で付けた名前を変えるだけで
// HTTP の契約が動いてしまう。実際、storage の sentinel error は 5 つのハンドラから
// 直接 errors.Is され、そのままレスポンスの code に対応していた。
var persistenceLayer = []string{
	`"sshc/internal/storage"`,
	`"sshc/internal/envelope"`,
	`"sshc/internal/objectstore"`,
}

// TestTheTransportDoesNotReachIntoPersistence は、HTTP 層と永続化層の間に
// サービス層を挟んだままにする。
//
// 用語が要るなら、それを出しているサービスが別名で公開する。internal/keys の
// IsExternalChange や internal/remotesync の Client がそれである。別名であって
// 包み直しではないので errors.Is はどちらの表記でも通り、翻訳の層は増えない。
func TestTheTransportDoesNotReachIntoPersistence(t *testing.T) {
	var found []string
	productionGoFiles(t, func(relative, contents string) {
		if !strings.HasPrefix(relative, "internal/httpserver/") {
			return
		}
		for _, dependency := range persistenceLayer {
			if strings.Contains(contents, dependency) {
				found = append(found, relative+" -> "+dependency)
			}
		}
	})
	slices.Sort(found)
	if len(found) != 0 {
		t.Errorf("HTTP 層が永続化層を直接 import している:\n  %s", strings.Join(found, "\n  "))
	}
}
