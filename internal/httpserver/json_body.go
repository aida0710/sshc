package httpserver

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/strictjson"
)

// errBodyTooLarge は、ボディが入口の上限を超えたことを表す。errInvalidBody を包むので、
// 大きすぎるボディを別に扱わない呼び出し側は、ほかの不正なボディと同じに断れる。
var errBodyTooLarge = fmt.Errorf("%w: larger than the supported maximum", errInvalidBody)

// decodeJSON は、maxRequestBody までのボディを decodeJSONWithin の規則で読む。
func decodeJSON(c *echo.Context, target any) error {
	return decodeJSONWithin(c, maxRequestBody, target)
}

// decodeJSONWithin は、limit バイトまでのボディを 1 個の JSON の値として target へ読む。
//
// どの入口も同じ規則で読む。未知のフィールドは、タイプミスが黙って既定値になるのを
// 防ぐために断る。null と、値のあとに続くデータ（`{"alias":"a"}}` など）も断る。
// 上限を超えたボディは errBodyTooLarge、それ以外の不正は errInvalidBody になる。
//
// 読んだバイト列は、パスワードやパスフレーズを含みうるので、返す前にすべて消す。
// JSON から取り出した string は immutable で消せないので、消せるのはこの生の
// バッファだけである。この限界は保証ではなく、ここに明記しておく。
func decodeJSONWithin(c *echo.Context, limit int64, target any) error {
	body := c.Request().Body
	if body == nil {
		return errInvalidBody
	}
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	defer clear(raw)
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		return errBodyTooLarge
	case err != nil:
		return errInvalidBody
	case int64(len(raw)) > limit:
		return errBodyTooLarge
	case bytes.Equal(bytes.TrimSpace(raw), []byte("null")):
		return errInvalidBody
	}
	if err := strictjson.Decode(raw, target); err != nil {
		return errInvalidBody
	}
	return nil
}
