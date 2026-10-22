package remoteengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// remoteUnlockPath は、接続先のエンジンの Vault のロックを解除する API である。接続先で
// sshc vault unlock が使うのと同じものである。
const remoteUnlockPath = "/api/v1/passwords/unlock"

var (
	// ErrWrongPassphrase は、接続先の Vault がパスワードを違うと断ったことを表す。
	ErrWrongPassphrase = errors.New("the remote vault refused the password")
	// ErrUnlockRefused は、接続先の Vault がパスワードの違い以外の理由でロックを解除しなかった
	// ことを表す。理由の code を添える。
	ErrUnlockRefused = errors.New("the remote vault was not unlocked")
)

// UnlockVault は、実行先に足したエンジンの Vault のロックを、passphrase で解除する。
// passphrase は接続先へ渡すだけで、このマシンには残さない。
func (m *Manager) UnlockVault(ctx context.Context, alias, passphrase string) error {
	engine, err := m.engine(alias)
	if err != nil {
		return err
	}
	return engine.unlockVault(ctx, passphrase)
}

// unlockVault は、接続先の資格が古くなっていれば受け取り直して一度だけ送り直す。接続先の
// エンジンが起動し直して前のポートで身元を示せないときも、handoff を読み直して一度だけ
// 送り直す。パスワードは、身元を示した接続にだけ書く。
func (e *remoteEngine) unlockVault(ctx context.Context, passphrase string) error {
	body, err := json.Marshal(struct {
		Passphrase string `json:"passphrase"`
	}{passphrase})
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		session, err := e.current(ctx)
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, session.origin()+remoteUnlockPath, bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		authorizeRemoteRequest(request, session)
		response, err := session.client().Do(request)
		if err != nil {
			if isDialFailure(err) {
				e.forget(session)
				if attempt == 0 {
					continue
				}
			}
			return err
		}
		code := refusalCode(response)
		switch {
		case response.StatusCode == http.StatusOK:
			return nil
		case response.StatusCode == http.StatusUnauthorized && attempt == 0:
			e.forget(session)
		case code == "wrong_passphrase":
			return ErrWrongPassphrase
		default:
			return fmt.Errorf("%w: %s (status %d)", ErrUnlockRefused, code, response.StatusCode)
		}
	}
}

// refusalCode は、接続先の答えの本文を読み切って閉じ、断りの code を返す。断りでなければ空である。
func refusalCode(response *http.Response) string {
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusBadRequest {
		return ""
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxRefusalBody))
	var refusal struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &refusal)
	return refusal.Code
}
