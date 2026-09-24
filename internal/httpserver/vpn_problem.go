package httpserver

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/vpnrefusal"
)

// vpnRefusalStatus は、VPN の拒否の語と、応答の状態の対応である。無い語は 409 にする。
var vpnRefusalStatus = map[string]int{
	vpnrefusal.CodeProfileUnknown:     http.StatusNotFound,
	vpnrefusal.CodeConnectionUnknown:  http.StatusNotFound,
	vpnrefusal.CodeVaultMissing:       http.StatusNotFound,
	vpnrefusal.CodeProfileInvalid:     http.StatusBadRequest,
	vpnrefusal.CodeDestinationInvalid: http.StatusBadRequest,
}

// vpnProblem は、VPN の拒否を画面と CLI が扱える応答へ直す。
//
// 項目の誤りには `field`・`reason`・`limit` を、経路を用意できなかったときには
// `reason` を添える。どれも Go が決めた語だけで、利用者の入力や秘密、ログの
// 断片は載せない。
func vpnProblem(c *echo.Context, err error) error {
	refusal, known := vpnrefusal.Of(err)
	if !known {
		return serviceProblem(c, err)
	}
	status, listed := vpnRefusalStatus[refusal.Code]
	if !listed {
		status = http.StatusConflict
	}
	return problemWith(c, status, problemPayload{
		Code: refusal.Code, Field: refusal.Field, Reason: refusal.Reason, Limit: refusal.Limit,
	})
}
