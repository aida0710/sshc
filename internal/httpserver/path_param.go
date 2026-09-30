package httpserver

import (
	"net/url"

	"github.com/labstack/echo/v5"
)

// パスの引数を、送り手が逃がす前の値で読む。
//
// echo は、送られたパスの書き方が Go の既定の書き方と違うと、逃がしたままのパス
// （URL.RawPath）で経路を選び、引数も逃がしたまま渡す。書き方が同じなら、戻した
// パス（URL.Path）の値を渡す。ブラウザの encodeURIComponent は `&` を逃がして `(` を
// 逃がさないので、空白や記号を含む名前は前者で届く。どちらで届いても同じ値にする。

// decodedPathParam は、パスの引数 name を、逃がす前の値で返す。
func decodedPathParam(c *echo.Context, name string) (string, error) {
	value := c.Param(name)
	if c.Request().URL.RawPath == "" {
		return value, nil
	}
	return url.PathUnescape(value)
}
