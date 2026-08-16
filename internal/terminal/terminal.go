// Package terminal は、このアプリケーション自身の中で開かれる端末セッションを持つ。
//
// 持っているのはセッションのレジストリと PTY のライフサイクルだけである。ssh の
// コマンドラインを組み立てるのは internal/platform であり、このパッケージは
// 「与えられた argv と環境で PTY を起こし、読み書きし、後始末する」以上のことを
// 知らない。だから ssh と、ユーザーのログインシェルは、ここでは同じものである。
package terminal

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Kind は、そのセッションの向こうにいるものである。
//
// これは表示上の区別ではない。"ssh" は alias を持ち、"shell" は持たない。
// localhost はローカルシェルであって ssh 接続ではないので、接続の一覧には
// 現れない。あの一覧は ~/.ssh/config の投影であり、localhost はそこに存在しない。
type Kind string

const (
	KindSSH   Kind = "ssh"
	KindShell Kind = "shell"
)

func ValidKind(kind Kind) bool { return kind == KindSSH || kind == KindShell }

// 転送の種類。
const (
	ForwardLocal   = "local"
	ForwardDynamic = "dynamic"
	ForwardAgent   = "agent"
)

// Forward は、そのセッションが開いている転送ひとつである。
//
// **このパッケージは SSH を知らない。** 型だけを置き、開くのは
// internal/sshclient である——Spec.Open と同じ理由による。
type Forward struct {
	Kind string
	// Listen は、このマシンで開いている場所。agent 転送では空。
	Listen string
	// To は、その先。dynamic と agent では空。
	To string
	// Problem は、開けなかった理由。空なら開いている。
	Problem string
}

// Forwarder は、そのセッションが開いている転送を報告する。
//
// Process が満たしていなくてよい。**ローカルシェルは何も転送しない。**
type Forwarder interface{ Forwards() []Forward }

// Size は端末の桁数と行数である。
//
// これが無いと、全画面を使うプログラム（vim、top、less）が壊れた幅で描画する。
// だからサイズは、あとから足す設定ではなく、セッションを開く要求の一部である。
type Size struct {
	Cols uint16
	Rows uint16
}

// 端末の大きさの上限。ブラウザが送ってくる値であり、TIOCSWINSZ へそのまま渡る。
const (
	MaxCols = 1000
	MaxRows = 1000
)

// Valid は、TIOCSWINSZ へ渡してよい大きさかどうかを答える。
func (size Size) Valid() bool {
	return size.Cols > 0 && size.Rows > 0 && size.Cols <= MaxCols && size.Rows <= MaxRows
}

// ExitInfo は、子プロセスが終わった理由である。
//
// これが記録されたセッションは一覧に残る。ssh が接続できなかった理由が読めるのは、
// そこだけだからだ。
type ExitInfo struct {
	Code   int
	Signal string
	At     time.Time
}

// Command は、PTY の中で起こすプログラムひとつである。
//
// コマンドラインのためのフィールドはない。このパッケージはコマンドラインを
// 組み立てないし、ここにあるものがシェルに解釈されることも決してない。
type Command struct {
	// Path は絶対的なプログラムパス。
	Path string
	// Argv0 は argv[0] である。空なら Path が使われる。ログインシェルを
	// 「-zsh」として起こすためだけに存在する——先頭のハイフンが、そのシェルに
	// ログインシェルとしての起動を伝える唯一の手段だからである。
	Argv0 string
	// Arguments は argv の残りであり、argv[0] は含まない。
	Arguments []string
	// Env は子プロセスの完全な環境である。nil はこのプロセスの環境の継承を意味する。
	Env []string
	// Dir は作業ディレクトリ。空ならこのプロセスのものを継ぐ。
	Dir string
}

// Process は、確保された PTY と、その向こうにいる子プロセスひとつである。
//
// インターフェースなのは、レジストリの上にあるものすべてを実プロセスなしで
// 検査できるようにするためである。実 PTY を使う検査は 1 本だけ置いてある。
type Process interface {
	io.ReadWriter
	// Resize は TIOCSWINSZ を発行する。
	Resize(Size) error
	// Hangup は、このセッションの木に終わってほしいという意思である。
	//
	// **要求であって、強制ではない。** Unix はセッションのプロセスグループへ
	// SIGHUP を送り、Windows は擬似コンソールを閉じる。どちらも「もう端末が
	// 無い」と伝えるだけで、聞かない相手を終わらせはしない。それを行うのは
	// 締切に達したときの強制停止である。
	//
	// **待ってはならない。** 返らない一本が、他のすべての停止と締切そのものを
	// 止めてしまう。
	Hangup() error
	// Wait は子プロセスの終了を待ち、その理由を返す。
	Wait() ExitInfo
	// Close は PTY を手放す。
	Close() error
}

// Starter は PTY を確保して子プロセスを起こす。
//
// context は**確保そのものだけ**を支配する。返った Process はそこから切り離された
// 寿命を持つ——さもなければ、開いた HTTP ハンドラが返った瞬間にセッションが死ぬ。
type Starter interface {
	Start(ctx context.Context, command Command, size Size) (Process, error)
}

// forceCloser は、graceful な Hangup が返らなくても下の輸送を断てる Process が
// 満たす、任意の追加契約である。
//
// **Process そのものは広げない。** これを実装するのは、切るべき OS の資源を
// 持っているもの——Unix の PTY のプロセスグループと、プロセス内 SSH のホップ——
// だけであり、持たない実装に空のメソッドを配って回る理由が無い。
//
// メソッドが公開なのは、実装が別のパッケージに居るからである。型は公開しない。
type forceCloser interface {
	ForceClose() error
}

var (
	// ErrSessionLimit は、上限に達した状態で開こうとした要求を拒否する。
	// 黙って古いセッションを閉じることはしない。
	ErrSessionLimit = errors.New("the terminal session limit has been reached")
	// ErrNotFound は、そのセッションが存在しないことを報告する。
	ErrNotFound = errors.New("no such terminal session")
	// ErrExited は、終了済みのセッションへ書こうとしたことを報告する。
	ErrExited = errors.New("the terminal session has exited")
	// ErrNoStarter は、PTY を確保する手段が配線されていないことを報告する。
	ErrNoStarter = errors.New("no pseudo-terminal is available")
	// ErrInvalidSize は、TIOCSWINSZ へ渡せない大きさを拒否する。
	ErrInvalidSize = errors.New("the terminal size is out of range")
	// ErrInvalidTitle は、一覧に出せない名前を拒否する。
	ErrInvalidTitle = errors.New("that is not a usable session name")
	// ErrShuttingDown は、停止を始めたレジストリが新しいセッションを断ることを
	// 報告する。**停止中に開けてはならない。** 開けば、停止処理が数え終えた後に
	// 生きたセッションが残る。
	ErrShuttingDown = errors.New("the terminal registry is shutting down")
)

// MaxTitle は、一覧に出す名前の長さの上限である。
//
// 一覧はナビゲーションの幅しか持たないので、長い名前は切り詰められて読めなく
// なる。切り詰めて受け取るのではなく拒否するのは、保存された値と表示される値が
// 違う状態を作らないためである。
const MaxTitle = 64

// CleanTitle は、一覧に出してよい名前だけを通す。
//
// 前後の空白は落とす。空になるもの、制御文字を含むもの、上限を超えるものは
// 拒否する。制御文字を拒むのは、この文字列がそのまま画面へ出るからであり、
// エスケープ列を名前に持たせる用途はひとつも無いからである。
func CleanTitle(title string) (string, error) {
	cleaned := strings.TrimSpace(title)
	if cleaned == "" || utf8.RuneCountInString(cleaned) > MaxTitle {
		return "", ErrInvalidTitle
	}
	for _, character := range cleaned {
		if character == utf8.RuneError || unicode.IsControl(character) {
			return "", ErrInvalidTitle
		}
	}
	return cleaned, nil
}

// Limits は、metadata が運ぶ埋め込みターミナルの上限である。
type Limits struct {
	// MaxSessions は同時に生きていられるセッションの本数。終了済みは数えない。
	MaxSessions int
	// Scrollback は 1 セッションあたりのリングバッファの大きさ（バイト）。
	Scrollback int
}

// 上限の既定値と範囲。metadata の検証と、読み取り時の既定への差し戻しが、
// 二箇所で別の数を持たないよう、ここが唯一の一覧である。
const (
	DefaultMaxSessions = 50
	MinMaxSessions     = 1
	MaxMaxSessions     = 200

	DefaultScrollback = 256 << 10
	MinScrollback     = 16 << 10
	MaxScrollback     = 4 << 20

	// 字の大きさは、このプロセスが何かに使う値ではない——PTY は px を知らない。
	// **それでもここに範囲を置く。** 保存されるのは metadata であり、metadata の
	// 妥当な範囲を決める場所はここ 1 つだけだからである。
	MinFontSize = 8
	MaxFontSize = 32

	// RetainedExited は、終了済みセッションを一覧に残す本数の上限である。
	// 最後の出力を読めるように残すのであって、生存上限には数えない。
	RetainedExited = 20
)

func DefaultLimits() Limits {
	return Limits{MaxSessions: DefaultMaxSessions, Scrollback: DefaultScrollback}
}

// Normalise は、範囲の外にある値を既定へ戻す。
//
// 拒否ではなく差し戻しなのは、これが読み取り側だからである。手で書かれた
// metadata の数字ひとつが、色もタグもお気に入りも道連れに読めなくしてよい理由はない。
func (limits Limits) Normalise() Limits {
	normalised := DefaultLimits()
	if limits.MaxSessions >= MinMaxSessions && limits.MaxSessions <= MaxMaxSessions {
		normalised.MaxSessions = limits.MaxSessions
	}
	if limits.Scrollback >= MinScrollback && limits.Scrollback <= MaxScrollback {
		normalised.Scrollback = limits.Scrollback
	}
	return normalised
}
