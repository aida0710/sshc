package platform

import (
	"context"
	"errors"
	"time"
)

// MaxCapturedOutput は、外部コマンド一回について、片方のストリームをメモリに
// どれだけ保持するかの上限。
const MaxCapturedOutput = 64 << 10

var (
	// ErrTimedOut は、コマンドがタイムアウト内に終わらず kill されたことを報告
	// する。
	ErrTimedOut = errors.New("command did not finish before its timeout")
	// ErrProgramPathNotAbsolute は、そうしなければ PATH 経由で探されることになる
	// プログラムを拒否する。
	ErrProgramPathNotAbsolute = errors.New("program path must be absolute")
)

// Command は外部プロセスひとつ。
//
// Path は絶対的なプログラムパスで、Arguments はその argv の残り。コマンドライン
// のためのフィールドはない。このアプリケーションはコマンドラインを組み立てない
// からだ。ここにあるものがシェルに解釈されることは決してない。
type Command struct {
	Path      string
	Arguments []string
	// Timeout は、それを超えたときプロセスを kill する。0 は、呼び出し側の context
	// だけが上限であることを意味する。
	Timeout time.Duration
	// Env は子プロセスの完全な環境。nil の Env はこのプロセスの環境を継承し、nil で
	// ない Env は、たとえ空でもそれを完全に置き換える。
	Env []string
}

// Output は、外部コマンド一回の上限付きの結果。非ゼロの終了ステータスはエラー
// ではなくデータである。err == nil のまま ExitCode に報告される。
type Output struct {
	Stdout    []byte
	Stderr    []byte
	ExitCode  int
	Truncated bool
	Stopped   bool
	Elapsed   time.Duration
}

// OutputRunner は外部プログラムを一回実行し、上限付きの出力を返す。
type OutputRunner interface {
	RunOutput(ctx context.Context, command Command) (Output, error)
}

// Toolchain は、このマシンにインストールされた OpenSSH のプログラムを見つける。
//
// **残っているのは ssh-keygen だけである。** 接続も、agent への登録も、
// アルゴリズムの一覧も、このプロセスの中で行う。ssh-keygen が残るのは、
// ハードウェアトークンとのやり取り（PIN、タッチ、libfido2）が x/crypto に
// 無いからである。
//
// このアプリケーションが実行してよいプログラムはすべてここに名前があるので、
// 呼び出し側が自前でプログラムパスを組み立てることは決してできない。必要なものを
// 要求し、絶対パスかエラーを受け取る。
type Toolchain interface {
	KeyGen() (string, error)
}
