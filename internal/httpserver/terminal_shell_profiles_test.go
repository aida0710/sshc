package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"sshc/internal/platform"
)

// 画面の検証は arguments を配列として読むので、null を返すと一覧ごと捨てて、シェルを選べなくなる。
func TestShellProfilesListArgumentsAsAnArrayWhenAShellHasNone(t *testing.T) {
	handlers := TerminalHandlers{ShellProfiles: func() []platform.ShellProfile {
		return []platform.ShellProfile{{ID: "default", Label: "zsh", Path: "/bin/zsh"}}
	}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/terminal/shell-profiles", nil)
	if err := handlers.ListShellProfiles(echo.New().NewContext(request, recorder)); err != nil {
		t.Fatal(err)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"arguments":[]`) {
		t.Fatalf("list = %s", body)
	}
}
