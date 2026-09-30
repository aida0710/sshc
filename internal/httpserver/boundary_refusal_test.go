package httpserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

// 同じ境界のエラーは、設定、接続、鍵、Vault のどの handler の系統から返っても
// 同じ status と code になる。どれかが 500 に落ちると、画面は待てば済む拒否と
// 内部の欠陥を見分けられない。
func TestEveryProblemMappingAnswersBoundaryRefusalsTheSameWay(t *testing.T) {
	mappings := map[string]func(*echo.Context, error) error{
		"config":     serviceProblem,
		"connection": connectionProblem,
		"key":        keyProblem,
		"vault":      vaultProblem,
	}
	refusals := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "pending", err: storage.ErrPendingTransaction, status: http.StatusConflict, code: "workspace_pending_transaction"},
		{name: "wrapped pending", err: fmt.Errorf("commit: %w", storage.ErrPendingTransaction), status: http.StatusConflict, code: "workspace_pending_transaction"},
		{name: "busy", err: storage.ErrWorkspaceBusy, status: http.StatusConflict, code: "workspace_busy"},
		{name: "locked", err: secret.ErrLocked, status: http.StatusConflict, code: "vault_locked"},
		{name: "connection changed", err: application.ErrConnectionChanged, status: http.StatusConflict, code: "connection_changed"},
		{name: "file too large", err: storage.ErrFileTooLarge, status: http.StatusUnprocessableEntity, code: "file_too_large"},
	}
	for mappingName, mapping := range mappings {
		for _, refusal := range refusals {
			t.Run(mappingName+"/"+refusal.name, func(t *testing.T) {
				recorder := problemResponse(t, mapping, refusal.err)
				if recorder.Code != refusal.status {
					t.Fatalf("status = %d, want %d: %s", recorder.Code, refusal.status, recorder.Body.String())
				}
				if code := problemCode(t, recorder.Body.Bytes()); code != refusal.code {
					t.Fatalf("code = %q, want %q", code, refusal.code)
				}
			})
		}
	}
}

// 鍵の移動は設定ファイルも書き換えるので、設定の衝突とグラフの誤りは設定の保存と同じ形で返す。
func TestKeyProblemAnswersConfigRefusalsLikeTheConfigSave(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "conflict", err: &application.ConflictError{}, status: http.StatusConflict, code: "config_conflict"},
		{name: "graph", err: &application.GraphError{}, status: http.StatusUnprocessableEntity, code: "config_graph_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := problemResponse(t, keyProblem, test.err)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.status, recorder.Body.String())
			}
			if code := problemCode(t, recorder.Body.Bytes()); code != test.code {
				t.Fatalf("code = %q, want %q", code, test.code)
			}
		})
	}
}

func problemResponse(t *testing.T, mapping func(*echo.Context, error) error, err error) *httptest.ResponseRecorder {
	t.Helper()
	engine := echo.New()
	engine.GET("/", func(c *echo.Context) error { return mapping(c, err) })
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	return recorder
}

// 鍵のパスを変える操作（グループの改名・削除と鍵の移動）は、Vault や鍵ファイルが
// 読んだあとに変わっていたら、何も書かずに断る。設定の系統からも鍵の系統からも、
// 内部の欠陥の 500 ではなく、読み直しを促す 409 で返す。
func TestKeyPathChangesAnswerAnExternalChangeWithAConflict(t *testing.T) {
	for _, mapping := range []struct {
		name    string
		problem func(*echo.Context, error) error
	}{{name: "config", problem: serviceProblem}, {name: "key", problem: keyProblem}} {
		for _, test := range []struct {
			name string
			err  error
		}{
			{name: "vault digest", err: &storage.ConflictError{Path: "sshc/secrets"}},
			{name: "key files", err: application.ErrKeyFilesChanged},
		} {
			t.Run(mapping.name+"/"+test.name, func(t *testing.T) {
				recorder := problemResponse(t, mapping.problem, test.err)
				if recorder.Code != http.StatusConflict {
					t.Fatalf("status = %d, want 409: %s", recorder.Code, recorder.Body.String())
				}
				if code := problemCode(t, recorder.Body.Bytes()); code != "external_change" {
					t.Fatalf("code = %q, want external_change", code)
				}
			})
		}
	}
}
