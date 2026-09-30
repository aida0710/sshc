package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

type aliasBody struct {
	Alias string `json:"alias"`
}

func decodeAliasBody(t *testing.T, body string, limit int64) (aliasBody, error) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	context := echo.New().NewContext(request, httptest.NewRecorder())
	var decoded aliasBody
	err := decodeJSONWithin(context, limit, &decoded)
	return decoded, err
}

// どの入口も同じ規則で読むので、値のあとに閉じ括弧が余っているボディも、null も、
// 未知のフィールドも、同じように断られる。
func TestDecodeJSONWithinRefusesWhatAnyEntranceWouldRefuse(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "stray closing brace", body: `{"alias":"a"}}`},
		{name: "stray closing bracket", body: `{"alias":"a"}]`},
		{name: "second value", body: `{"alias":"a"} {}`},
		{name: "null", body: `null`},
		{name: "empty", body: ``},
		{name: "unknown field", body: `{"alias":"a","unknown":true}`},
		{name: "truncated", body: `{"alias":"a"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeAliasBody(t, test.body, 64); !errors.Is(err, errInvalidBody) {
				t.Fatalf("decodeJSONWithin(%q) = %v, want errInvalidBody", test.body, err)
			}
		})
	}
}

func TestDecodeJSONWithinAcceptsOneValueWithSurroundingWhitespace(t *testing.T) {
	decoded, err := decodeAliasBody(t, " {\"alias\":\"a\"}\n", 64)
	if err != nil || decoded.Alias != "a" {
		t.Fatalf("decodeJSONWithin = %+v, %v; want alias a", decoded, err)
	}
}

// 上限ちょうどは読み、1 バイトでも超えれば errBodyTooLarge で断る。errBodyTooLarge は
// errInvalidBody でもあるので、大きさを別に扱わない入口は 400 で断れる。
func TestDecodeJSONWithinRefusesABodyOverTheLimitAsTooLarge(t *testing.T) {
	body := `{"alias":"a"}`
	if _, err := decodeAliasBody(t, body, int64(len(body))); err != nil {
		t.Fatalf("a body of exactly the limit = %v", err)
	}
	_, err := decodeAliasBody(t, body, int64(len(body)-1))
	if !errors.Is(err, errBodyTooLarge) || !errors.Is(err, errInvalidBody) {
		t.Fatalf("a body one byte over the limit = %v, want errBodyTooLarge wrapping errInvalidBody", err)
	}
}
