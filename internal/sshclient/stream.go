package sshclient

import (
	"context"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

// RemoteFailureExit は、相手のコマンドが終了状態を残さずに終わったときの番号
// である。
//
// OpenSSH と同じ 255 を使う。呼び出し側は既にこの番号を「繋がらなかった」
// として知っている。1 にすると「コマンドが 1 で終わった」と区別できなくなる。
const RemoteFailureExit = 255

// Streams は、相手のコマンドに繋ぐ三本である。
//
// stdout と stderr を分ける。対話セッションがこれを一本に畳んでいるのは
// 端末がひとつだからで、端末が無いここではその理由が無い。混ぜてしまうと、
// 出力を集めた側が、コマンドの結果と診断を区別できない。
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Stream は、相手のコマンドをひとつ走らせ、その出力を流しながら終了状態を返す。
//
// Run と異なり出力量と実行時間に独自の上限を設けず、終了はリモートまたは ctx が決める。
//
// 端末は要求しない。要求すれば出力に画面制御が混ざり、集めた側が読めなく
// なる。端末が要る用（`sudo` など）は、この入口の担当ではない。
//
// 保存済み資格情報は対話接続と同じ認証経路で使用するが、追加質問は拒否し、
// 未知のホスト鍵も受け入れない。標準入力はすべてリモートコマンドへ渡す。
//
// error はコマンドを実行できなかった場合だけ返す。リモートの終了コードは第1戻り値で返す。
func (d Dialer) Stream(
	ctx context.Context, target Target, command string, streams Streams,
) (int, error) {
	if command == "" {
		return RemoteFailureExit, errors.New("no remote command was given")
	}
	// 未知のホストを暗黙に受け入れない。Run と同じ理由である。
	strict := requireKnownHosts(target)
	// ProxyCommand is a local process, not remote command stderr. It must still
	// be visible at this CLI boundary even when connection verbosity is quiet.
	trace := newTracer(Quiet, streams.Err)

	client, closers, err := d.chain(ctx, strict, noPrompt, trace)
	if err != nil {
		return RemoteFailureExit, err
	}
	defer func() {
		_ = client.Close()
		closeAll(closers)
	}()

	session, err := client.NewSession()
	if err != nil {
		return RemoteFailureExit, err
	}
	defer func() { _ = session.Close() }()

	session.Stdin = streams.In
	session.Stdout = streams.Out
	session.Stderr = streams.Err
	for _, variable := range strict.SetEnv {
		// 拒否されても続ける。サーバーが AcceptEnv を絞っているのは普通のことで、
		// それを理由に諦める必要はない。
		_ = session.Setenv(variable.Name, variable.Value)
	}

	// ctx が終わったらセッションを閉じる。閉じなければ Run は相手が終わる
	// まで返らず、Ctrl-C を押したユーザーが待たされ続ける。
	finished := make(chan struct{})
	defer close(finished)

	// 設定された ServerAliveInterval を落とさない。対話セッションはこれを
	// 尊重していて、こちらだけ無視していた。長く暗黙に走るコマンドこそ、
	// 途中の機器に接続を捨てられて困る側である。既定を作りはしない（OpenSSH も
	// 既定では送らない）。設定したユーザーの指示を通すだけである。
	if keepAlive := keepAliveLoop(client, strict.KeepAlive, strict.KeepAliveMax, finished); keepAlive != nil {
		go keepAlive()
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-finished:
		}
	}()

	runErr := session.Run(command)
	if cause := ctx.Err(); cause != nil {
		return RemoteFailureExit, cause
	}
	var exit *ssh.ExitError
	switch {
	case runErr == nil:
		return 0, nil
	case errors.As(runErr, &exit):
		// 終了コードは結果であって失敗ではない。相手が応答したのだから、その結果を返す。
		return exit.ExitStatus(), nil
	}
	var missing *ssh.ExitMissingError
	if errors.As(runErr, &missing) {
		return RemoteFailureExit, fmt.Errorf("the remote command ended without a status: %w", runErr)
	}
	return RemoteFailureExit, runErr
}
