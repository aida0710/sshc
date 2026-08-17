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
// **OpenSSH と同じ 255 を使う。** 呼び出し側は既にこの番号を「繋がらなかった」
// として知っている。1 にすると「コマンドが 1 で終わった」と区別できなくなる。
const RemoteFailureExit = 255

// Streams は、相手のコマンドに繋ぐ三本である。
//
// **stdout と stderr を分ける。** 対話セッションがこれを一本に畳んでいるのは
// 端末がひとつだからで、端末が無いここではその理由が無い。混ぜてしまうと、
// 出力を集めた側が、コマンドの答えと診断を区別できない。
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Stream は、相手のコマンドをひとつ走らせ、その出力を流しながら終了状態を返す。
//
// **Run とは集め方が違う。** あちらは短い管理操作のためのもので、取り込む量に
// 上限があり（`MaxCapturedOutput`）、既定の制限時間も掛ける。テストスイートの
// 出力はその上限を軽く超えるし、走り終わるまでの時間に上限を置けば、遅い相手で
// 打ち切られる。ここは上限を持たず、終わりを決めるのは相手と ctx だけである。
//
// **端末は要求しない。** 要求すれば出力に画面制御が混ざり、集めた側が読めなく
// なる。端末が要る用（`sudo` など）は、この入口の担当ではない。
//
// **何も尋ねない。** Prompter を渡さないので、保存済みの資格情報で通らない接続は
// そこで失敗する。未知のホスト鍵も受け入れない——無人で走る操作が信頼を増やして
// はならない。保存済みの鍵パスフレーズは使われる（あれは尋ねる前に試される）が、
// 保存済みのアカウントパスワードは使われない。password 方式そのものが、尋ねる
// 相手が居ないときは提示されないからである。
//
// 返る error は、コマンドが失敗したことではなく **走らせられなかったこと** を
// 表す。コマンドが 1 で終わったのは (1, nil) である。
func (d Dialer) Stream(
	ctx context.Context, target Target, command string, streams Streams,
) (int, error) {
	if command == "" {
		return RemoteFailureExit, errors.New("no remote command was given")
	}
	// 未知のホストを黙って受け入れない。Run と同じ理由である。
	strict := target
	strict.Strict = "yes"

	client, closers, err := d.chain(ctx, strict, nil)
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

	// **ctx が終わったらセッションを閉じる。** 閉じなければ Run は相手が終わる
	// まで返らず、Ctrl-C を押した人が待たされ続ける。
	finished := make(chan struct{})
	defer close(finished)

	// **設定された ServerAliveInterval を落とさない。** 対話セッションはこれを
	// 尊重していて、こちらだけ無視していた——長く黙って走るコマンドこそ、
	// 途中の機器に接続を捨てられて困る側である。既定を作りはしない（OpenSSH も
	// 既定では送らない）。設定した人の指示を通すだけである。
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
		// 終了コードは結果であって失敗ではない。相手が答えたのだから、その答えを返す。
		return exit.ExitStatus(), nil
	}
	var missing *ssh.ExitMissingError
	if errors.As(runErr, &missing) {
		return RemoteFailureExit, fmt.Errorf("the remote command ended without a status: %w", runErr)
	}
	return RemoteFailureExit, runErr
}
