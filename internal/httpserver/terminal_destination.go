package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/validate"
)

var errNoTargetResolver = errors.New("this engine does not resolve SSH targets")

// Destination は、alias がこのエンジンの SSH の設定で行き着く先（最後の段の HostName・Port・User）を
// 返す。繋ぎには行かない。実行先で開いた SSH のセッションのファイルをこのマシンから読む前に、画面が
// 実行先の答えとこのマシンの答えを比べ、同じ行き先のときだけ読む（別のマシンのファイルを見せない）。
func (h TerminalHandlers) Destination(c *echo.Context) error {
	alias := c.Param("connectionAlias")
	if validate.Alias(alias) != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	if h.ResolveTarget == nil {
		return unexpectedProblem(c, "alias_unresolvable", errNoTargetResolver)
	}
	target, err := h.ResolveTarget(alias)
	if err != nil {
		// 解決の失敗の文には、利用者が書いた ProxyCommand などが入りうるので返さない。
		return problem(c, http.StatusUnprocessableEntity, "alias_unresolvable")
	}
	return c.JSON(http.StatusOK, api.TerminalDestination{
		Alias: alias, HostName: target.HostName, Port: target.Port, User: target.User,
	})
}
