package sshclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"golang.org/x/crypto/ssh"
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

// Run は、リモートで 1 つのコマンドを走らせる。
//
// 端末は要求しない。引数も無い。コマンドはリモートのシェルが 1 本の
// 文字列として受け取る。これは OpenSSH の `ssh host 'command'` と同じであり、
// 呼び出し側が組み立てるのは自分が書いた定数だけである。
//
// 保存済み資格情報は対話接続と同じ認証経路で使用するが、追加質問は拒否する。
// 保存済みの結果で通らない接続はそこで失敗し、ユーザー入力を待たない。
//
// 制限時間内で終わる管理操作を対象とするため keepalive は送らない。結果の解析が
// ロケールに依存しないよう、ssh_config の SetEnv も送らない。
func (d Dialer) Run(ctx context.Context, target Target, command string, stdin []byte) (Output, error) {
	started := time.Now()
	failed := func() Output {
		return Output{ExitCode: RemoteFailureExit, Elapsed: time.Since(started)}
	}

	// 非対話処理では未知のホストを信頼済みに変更しない。
	strict := requireKnownHosts(target)

	timeout := strict.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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

	var stdout, stderr cappedBuffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if len(stdin) > 0 {
		session.Stdin = bytes.NewReader(stdin)
	}

	// Run は session.Run の最中にも ctx の所有下にある。チャンネルだけでなく
	// 輸送も閉じるのは、応答しない相手の Close を待たずに解除するためである。
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Close()
			closeAll(closers)
			_ = session.Close()
		case <-finished:
		}
	}()

	output := Output{ExitCode: RemoteFailureExit}
	runErr := session.Run(command)
	output.Stdout, output.Stderr = stdout.Bytes(), stderr.Bytes()
	output.Truncated = stdout.truncated || stderr.truncated
	output.Elapsed = time.Since(started)
	if cause := ctx.Err(); cause != nil {
		return output, cause
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

// cappedBuffer は、上限まで書き込みを受け、それ以降は捨てる。
//
// 書き手にエラーを返さない。返せば、上限に達したことがコマンドの失敗として
// 伝わってしまう。切り詰めたという事実は truncated が運ぶ。
type cappedBuffer struct {
	buffer    bytes.Buffer
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := MaxCapturedOutput - b.buffer.Len(); room > 0 {
		if len(p) <= room {
			return b.buffer.Write(p)
		}
		if _, err := b.buffer.Write(p[:room]); err != nil {
			return 0, err
		}
	}
	b.truncated = true
	return len(p), nil
}

func (b *cappedBuffer) Bytes() []byte { return b.buffer.Bytes() }

var _ io.Writer = (*cappedBuffer)(nil)
