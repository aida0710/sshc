package acceptance_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/handoff"
	"sshc/internal/remoteengine"
	"sshc/internal/session"
	"sshc/internal/storage"
)

// engineLink は、SSH の接続の代わりに、同じマシンで動いている実行先のエンジンへつなぐ。実行先のエンジンは
// 本物のサーバーで、handoff も本物のファイルを読む。
type engineLink struct {
	remoteHome string
	done       chan struct{}
}

func (l *engineLink) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}

// Run が受けるコマンドは、実行先の handoff を読むものだけである。
func (l *engineLink) Run(context.Context, remoteengine.RunRequest) (remoteengine.RunResult, error) {
	document, err := os.ReadFile(filepath.Join(l.remoteHome, ".ssh", "sshc", "cli"))
	return remoteengine.RunResult{Stdout: document}, err
}
func (l *engineLink) Ping(time.Duration) error { return nil }
func (l *engineLink) Wait() error              { <-l.done; return nil }
func (l *engineLink) Close() error {
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	return nil
}

// forwardingTo は、remote を実行先として足した、このマシンの側の取り次ぎを返す。
func forwardingTo(t *testing.T, remote *fixture) *remoteengine.Manager {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := remoteengine.NewManager(remoteengine.ManagerOptions{
		Store: remoteengine.NewStore(workspace), LocalVersion: "acceptance",
		Connect: func(context.Context, string) (remoteengine.Link, error) {
			return &engineLink{remoteHome: remote.home, done: make(chan struct{})}, nil
		},
	})
	t.Cleanup(manager.Close)
	if err := manager.AddEngine(context.Background(), "remote"); err != nil {
		t.Fatalf("add the remote engine: %v", err)
	}
	return manager
}

// forwarded は、ブラウザの要求を取り次ぎ、実行先の本物のハンドラの答えを返す。
func forwarded(t *testing.T, manager *remoteengine.Manager, method, remotePath, body string, header map[string]string) (int, string) {
	t.Helper()
	request := httptest.NewRequest(method, "/api/v1/engines/remote"+strings.TrimPrefix(remotePath, "/api/v1"), strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range header {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	path, _, _ := strings.Cut(remotePath, "?")
	if err := manager.ForwardAPI(recorder, request, "remote", path); err != nil {
		t.Fatalf("forward %s %s: %v", method, remotePath, err)
	}
	return recorder.Code, recorder.Body.String()
}

// 取り次ぎの相手を代役ではなく本物のサーバーにして、画面が頼る答えを確かめる: 確認のトークンで守られた操作、
// Vault のロック、マスターパスワードの間違い。
func TestForwardedOperationsAreAnsweredByTheRemoteEnginesOwnHandlers(t *testing.T) {
	t.Parallel()
	remote := newFixture(t)
	manager := forwardingTo(t, remote)

	t.Run("a private key is revealed with the token the remote engine issued", func(t *testing.T) {
		keyID := remote.keyID()
		status, body := forwarded(t, manager, http.MethodPost, "/api/v1/actions",
			string(mustJSON(t, map[string]any{"kind": session.ActionRevealPrivateKey, "target": keyID})), nil)
		var issued struct {
			Token string `json:"token"`
		}
		if status != http.StatusCreated || json.Unmarshal([]byte(body), &issued) != nil || issued.Token == "" {
			t.Fatalf("the remote engine issued no token: %d %s", status, body)
		}
		status, body = forwarded(t, manager, http.MethodPost, "/api/v1/keys/"+keyID+"/reveal", "", map[string]string{handoff.ActionHeader: issued.Token})
		if status != http.StatusOK || !strings.Contains(body, remote.canaries.PrivateKeyLine) {
			t.Fatalf("reveal with the remote engine's token = %d", status)
		}
		// 同じトークンは2度使えない。断りは操作そのもののもので、画面がその code で読む。
		status, body = forwarded(t, manager, http.MethodPost, "/api/v1/keys/"+keyID+"/reveal", "", map[string]string{handoff.ActionHeader: issued.Token})
		if status != http.StatusForbidden || !strings.Contains(body, `"code":"action_token_`) || strings.Contains(body, remote.canaries.PrivateKeyLine) {
			t.Fatalf("reveal with a spent token = %d %s", status, body)
		}
		status, body = forwarded(t, manager, http.MethodPost, "/api/v1/keys/"+keyID+"/reveal", "", nil)
		if status != http.StatusForbidden || !strings.Contains(body, `"code":"action_token_required"`) {
			t.Fatalf("reveal without a token = %d %s", status, body)
		}
	})

	t.Run("a locked remote Vault is told apart from this machine's, and a wrong master password is said as such", func(t *testing.T) {
		if status, body := forwarded(t, manager, http.MethodPost, "/api/v1/passwords/lock", "", nil); status != http.StatusOK {
			t.Fatalf("lock the remote Vault = %d %s", status, body)
		}
		t.Cleanup(remote.unlockAgain)
		// ロック中でも、Vault の状態は読める（画面がロックの解除の欄を出す）。
		status, body := forwarded(t, manager, http.MethodGet, "/api/v1/passwords", "", nil)
		if status != http.StatusOK || !strings.Contains(body, `"unlocked":false`) {
			t.Fatalf("the status of a locked remote Vault = %d %s", status, body)
		}
		// ほかの操作は、このマシンの Vault のロック（vault_locked）と取り違えない code で断る。
		status, body = forwarded(t, manager, http.MethodGet, "/api/v1/credentials", "", nil)
		if status != http.StatusBadGateway || !strings.Contains(body, `"code":"remote_engine_vault_locked"`) {
			t.Fatalf("an operation behind a locked remote Vault = %d %s", status, body)
		}
		status, body = forwarded(t, manager, http.MethodPost, "/api/v1/passwords/unlock", `{"passphrase":"not the master password"}`, nil)
		if status != http.StatusForbidden || !strings.Contains(body, `"code":"wrong_passphrase"`) {
			t.Fatalf("unlock with a wrong master password = %d %s", status, body)
		}
		status, body = forwarded(t, manager, http.MethodPost, "/api/v1/passwords/unlock", `{"passphrase":"`+fixtureMasterPassword+`"}`, nil)
		if status != http.StatusOK || !strings.Contains(body, `"unlocked":true`) {
			t.Fatalf("unlock with the master password = %d %s", status, body)
		}
	})

	t.Run("the SSH configuration read is the remote machine's", func(t *testing.T) {
		status, body := forwarded(t, manager, http.MethodGet, "/api/v1/config/overview", "", nil)
		if status != http.StatusOK || !strings.Contains(body, `"hosts"`) {
			t.Fatalf("the remote overview = %d %s", status, body)
		}
	})

	// 保存の断りは、何がぶつかったかを本文で伝える。取り次ぎが本文を落とすと、画面は理由を出せない。
	t.Run("a save the remote engine refuses says why in its own words", func(t *testing.T) {
		before := remote.read("config")
		status, body := forwarded(t, manager, http.MethodPost, "/api/v1/config/save",
			string(mustJSON(t, map[string]any{"kind": "file_raw", "path": "config", "base": "not what the file holds", "raw": "Host changed\n"})), nil)
		if status != http.StatusConflict || !strings.Contains(body, `"code":`) || strings.Contains(body, "remote_engine_") {
			t.Fatalf("a save against a stale base = %d %s", status, body)
		}
		if string(remote.read("config")) != string(before) {
			t.Fatal("the refused save still changed the remote config")
		}
	})
}
