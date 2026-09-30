package httpserver

import (
	"errors"
	"net/http"

	"sshc/internal/application"
	"sshc/internal/secret"
)

// boundaryRefusalFor は、err が、どの画面の操作でも起きうる storage・secret・application の
// 境界の拒否なら、それを HTTP の status と安定した code に写したものを返す。
//
// 設定、接続、鍵、Vault の各 problem 関数は、自分の対応付けに当てはまらなかったエラーを
// 500 にする前にこれを通す。同じエラーが handler の系統ごとに 409 と 500 に分かれると、
// 画面は待てば済む拒否と内部の欠陥を見分けられないからである。
func boundaryRefusalFor(err error) (problemReply, bool) {
	switch {
	// ErrPendingTransaction は ErrWorkspaceBusy を包むので、先に判定する。待っても
	// 解消しないので、busy と同じ案内（少し待って再試行）にしてはいけない。
	case errors.Is(err, application.ErrPendingTransaction):
		return problemReply{
			status: http.StatusConflict, code: "workspace_pending_transaction",
			detail: "an interrupted change must be completed or rolled back from History before sshc can save again",
		}, true
	case errors.Is(err, application.ErrWorkspaceBusy):
		return problemReply{
			status: http.StatusConflict, code: "workspace_busy",
			detail: "another sshc process kept this workspace locked for more than 30 seconds",
		}, true
	// 施錠中の要求は Security middleware が先に断る。ここへ来るのは、処理の途中で
	// 施錠された場合だけである。画面がロック画面へ移れるよう、同じ code にする。
	case errors.Is(err, secret.ErrLocked):
		return problemReply{status: http.StatusConflict, code: "vault_locked"}, true
	case errors.Is(err, application.ErrConnectionChanged):
		return problemReply{status: http.StatusConflict, code: "connection_changed"}, true
	case errors.Is(err, application.ErrFileTooLarge):
		return problemReply{status: http.StatusUnprocessableEntity, code: "file_too_large"}, true
	}
	return problemReply{}, false
}
