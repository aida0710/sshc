package sshclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/iowrite"
)

// MaxCapturedOutput は、リモートのコマンドから取り込む量の上限である。
//
// 相手が延々と喋る可能性は常にある。取り込む量に上限が無ければ、こちらの
// メモリがその上限になる。
const MaxCapturedOutput = 64 << 10

// Output は、リモートで走らせたコマンドひとつの結果である。
type Output struct {
	Stdout    []byte
	Stderr    []byte
	ExitCode  int
	Truncated bool
	Elapsed   time.Duration
}

// Command は、Run がリモートで走らせるコマンドひとつである。
type Command struct {
	// Line は、リモートのシェルが 1 本の文字列として受け取るコマンドである。
	Line string
	// Stdin は、コマンドの標準入力へ渡す内容である。空なら何も渡さない。
	Stdin []byte
	// Limit は、コマンドを走らせてから終わるまでの上限である。0 なら上限を置かず、
	// ctx の取り消しだけで止まる。接続の確立には、ホップごとに ConnectTimeout が掛かる。
	Limit time.Duration
}

// ErrCommandTimedOut は、コマンドが Command.Limit までに終わらなかったことを表す。
// 呼び出し側が期限切れとして扱えるよう、context.DeadlineExceeded として判定できる。
var ErrCommandTimedOut = fmt.Errorf("the remote command did not finish within its time limit: %w", context.DeadlineExceeded)

// Run は、リモートで 1 つのコマンドを走らせる。
//
// 端末は要求しない。引数も無い。コマンドはリモートのシェルが 1 本の
// 文字列として受け取る。これは OpenSSH の `ssh host 'command'` と同じである。
//
// 保存済み資格情報は対話接続と同じ認証経路で使用するが、追加質問は拒否する。
// 保存済みの結果で通らない接続はそこで失敗し、ユーザー入力を待たない。
//
// keepalive は ServerAliveInterval を設定した接続にだけ送る。上限を置かずに長く
// 走るコマンドこそ、途中の機器に接続を捨てられて困るからである。結果の解析が
// ロケールに依存しないよう、ssh_config の SetEnv は送らない。
func (d Dialer) Run(ctx context.Context, target Target, command Command) (Output, error) {
	started := time.Now()
	failed := func() Output {
		return Output{ExitCode: RemoteFailureExit, Elapsed: time.Since(started)}
	}

	// 非対話処理では未知のホストを信頼済みに変更しない。
	strict := requireKnownHosts(target)

	// ConnectTimeout は接続の確立だけに掛ける。chain がホップごとに掛けている。
	// ここで Run 全体に掛けると、接続の速い相手でも長いコマンドが途中で切られる。
	client, closers, err := d.chain(ctx, strict, noPrompt, nil)
	if err != nil {
		return failed(), err
	}
	defer func() {
		_ = client.Close()
		closeAll(closers)
	}()

	session, err := client.NewSession()
	if err != nil {
		return failed(), err
	}
	defer func() { _ = session.Close() }()

	// 上限を超えた分は捨て、書き手にはエラーを返さない。返せば、上限に達したことが
	// コマンドの失敗として伝わる。切り詰めたという事実は Output.Truncated が運ぶ。
	stdout, stderr := iowrite.NewCappedBuffer(MaxCapturedOutput), iowrite.NewCappedBuffer(MaxCapturedOutput)
	streams := Streams{Out: stdout, Err: stderr}
	if len(command.Stdin) > 0 {
		streams.In = bytes.NewReader(command.Stdin)
	}
	encoded, closeEncoding, err := encodeStreams(streams, strict.Encoding)
	if err != nil {
		return failed(), err
	}
	session.Stdin = encoded.In
	session.Stdout = encoded.Out
	session.Stderr = encoded.Err

	running, stop := ctx, context.CancelFunc(func() {})
	if command.Limit > 0 {
		running, stop = context.WithTimeout(ctx, command.Limit)
	}
	defer stop()

	// Run は session.Run の最中にも ctx の所有下にある。チャンネルだけでなく
	// 輸送も閉じるのは、応答しない相手の Close を待たずに解除するためである。
	finished := make(chan struct{})
	defer close(finished)
	if keepAlive := keepAliveLoop(client, keepAliveSettings{interval: strict.KeepAlive, count: strict.KeepAliveMax, done: finished}); keepAlive != nil {
		go keepAlive()
	}
	go func() {
		select {
		case <-running.Done():
			_ = client.Close()
			closeAll(closers)
			_ = session.Close()
		case <-finished:
		}
	}()

	output := Output{ExitCode: RemoteFailureExit}
	runErr := session.Run(command.Line)
	encodingErr := closeEncoding()
	output.Stdout, output.Stderr = stdout.Bytes(), stderr.Bytes()
	output.Truncated = stdout.Truncated() || stderr.Truncated()
	output.Elapsed = time.Since(started)
	if cause := ctx.Err(); cause != nil {
		return output, cause
	}
	if running.Err() != nil {
		return output, ErrCommandTimedOut
	}
	if runErr == nil && encodingErr != nil {
		return output, encodingErr
	}

	var exit *ssh.ExitError
	switch {
	case runErr == nil:
		output.ExitCode = 0
	case errors.As(runErr, &exit):
		// 終了コードは結果であって失敗ではない。リモートが応答したのだから、
		// その結果を返す。
		output.ExitCode = exit.ExitStatus()
	default:
		return output, runErr
	}
	return output, nil
}
