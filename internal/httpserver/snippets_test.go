package httpserver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/snippets"
)

// 上限超過は利用者の操作で起きる拒否なので、文書の破損や保存の失敗と同じ
// 500 にせず、上限ごとの code で返す。
func TestSnippetLimitsAreAnsweredAsConflictsDistinctFromACorruptDocument(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{err: snippets.ErrTooManySnippets, wantStatus: http.StatusConflict, wantCode: "snippet_limit"},
		{err: fmt.Errorf("save: %w", snippets.ErrTooManyStartupBindings), wantStatus: http.StatusConflict, wantCode: "startup_snippet_limit"},
		{err: snippets.ErrInvalidDocument, wantStatus: http.StatusInternalServerError, wantCode: "snippet_failed"},
	}
	for _, test := range tests {
		t.Run(test.wantCode, func(t *testing.T) {
			engine := echo.New()
			engine.GET("/", func(c *echo.Context) error { return snippetProblem(c, test.err) })
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if code := problemCode(t, recorder.Body.Bytes()); code != test.wantCode {
				t.Fatalf("code = %q, want %q", code, test.wantCode)
			}
		})
	}
}
