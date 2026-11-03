package sshclient

import (
	"errors"
	"strings"
)

// ErrAuthenticationRejected は、差し出した認証をサーバーがどれも受け入れなかったことを表す。
//
// 鍵が authorized_keys から外された、パスワードが違う、差し出した鍵が多すぎて
// MaxAuthTries で切られた、といった失敗である。設定か資格情報を直さない限り、
// 繋ぎ直しても同じ理由で断られる。
var ErrAuthenticationRejected = errors.New("the server rejected every authentication method offered")

// rejectedAuthenticationText は、x/crypto/ssh が認証の拒否を言う文の頭である。
// x/crypto/ssh はこの失敗に型を付けず、この文でだけ返す（client_auth.go）。
const rejectedAuthenticationText = "ssh: unable to authenticate"

// disconnectText は、x/crypto/ssh がサーバーから受けた SSH_MSG_DISCONNECT を言う
// 文の頭である。
const disconnectText = "ssh: disconnect"

// tooManyAuthenticationFailuresText は、サーバーが MaxAuthTries を超えた接続を切る
// ときの理由である。OpenSSH は「Too many authentication failures」、x/crypto/ssh は
// 小文字で送るので、大文字小文字を区別せずに比べる。
//
// エージェントに鍵を多く入れていると、サーバーは「no supported methods remain」に
// なる前にこの理由で切る。x/crypto/ssh はこの切断を包まずにそのまま返す。
const tooManyAuthenticationFailuresText = "too many authentication failures"

// unansweredKeyboardInteractiveText は、サーバーが keyboard-interactive を問いを出さずに断った
// ときの x/crypto/ssh の文である（client_auth.go）。51 は SSH_MSG_USERAUTH_FAILURE で、PAM の無い
// OpenSSH が鍵の登録の無い相手へ返す形である。OpenSSH なら Permission denied と出る場面なので、
// 認証の拒否として扱う。
const unansweredKeyboardInteractiveText = "ssh: unexpected message type 51"

// authenticationFailedSentence は、認証の失敗を画面のターミナルで言う文である。
const authenticationFailedSentence = "認証に失敗しました。"

// authenticationRejected は、元の文を保ったまま ErrAuthenticationRejected としても
// 見分けられる失敗である。接続ログに出る文は x/crypto/ssh のもののままにする。
type authenticationRejected struct {
	err error
	// serverMethods は、サーバーが最後に受け付けると言った認証方式である。分からなければ空。
	serverMethods []string
}

func (rejected authenticationRejected) Error() string { return rejected.err.Error() }

// sentence は、画面のターミナルに出す文である。何が足りないかを言えるよう、方式の一覧を添える。
func (rejected authenticationRejected) sentence() string {
	if len(rejected.serverMethods) == 0 {
		return authenticationFailedSentence
	}
	return authenticationFailedSentence + "サーバーが受け付ける方式：" + strings.Join(rejected.serverMethods, ", ")
}

func (rejected authenticationRejected) Unwrap() []error {
	return []error{ErrAuthenticationRejected, rejected.err}
}

// classifyHandshakeFailure は、ハンドシェイクの失敗のうち認証の拒否に型を付ける。
func classifyHandshakeFailure(err error) error {
	text := err.Error()
	if strings.Contains(text, rejectedAuthenticationText) || strings.Contains(text, unansweredKeyboardInteractiveText) ||
		isDisconnectedForTooManyAuthenticationFailures(text) {
		return authenticationRejected{err: err}
	}
	return err
}

// withServerMethods は、認証の拒否に、サーバーが受け付けると言った認証方式を添える。
func withServerMethods(err error, serverMethods []string) error {
	var rejected authenticationRejected
	if !errors.As(err, &rejected) {
		return err
	}
	rejected.serverMethods = serverMethods
	return rejected
}

func isDisconnectedForTooManyAuthenticationFailures(text string) bool {
	return strings.Contains(text, disconnectText) &&
		strings.Contains(strings.ToLower(text), tooManyAuthenticationFailuresText)
}

// storedCredentialRejected は、保存済みの値が拒否され、非対話の接続なので代わりの
// 値を利用者に尋ねられなかった失敗である。
//
// 拒否（ErrAuthenticationRejected）としても、尋ねられなかった失敗
// （ErrPromptUnavailable）としても見分けられる。認証テストは拒否として
// authentication_denied と報告し、CLI は保存した値を確かめる案内を添える。
type storedCredentialRejected struct {
	// credentials は、拒否された保存済みの値の名前（password、verification code）である。
	credentials []string
	err         error
}

func (rejected storedCredentialRejected) Error() string {
	verb := "was"
	if len(rejected.credentials) > 1 {
		verb = "were"
	}
	return "the saved " + strings.Join(rejected.credentials, " and ") + " " + verb + " rejected; " + rejected.err.Error()
}

func (rejected storedCredentialRejected) Unwrap() []error {
	return []error{ErrAuthenticationRejected, rejected.err}
}
