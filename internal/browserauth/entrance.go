package browserauth

import (
	"errors"
	"fmt"

	"sshc/internal/session"
)

var (
	// ErrRegistrationRejected は、提示された登録 token では入り直せないことを表す。
	ErrRegistrationRejected = errors.New("browser registration rejected")
	// ErrSessionNotIssued は、登録は受け付けたがセッションを発行できなかったことを表す。
	ErrSessionNotIssued = errors.New("browser session not issued")
)

// Entrance は、ブラウザ登録からの入り直しとサインアウトで、登録とそこから入った
// セッションを一緒に扱う。
//
// 登録を消したら、その登録から入ったセッションもまとめて失効させる。登録だけを
// 消すと、複製された token で先に入り直した側のセッションが、複製を検知した後も
// サインアウトした後も、セッションの期限まで残る。
type Entrance struct {
	Registrations *Store
	Sessions      *session.Manager
}

// Entry は、入り直したブラウザに渡すものである。
type Entry struct {
	Credentials session.Credentials
	// SetCookie は、新しいセッションを発行したので cookie を設定し直すかを表す。
	SetCookie bool
	// BrowserToken は差し替え後の登録 token。ブラウザはこれを保存し直す。
	BrowserToken string
}

// Recover は、登録 token で入り直す。有効な既存のセッションがあればそこへ加える。
// 複製された token を検知したら、その登録から入ったセッションをすべて失効させる。
func (e Entrance) Recover(existingSessionID, presented string) (Entry, error) {
	recovery, err := e.Registrations.Recover(presented)
	if err != nil {
		return Entry{}, err
	}
	e.Sessions.RevokeRegistration(recovery.Stolen)
	if !recovery.Accepted() {
		return Entry{}, ErrRegistrationRejected
	}
	credentials, setCookie, err := e.Sessions.JoinOrIssue(existingSessionID, recovery.Registration)
	if err != nil {
		return Entry{}, fmt.Errorf("%w: %w", ErrSessionNotIssued, err)
	}
	return Entry{Credentials: credentials, SetCookie: setCookie, BrowserToken: recovery.Token}, nil
}

// SignOut は、このブラウザのセッションを消す。登録 token が添えられていれば登録も
// 消し、その登録から入ったほかのセッションも失効させる。
func (e Entrance) SignOut(sessionID, presented string) error {
	if presented != "" && e.Registrations != nil {
		forgotten, err := e.Registrations.Forget(presented)
		if err != nil {
			return err
		}
		e.Sessions.RevokeRegistration(forgotten)
	}
	e.Sessions.Revoke(sessionID)
	return nil
}
