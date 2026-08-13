package acceptance_test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scrollbackCanary は、端末の出力として PTY からこのプロセスへ入る文字列である。
//
// リモートが表示した何もかもがスクロールバックに混ざる。保存済みパスワードを
// 使った接続の痕跡もそこに入る。だからこの文字列は、~/.ssh/sshc/ のどのファイル
// にも、どのログ行にも現れてはならない。
const scrollbackCanary = "scrollback-canary-8d31f7"

// TestTheScrollbackNeverReachesTheStateDirectory は、端末の出力がディスクへ
// 出ていかないことを表明する。
//
// 既存の漏洩検査は注入した logger だけを見ているので、これは別に書いてある。
// 見るのはファイルシステムそのものである——世代バックアップ、journal、history、
// metadata、vault、handoff、そして状態ディレクトリ配下の何もかも。
func TestTheScrollbackNeverReachesTheStateDirectory(t *testing.T) {
	f := newFixture(t)

	// ローカルシェルを一本開く。ハーネスの starter は本物の PTY を確保しないので、
	// ここで届くバイト列はテストが押し込んだものだけである。
	response := f.do(http.MethodPost, "/api/v1/terminal/sessions", []byte(`{"kind":"shell"}`))
	status := response.StatusCode
	body := readBody(t, response)
	if status != http.StatusCreated {
		t.Fatalf("open a shell = %d: %s", status, body)
	}
	var opened struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
		StreamTicket string `json:"streamTicket"`
	}
	if err := json.Unmarshal([]byte(body), &opened); err != nil {
		t.Fatal(err)
	}
	if opened.Session.ID == "" {
		t.Fatalf("the response carried no session: %s", body)
	}

	// 端末が出力したことにする。これがスクロールバックへ入り、そこから先は
	// WebSocket 以外のどこへも出ていってはならない。
	f.terminal.emit(scrollbackCanary + "\r\n")

	// セッションを閉じ、書き込みを誘発しうる操作を一通り走らせる。
	closed := f.do(http.MethodDelete, "/api/v1/terminal/sessions/"+opened.Session.ID, nil)
	readBody(t, closed)
	stateDirectory := filepath.Join(f.home, ".ssh", "sshc")
	found := []string{}
	err := filepath.WalkDir(stateDirectory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(contents), scrollbackCanary) {
			found = append(found, strings.TrimPrefix(path, f.home))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("the scrollback reached the state directory: %v", found)
	}

	// ~/.ssh 全体も見る。設定ツリーの側へ落ちていないことまで含めて表明する。
	found = nil
	if err := filepath.WalkDir(f.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(contents), scrollbackCanary) {
			found = append(found, strings.TrimPrefix(path, f.home))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("the scrollback reached the ssh directory: %v", found)
	}

	// ログ行も見る。既存の漏洩検査と同じ針だが、対象がスクロールバックである。
	if strings.Contains(f.logs.String(), scrollbackCanary) {
		t.Fatal("the scrollback was written to a log line")
	}
}
