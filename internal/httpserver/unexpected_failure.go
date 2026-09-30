package httpserver

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/labstack/echo/v5"
)

// unexpectedProblem は、想定外の失敗の原因を記録してから 500 の problem を返す。
func unexpectedProblem(c *echo.Context, code string, err error) error {
	logUnexpectedFailure(c, code, err)
	return problem(c, http.StatusInternalServerError, code)
}

// unexpectedReply は、500 番台で返す想定外の失敗について、原因を記録してから reply を返す。
func unexpectedReply(c *echo.Context, reply problemReply, err error) error {
	logUnexpectedFailure(c, reply.code, err)
	return writeProblemReply(c, reply)
}

// unexpectedNoContent は、本文なしで答える /cli/ のルート（CLI セッションと Vault の操作）の
// 想定外の失敗について、原因を記録する。
func unexpectedNoContent(c *echo.Context, err error) error {
	logUnexpectedFailure(c, "", err)
	return c.NoContent(http.StatusInternalServerError)
}

// logUnexpectedFailure は、想定外の失敗の原因を、注入された logger へ 1 行残す。
//
// 応答には code しか載せないので、ここで残さないと、書き込みの失敗や壊れたファイルの
// 原因を engine のログからも追えない。出し先は c.Logger()（server が Options.Logger を
// 入れたもの）に限る。ログへの秘密混入の検査はその logger だけを見ているので、
// グローバルの slog へは書かない。err の文からは絶対パスを伏せる。パスにはアカウント名が
// 入り、検査はホームディレクトリのパスもログに出てはいけないものとして扱う。
// err が nil（依存が配線されていないなど）でも、どの route がどの code で失敗したかは残す。
func logUnexpectedFailure(c *echo.Context, code string, err error) {
	attributes := []any{
		slog.String("method", c.Request().Method),
		slog.String("route", c.Path()),
		slog.String("code", code),
	}
	if err != nil {
		attributes = append(attributes,
			slog.Any("error_types", errorTypes(err)),
			slog.String("error", errorTextWithoutPaths(err)),
		)
	}
	c.Logger().Error("request failed unexpectedly", attributes...)
}

// maxErrorChainDepth は、記録する包みの型の数の上限である。原因を見分けるには数段で足り、
// 自分自身を包む誤った実装があってもログを止めないために上限を置く。
const maxErrorChainDepth = 8

// errorTypes は、err から包まれた順に、各段の型の名前を返す。文を読まなくても、
// どの層で起きた失敗か（*fs.PathError か、syscall.Errno か）を見分けられるようにする。
func errorTypes(err error) []string {
	var types []string
	for current := err; current != nil && len(types) < maxErrorChainDepth; current = errors.Unwrap(current) {
		types = append(types, fmt.Sprintf("%T", current))
	}
	return types
}

// absolutePathStart は、Unix の "/" か Windows の "C:\" や "\" で始まる絶対パスの頭である。
// engine の OS によらず同じ規則で伏せるので、filepath.IsAbs は使わない。
const absolutePathStart = `(?:[A-Za-z]:)?[\\/]`

// absolutePath は、文の始まりか区切り（空白、引用符、括弧、=）の直後にある絶対パスに一致する。
// "HTTP/1.1" や URL の "//host" は区切りの直後ではないので一致しない。パスは空白の手前で
// 終わるとみなすので、空白を含むパスはその先が残る。
var absolutePath = regexp.MustCompile(`(^|[\s"'(=])(` + absolutePathStart + `[^\s"'():;,]*)`)

var startsAsAbsolutePath = regexp.MustCompile(`^` + absolutePathStart)

// errorTextWithoutPaths は、err の文から絶対パスを伏せる。
//
// 包みの中の *fs.PathError と *os.LinkError が持つ絶対パスは、先にそのままの文字列で伏せる。
// "C:\Users\Jane Doe\.ssh\config" のように空白を含むホームでも、アカウント名を残さない
// ためである。それ以外の形で文に埋め込まれたパスは absolutePath で探すので、空白を含むと
// 空白より後ろが残る。
func errorTextWithoutPaths(err error) string {
	text := err.Error()
	for _, path := range absolutePathsCarriedBy(err) {
		text = strings.ReplaceAll(text, path, "<path>")
	}
	return withoutAbsolutePaths(text)
}

// absolutePathsCarriedBy は、err の包みをたどり、パスを持つ標準のエラーの絶対パスを、
// 長いものから返す。短いパスが長いパスの頭と同じでも、長いパスを先に伏せれば残らない。
func absolutePathsCarriedBy(err error) []string {
	var paths []string
	var visit func(current error, depth int)
	visit = func(current error, depth int) {
		if current == nil || depth >= maxErrorChainDepth {
			return
		}
		switch carrier := current.(type) {
		case *fs.PathError:
			paths = append(paths, carrier.Path)
		case *os.LinkError:
			paths = append(paths, carrier.Old, carrier.New)
		}
		switch wrapper := current.(type) {
		case interface{ Unwrap() error }:
			visit(wrapper.Unwrap(), depth+1)
		case interface{ Unwrap() []error }:
			for _, inner := range wrapper.Unwrap() {
				visit(inner, depth+1)
			}
		}
	}
	visit(err, 0)
	paths = slices.DeleteFunc(paths, func(path string) bool { return !startsAsAbsolutePath.MatchString(path) })
	slices.SortFunc(paths, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	return paths
}

func withoutAbsolutePaths(text string) string {
	return absolutePath.ReplaceAllString(text, "${1}<path>")
}
