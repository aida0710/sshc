package sshclient

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"

	"sshc/internal/keys"
)

// ConnectFailure は、接続できなかった理由の種類である。利用者向けの文（FailureSentence）と、
// 画面の一覧に出す短い語（HTTP の層が通信形式の語へ変える）を選ぶのに使う。
type ConnectFailure int

const (
	// ConnectFailureUnknown は、下のどれにも当たらない失敗である。
	ConnectFailureUnknown ConnectFailure = iota
	// ConnectFailureRefused は、接続先のポートが接続を断った。
	ConnectFailureRefused
	// ConnectFailureTimedOut は、接続先が時間内に応答しなかった。
	ConnectFailureTimedOut
	// ConnectFailureHostNotFound は、接続先の名前が DNS に無かった。
	ConnectFailureHostNotFound
	// ConnectFailureNoNetwork は、このマシンの回線が無い（ルートが無い、DNS サーバーに届かない）。
	ConnectFailureNoNetwork
	// ConnectFailureHostUnreachable は、回線はあるが接続先まで届かない。
	ConnectFailureHostUnreachable
	// ConnectFailureClosed は、やり取りの途中で相手が接続を切った。
	ConnectFailureClosed
)

// ClassifyConnectFailure は、接続の失敗を種類に分ける。認証の拒否や設定の誤りのように、
// 回線の問題ではない失敗は ConnectFailureUnknown を返す。
func ClassifyConnectFailure(err error) ConnectFailure {
	if errors.Is(err, ErrAuthenticationRejected) || errors.Is(err, context.Canceled) {
		return ConnectFailureUnknown
	}
	// DNS の失敗も Timeout() を答えるので、タイムアウトより先に見る。
	var lookup *net.DNSError
	if errors.As(err, &lookup) {
		if lookup.IsNotFound {
			return ConnectFailureHostNotFound
		}
		// DNS サーバーに届かない。回線が切れたマシンの多くは、接続先が IP アドレスでなければ
		// ここで止まる。
		return ConnectFailureNoNetwork
	}
	var unopened *ssh.OpenChannelError
	switch {
	case errors.As(err, &unopened):
		return classifyJumpHostAnswer(unopened.Message)
	case timedOut(err):
		return ConnectFailureTimedOut
	case connectionRefused(err):
		return ConnectFailureRefused
	case networkUnreachable(err):
		return ConnectFailureNoNetwork
	case hostUnreachable(err):
		return ConnectFailureHostUnreachable
	case connectionCut(err):
		return ConnectFailureClosed
	}
	return ConnectFailureUnknown
}

// timedOut は、時間内に応答が無かったかを返す。net の失敗の多くは Timeout() だけで答える。
func timedOut(err error) bool {
	var timeout interface{ Timeout() bool }
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) || connectionTimedOut(err) ||
		(errors.As(err, &timeout) && timeout.Timeout())
}

// classifyJumpHostAnswer は、ProxyJump のホップが次のホップへ繋げなかったときの文を種類に
// 分ける。文は OpenSSH の strerror なので、この文でしか見分けられない。
func classifyJumpHostAnswer(answer string) ConnectFailure {
	lower := strings.ToLower(answer)
	switch {
	case strings.Contains(lower, "refused"):
		return ConnectFailureRefused
	case strings.Contains(lower, "timed out"):
		return ConnectFailureTimedOut
	case strings.Contains(lower, "no route"), strings.Contains(lower, "unreachable"):
		return ConnectFailureHostUnreachable
	case strings.Contains(lower, "name or service"), strings.Contains(lower, "could not resolve"):
		return ConnectFailureHostNotFound
	}
	return ConnectFailureUnknown
}

// connectionCut は、やり取りの途中で相手が接続を切ったかを返す。サーバーが SSH_MSG_DISCONNECT を
// 送って切ったときも含む。
func connectionCut(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || connectionReset(err) ||
		strings.Contains(err.Error(), disconnectText)
}

// failureSentence は、理由の種類ごとの利用者向けの文である。
var failureSentence = map[ConnectFailure]string{
	ConnectFailureRefused:         "接続を拒否されました。SSHサーバーが動いているか、ポート番号が合っているかを確認してください。",
	ConnectFailureTimedOut:        "接続先から応答がありません。接続先の電源とネットワーク、ファイアウォールを確認してください。",
	ConnectFailureHostNotFound:    "接続先の名前からIPアドレスを引けません。HostNameを確認してください。",
	ConnectFailureNoNetwork:       "ネットワークに接続できません。回線を確認してください。",
	ConnectFailureHostUnreachable: "接続先まで届きません。ネットワークと接続先の電源を確認してください。",
	ConnectFailureClosed:          "接続先が接続を切りました。接続先のSSHサーバーの状態を確認してください。",
}

// typedFailureSentence は、この package の型で分かる失敗の文である。上から順に見る。
var typedFailureSentence = []struct {
	err      error
	sentence string
}{
	{ErrHostKeyDeclined, "ホスト鍵を受け入れなかったため、接続しませんでした。"},
	{ErrHostKeyUnknown, "このホスト鍵はknown_hostsにありません。フィンガープリントを確認して登録してから、接続し直してください。"},
	{ErrHostKeyChanged, "ホスト鍵が変わりました。Known Hostsを確認してから接続し直してください。"},
	{ErrHostKeyRevoked, "ホスト鍵は失効済みとして登録されているため接続できません。"},
	{ErrHostKeyChangedDuringRekey, "接続の途中でホスト鍵が変わったため、接続を切りました。"},
	{ErrNoIdentity, "利用できる秘密鍵またはssh-agentの鍵がありません。"},
	{ErrNoAuthMethod, "この接続で利用できる認証方式がありません。"},
	{ErrPromptAborted, "認証を中止しました。"},
	{keys.ErrPassphraseRequired, "秘密鍵のパスフレーズが必要です。"},
	{keys.ErrWrongPassphrase, "秘密鍵のパスフレーズが違います。"},
}

// FailureSentence は、接続の失敗を、画面のターミナルに出す日本語の文にする。元の英語の文は
// 接続ログの「詳しく」から出す（Dialer.failSession）。分からない失敗は、元の文を添えて返す。
func FailureSentence(err error) string {
	var jump *JumpHostError
	if errors.As(err, &jump) {
		return jump.sentence()
	}
	var explained *ExplainedError
	if errors.As(err, &explained) {
		return explained.Sentence
	}
	var symlinked *KnownHostsSymlinkError
	if errors.As(err, &symlinked) {
		return "known_hostsがシンボリックリンクのため、ホスト鍵を保存できませんでした。sshで一度接続して鍵を登録してから、接続し直してください。"
	}
	var rejected authenticationRejected
	if errors.As(err, &rejected) {
		return rejected.sentence()
	}
	if errors.Is(err, ErrAuthenticationRejected) {
		return authenticationFailedSentence
	}
	for _, typed := range typedFailureSentence {
		if errors.Is(err, typed.err) {
			return typed.sentence
		}
	}
	if sentence, known := failureSentence[ClassifyConnectFailure(err)]; known {
		return sentence
	}
	return "接続できませんでした：" + err.Error()
}
