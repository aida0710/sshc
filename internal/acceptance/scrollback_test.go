package acceptance_test

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/testwait"
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
// 見るのはファイルシステムそのものである。世代バックアップ、journal、history、
// metadata、vault、handoff、そして状態ディレクトリ配下の何もかも。
func TestTheScrollbackNeverReachesTheStateDirectory(t *testing.T) {
	t.Parallel()
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

// TestASessionLogIsWrittenOnlyWhereTheMachineKeepsItsLogs は、セッションのログを保存する設定にしたときだけ、
// 端末の出力がそのマシンの決めたフォルダ（~/.ssh の外）にファイルとして残り、~/.ssh へは出ていかない
// ことを表明する。
func TestASessionLogIsWrittenOnlyWhereTheMachineKeepsItsLogs(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	logs := filepath.Join(t.TempDir(), "session-logs")

	var settings struct {
		MachineID string          `json:"machineId"`
		Global    json.RawMessage `json:"global"`
		Machines  []struct {
			ID       string          `json:"id"`
			Settings json.RawMessage `json:"settings"`
		} `json:"machines"`
	}
	read := f.do(http.MethodGet, "/api/v1/settings", nil)
	if err := json.Unmarshal([]byte(readBody(t, read)), &settings); err != nil {
		t.Fatal(err)
	}
	var machine json.RawMessage
	for _, candidate := range settings.Machines {
		if candidate.ID == settings.MachineID {
			machine = candidate.Settings
		}
	}
	put := func(path string, base json.RawMessage, change string) {
		t.Helper()
		body := []byte(`{"base":` + string(base) + `,"settings":` + change + `}`)
		response := f.do(http.MethodPut, path, body)
		if status, text := response.StatusCode, readBody(t, response); status != http.StatusOK {
			t.Fatalf("PUT %s = %d: %s", path, status, text)
		}
	}
	put("/api/v1/settings/global", settings.Global, `{"terminal":{"sessionLog":true}}`)
	folder, err := json.Marshal(logs)
	if err != nil {
		t.Fatal(err)
	}
	put("/api/v1/settings/machines/"+settings.MachineID, machine, `{"sessionLogDirectory":`+string(folder)+`}`)

	response := f.do(http.MethodPost, "/api/v1/terminal/sessions", []byte(`{"kind":"shell"}`))
	body := readBody(t, response)
	var opened struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal([]byte(body), &opened); err != nil || opened.Session.ID == "" {
		t.Fatalf("open a shell = %d: %s", response.StatusCode, body)
	}
	f.terminal.emit(scrollbackCanary + "\r\n")
	readBody(t, f.do(http.MethodDelete, "/api/v1/terminal/sessions/"+opened.Session.ID, nil))

	// The engine writes what is left on the screen as the session ends, after the close has answered.
	var inLogs []string
	testwait.Until(t, func() bool {
		inLogs = filesContaining(t, logs, scrollbackCanary)
		return len(inLogs) > 0
	})
	if len(inLogs) != 1 || !strings.HasPrefix(filepath.Base(inLogs[0]), "local-") {
		t.Fatalf("logs holding the output = %v, want one log of the local shell", inLogs)
	}
	if found := filesContaining(t, f.root, scrollbackCanary); len(found) != 0 {
		t.Fatalf("the session log reached the ssh directory: %v", found)
	}
}

// filesContaining は、root の下で needle を含むファイルを返す。
func filesContaining(t *testing.T, root, needle string) []string {
	t.Helper()
	var found []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if contents, readErr := os.ReadFile(path); readErr == nil && strings.Contains(string(contents), needle) {
			found = append(found, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return found
}
