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
// **端末は要求しない。** 引数も無い——コマンドはリモートのシェルが 1 本の
// 文字列として受け取る。これは OpenSSH の `ssh host 'command'` と同じであり、
// 呼び出し側が組み立てるのは自分が書いた定数だけである。
//
// **何も尋ねない。** Prompter を渡さないので、保存済みの資格情報で通らない
// 接続はそこで失敗する。無人で走る操作が人を待つことはない。
//
// **Stream と違うところが 2 つあり、どちらも意図である。**
//
// keepalive を張らない。ここが走らせるのは上限時間の中で終わる短い操作なので、
// 「相手が黙ったまま生きているか」を確かめる必要が無い。Stream の方は人が開いた
// セッションで、いつまで続くか分からないから要る。
//
// 設定の `SetEnv` を送らない。**ここは出力を読むからである** ——このアプリケーション
// が書いた定型のプログラムを走らせ、その標準出力を答えとして解析する。利用者が
// 設定に書いた環境変数が LANG や LC_ALL を変えれば、読む相手の言葉が変わりうる。
// Stream の方は出力を人が読むので、利用者の設定がそのまま効くのが正しい。
func (d Dialer) Run(ctx context.Context, target Target, command string, stdin []byte) (Output, error) {
	started := time.Now()

	// 未知のホストを黙って受け入れない。無人の操作が信頼を増やしてはならない。
	strict := target
	strict.Strict = "yes"

	timeout := strict.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, closers, err := d.chain(ctx, strict, nil, nil)
	if err != nil {
		return Output{Elapsed: time.Since(started)}, err
	}
	defer func() {
		_ = client.Close()
		closeAll(closers)
	}()

	session, err := client.NewSession()
	if err != nil {
		return Output{Elapsed: time.Since(started)}, err
	}
	defer func() { _ = session.Close() }()

	var stdout, stderr cappedBuffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if len(stdin) > 0 {
		session.Stdin = bytes.NewReader(stdin)
	}

	output := Output{}
	runErr := session.Run(command)
	output.Stdout, output.Stderr = stdout.Bytes(), stderr.Bytes()
	output.Truncated = stdout.truncated || stderr.truncated
	output.Elapsed = time.Since(started)

	var exit *ssh.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exit):
		// 終了コードは結果であって失敗ではない。リモートが答えたのだから、
		// その答えを返す。
		output.ExitCode = exit.ExitStatus()
	default:
		return output, runErr
	}
	return output, nil
}

// cappedBuffer は、上限まで書き込みを受け、それ以降は捨てる。
//
// **書き手にエラーを返さない。** 返せば、上限に達したことがコマンドの失敗として
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
