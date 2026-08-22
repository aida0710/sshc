package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

// raw は、JSON ではない本文をそのまま送る。画像はバイト列であって書類ではない。
func (h *testHarness) raw(t *testing.T, method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Host = "127.0.0.1:43123"
	request.Header.Set(echo.HeaderContentType, "application/octet-stream")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	// **読み取りにも CSRF ヘッダーが要る。** この API はそう決めてある。
	request.Header.Set(CSRFHeader, h.csrf)
	if method != http.MethodGet {
		request.Header.Set(echo.HeaderOrigin, "http://127.0.0.1:43123")
	}
	request.AddCookie(h.cookie)
	response := httptest.NewRecorder()
	h.echo.ServeHTTP(response, request)
	return response
}

func pngBytes(payload string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), []byte(payload)...)
}

// **名乗られた型ではなく、中身から決まった型を返す。**
//
// 送ってきた側の Content-Type はこのバイト列について何も保証しない。`.png` と
// 名乗る HTML を image/png として返せば、それを開いたブラウザが何をするかは
// もうこちらの手を離れる。
func TestABackgroundIsServedAsWhatItsBytesSay(t *testing.T) {
	harness := newConfigHarness(t)

	created := harness.raw(t, http.MethodPost, "/api/v1/terminal/backgrounds?name=Office%20Wall", pngBytes("real"))
	if created.Code != http.StatusCreated {
		t.Fatalf("POST = %d, body %s", created.Code, created.Body.String())
	}
	var stored struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Name != "office-wall.png" || stored.Type != "image/png" {
		t.Fatalf("stored = %#v, want the server to have named it", stored)
	}

	served := harness.raw(t, http.MethodGet, "/api/v1/terminal/backgrounds/"+stored.Name, nil)
	if served.Code != http.StatusOK {
		t.Fatalf("GET = %d", served.Code)
	}
	if got := served.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want the type its bytes say", got)
	}
	if !bytes.Equal(served.Body.Bytes(), pngBytes("real")) {
		t.Fatalf("the bytes came back changed")
	}
	// 推測させない。これが無ければ、名乗った型より先へブラウザが進む。
	if got := served.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}

// 画像でないものは置かれない。**拡張子は名乗るだけで、何も証明しない。**
func TestUploadingSomethingThatIsNotAnImageIsRefused(t *testing.T) {
	harness := newConfigHarness(t)

	response := harness.raw(t, http.MethodPost, "/api/v1/terminal/backgrounds?name=evil.png",
		[]byte("<html><script>alert(1)</script></html>"))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("POST = %d, want it refused", response.Code)
	}
}

// 置かれていない名前は読めない。**綴りで登ろうとしても同じである。**
func TestReadingABackgroundThatWasNeverStoredIsRefused(t *testing.T) {
	harness := newConfigHarness(t)
	if created := harness.raw(t, http.MethodPost, "/api/v1/terminal/backgrounds?name=wall", pngBytes("x")); created.Code != http.StatusCreated {
		t.Fatalf("POST = %d", created.Code)
	}

	// **404 を名指しする。** 「200 でなければよい」にすると、認証で弾かれた
	// だけの応答でも通ってしまう——実際、一度それで緑になっていた。
	for _, name := range []string{"nothing.png", "..%2F..%2Fconfig", "%2Eetc%2Fpasswd"} {
		response := harness.raw(t, http.MethodGet, "/api/v1/terminal/backgrounds/"+name, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%q gave %d, want 404: %s", name, response.Code, response.Body.String())
		}
	}
}

// 一覧は、残りをこちらが数えて返す。**画面が上限を書き写さないためである。**
func TestTheListSaysHowMuchRoomIsLeft(t *testing.T) {
	harness := newConfigHarness(t)
	if created := harness.raw(t, http.MethodPost, "/api/v1/terminal/backgrounds?name=wall", pngBytes("abc")); created.Code != http.StatusCreated {
		t.Fatalf("POST = %d", created.Code)
	}

	response := harness.raw(t, http.MethodGet, "/api/v1/terminal/backgrounds", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("GET = %d", response.Code)
	}
	var listed struct {
		Backgrounds []struct {
			Name  string `json:"name"`
			Bytes int    `json:"bytes"`
		} `json:"backgrounds"`
		RemainingBytes int `json:"remainingBytes"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Backgrounds) != 1 {
		t.Fatalf("listed = %#v", listed.Backgrounds)
	}
	if listed.RemainingBytes <= 0 || listed.RemainingBytes >= 1<<30 {
		t.Fatalf("remainingBytes = %d, want what is actually left", listed.RemainingBytes)
	}
}

func TestABackgroundCanBeThrownAway(t *testing.T) {
	harness := newConfigHarness(t)
	created := harness.raw(t, http.MethodPost, "/api/v1/terminal/backgrounds?name=wall", pngBytes("x"))
	if created.Code != http.StatusCreated {
		t.Fatalf("POST = %d", created.Code)
	}

	if removed := harness.raw(t, http.MethodDelete, "/api/v1/terminal/backgrounds/wall.png", nil); removed.Code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", removed.Code)
	}
	if served := harness.raw(t, http.MethodGet, "/api/v1/terminal/backgrounds/wall.png", nil); served.Code != http.StatusNotFound {
		t.Fatalf("GET after delete = %d", served.Code)
	}
}
