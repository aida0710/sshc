// Package terminal は、このアプリケーション自身の中で開かれる端末セッションを持つ。
package terminal

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"sshc/internal/iowrite"
)

// Kind はセッションの種類を表す。
type Kind string

const (
	KindSSH   Kind = "ssh"
	KindShell Kind = "shell"
)

func ValidKind(kind Kind) bool { return kind == KindSSH || kind == KindShell }

// 転送の種類。
const (
	ForwardLocal   = "local"
	ForwardRemote  = "remote"
	ForwardDynamic = "dynamic"
	ForwardAgent   = "agent"
)

// 転送を開けなかった理由の語。画面は語を言語ごとの文にする。Go のエラーの文は、
// 設定から開いた転送ではターミナルへ書く 1 行に、一時転送では API の detail に残す。
const (
	ForwardProblemAddressInUse     = "address_in_use"
	ForwardProblemPermissionDenied = "permission_denied"
	ForwardProblemAgentUnreachable = "agent_unreachable"
	ForwardProblemRemoteDenied     = "remote_denied"
	ForwardProblemRemoteTimeout    = "remote_timeout"
	ForwardProblemFailed           = "failed"
)

// Forward は、そのセッションが開いている転送ひとつである。
type Forward struct {
	// ID は同じセッション内で一意な、停止操作用の不透明な識別子。
	ID   string
	Kind string
	// Listen は待ち受け場所。remote はSSH接続先、それ以外はこのマシン。agent では空。
	Listen string
	// To は、その先。dynamic と agent では空。
	To string
	// Problem は、開けなかった理由の語（ForwardProblem*）。空なら開いている。
	Problem string
	// Temporary は接続後に追加され、設定ファイルには由来しない転送を表す。
	Temporary bool
}

// Forwarder は、そのセッションが開いている転送を報告する。
type Forwarder interface{ Forwards() []Forward }

// ForwardController は接続済みSSHセッション上の一時ポート転送を操作する。
// 実装しないローカルshellや接続途中のprocessへは公開しない。
type ForwardController interface {
	StartForward(kind, listenPort, destination string) (Forward, error)
	StopForward(id string) error
}

// DirectoryReporter は、端末でいま前面にいるプログラムの作業ディレクトリを答える
// Process である。プログラムが出力した相対パスは、このディレクトリから見たもので
// ある。答えられるのは Linux の擬似端末だけで、SSH の接続先のものは分からない。
type DirectoryReporter interface {
	ForegroundDirectory() (string, error)
}

// ForegroundReporter は、シェルが端末の前面で動かしているプログラムを答える Process
// である。答えられるのは Linux と macOS の擬似端末だけで、SSH の接続先で何が動いて
// いるかは分からない。
type ForegroundReporter interface {
	// ForegroundProgram は、前面のプログラムを返す。シェル自身が前面にいるときと、
	// 分からないときは false を返す。
	ForegroundProgram() (ForegroundProgram, bool)
}

// ForegroundProgram は、シェルが前面で動かしているプログラムである。Group はその
// プロセスグループの番号で、タイトルをそのプログラムが送ったかを見分けるのに使う。
// Running は、そのプログラムが動き始めてからの長さである。
type ForegroundProgram struct {
	Group   int
	Name    string
	Running time.Duration
}

// Readier は、非同期に開いた Process が、使えるようになるための接続の作業を終えたことを知らせる。これを
// 持たない Process は、Open が戻った時点で使える。SSH のセッションがこれを実装するのは、握手が終わる前に
// 認証の問いを流さなければならないためである。
//
// Ready は、準備が終わると閉じる。値は送らないので、何人でも待てる。結果は
// ReadyErr で読む（nil なら使える状態になった）。Ready が閉じる前の ReadyErr は nil。
type Readier interface {
	Ready() <-chan struct{}
	ReadyErr() error
}

// Promptingは、Ready前でも利用者の入力を認証promptへの回答として受け取れる
// Processである。これを実装しない非同期Processへの先行入力は捨てる。
type Prompting interface{ AwaitingPrompt() bool }

// ConnectionProgress は、非同期に開いた SSH の接続が、今どの段階にいるかを伝える。State とはわざと分けて
// ある。State はターミナルを使えるかを表し、この値は、接続中の SSH の輸送が今していることを説明する。
type ConnectionProgress struct {
	Phase    string
	Alias    string
	HostName string
	User     string
	Hop      int
	Hops     int
	// HostKey は、この経由先のホストが出した鍵である。確かめているあいだ（Phase が ConnectionHostKey）入る。
	// 知らない鍵は、これを添えて尋ねる（HostKeyQuestion）。
	HostKey *OfferedHostKey
}

// OfferedHostKey は、人が見比べる形のホスト鍵である。種類と SHA256 の fingerprint を持つ。
type OfferedHostKey struct {
	Type        string
	Fingerprint string
}

// HostKeyQuestion は、接続中の SSH のセッションが答えを待っている問いである。known_hosts に無いホスト鍵を
// 信頼するかを尋ねる。ターミナルは言葉でも尋ね、画面はボタンを添えて出す。
type HostKeyQuestion struct {
	Alias       string
	HostName    string
	KeyType     string
	Fingerprint string
}

// HostKeyQuestionAbout は、接続が待っている問いを返す。接続がホスト鍵を確かめていて（この進み具合）、
// 待っている yes/no の問い（openQuestion。sshclient.UnknownHostPrompt の言葉）がその鍵についてのもので
// あるときだけ返す。答え終えた問いや、ほかの経由先の鍵についての問いは、もう出さない。ターミナルも SFTP の
// タブも、この形で尋ねる。
func (p ConnectionProgress) HostKeyQuestionAbout(openQuestion string) (HostKeyQuestion, bool) {
	if p.Phase != ConnectionHostKey || p.HostKey == nil || !strings.Contains(openQuestion, p.HostKey.Fingerprint) {
		return HostKeyQuestion{}, false
	}
	return HostKeyQuestion{
		Alias: p.Alias, HostName: p.HostName, KeyType: p.HostKey.Type, Fingerprint: p.HostKey.Fingerprint,
	}, true
}

// HostKeyQuestioning は、待っている問いを伝えられ、打ち込みではなくボタンから答えを受け取れる SSH の
// process が実装する。
type HostKeyQuestioning interface {
	// HostKeyQuestion は、開いている問いがあれば返す。
	HostKeyQuestion() (HostKeyQuestion, bool)
	// AnswerHostKey は、この fingerprint の鍵についての問いがまだ開いていれば、それに答えて true を返す。
	// 開いていなければ何もしない。
	AnswerHostKey(fingerprint string, accept bool) bool
}

// TypedPassword は、利用者がこのセッションのターミナルで打ち、認証が通ったアカウントの
// パスワードである。Vault に保存するかの答えが出るまでエンジンのメモリにだけある。画面へも
// ログへも出さない。一覧（View）は、問いがあることだけを言う。
type TypedPassword struct {
	Alias string
	// Binding は、打ったときの認証先の digest（sshclient.Target.AuthenticationBinding）である。
	// 答えるまでに接続先の設定が変わっていたら、別の相手のパスワードとして保存しない。
	Binding  string
	Password string
}

// PasswordSaveAsking は、利用者が打ち込んだパスワードを、保存するかの答えが出るまで持つ SSH の process が
// 実装する。
type PasswordSaveAsking interface {
	// TypedPassword は、答えを待っているパスワードがあれば返す。
	TypedPassword() (TypedPassword, bool)
	// ForgetTypedPassword は、それを手放す。
	ForgetTypedPassword()
}

const (
	ConnectionDialing        = "dialing"
	ConnectionHostKey        = "host_key"
	ConnectionAuthenticating = "authenticating"
	ConnectionAuthenticated  = "authenticated"
	ConnectionOpeningSession = "opening_session"
)

// Progressing は、接続の今の段階を説明できる SSH の process が実装する。ローカルのシェルは同期でつながる
// ので、この追加の状態は要らない。
type Progressing interface{ ConnectionProgress() ConnectionProgress }

// Announcer は、接続ログの設定に関係なく出す行（[sshc] で始まる）を、そのセッションの
// ターミナルへ 1 行書ける Process である。接続の後で、利用者の操作が要ることを
// そのターミナルで知らせるために使う。
type Announcer interface{ Announce(message string) }

// ExactInput は、入力のまとまり1つを丸ごと受け取るか、エラーを返す。黙ってバイトを落とすことは無い。対話の
// SSH は、これを Process.Write とは別に実装する。Process.Write は打鍵のためのもので、入力のバッファが
// いっぱいのときに落とすことをわざと許している。
type ExactInput interface {
	WriteExact(context.Context, []byte) error
}

// DefaultTerminalType は、sshc のターミナルが名乗る種別（TERM の値）である。
//
// 画面を描くのは xterm.js なので、その能力に合った名前にする。SSH の PTY の
// 要求、ローカルシェルの TERM、Telnet の TERMINAL-TYPE がどれもこれを名乗る。
// 名前を変えるときはここだけを変える。1か所でもずれると、vim や less が
// 接続の種類ごとに違う制御列を送ってくる。
const DefaultTerminalType = "xterm-256color"

// ColorTerm は、sshc のターミナルが 24 ビット色を描けることを知らせる COLORTERM の値である。
// TERM の xterm-256color は 256 色までしか言わないので、vim・bat・delta などはこれを見て
// 真色を選ぶ。
const ColorTerm = "truecolor"

// TerminalProgram は、中のプログラムが sshc のターミナルを名前で見分ける TERM_PROGRAM の値である。
const TerminalProgram = "sshc"

// Size はターミナルの桁数と行数である。
type Size struct {
	Cols uint16
	Rows uint16
}

// DefaultSize は、ターミナルの大きさが分からないときに使う値を返す。
//
// ブラウザが大きさを送ってこないときや、パイプの中で走っていて問い合わせられ
// ないときがそれである。80x24 は、それでも読める既定として長く使われてきた
// 寸法であり、別の数を選ぶ理由が無い。受け取った側が書き換えても、ほかの
// パッケージの既定が変わらないよう、呼ぶたびに新しい値を返す。
func DefaultSize() Size {
	return Size{Cols: 80, Rows: 24}
}

// 端末の大きさの上限。ブラウザが送ってくる値であり、TIOCSWINSZ へそのまま渡る。
const (
	MaxCols = 1000
	MaxRows = 1000
)

// Valid は、TIOCSWINSZ へ渡してよい大きさかどうかを返す。
func (size Size) Valid() bool {
	return size.Cols > 0 && size.Rows > 0 && size.Cols <= MaxCols && size.Rows <= MaxRows
}

// ExitInfo は、子プロセスが終わった理由である。
type ExitInfo struct {
	Code   int
	Signal string
	At     time.Time
	// TransportLost は、シェルが終わったのではなく、その下の輸送（SSH の接続）が
	// 落ちたことを表す。自動再接続はこれを見て繋ぎ直す。終了コードの値には
	// この意味を載せない。
	TransportLost bool
	// TransportLossReason は、輸送が落ちた理由の元の文である。`sshc ssh` が出す。画面の
	// ターミナルは再接続の行で切れたことを言うので、接続ログの「詳しく」にだけ残す。
	TransportLossReason string
	// Notice は、終わり方について sshc がターミナルへ書く文である（接続ログの深さを
	// 上げたときの keepalive の上限や終了状態の行など）。
	// Process はこれを出力へ書かずに返す。出力の終わりに書くと、終わったプログラムの
	// 代替画面に書かれ、engine がモードを戻したときに見えなくなる。
	Notice string
}

// State は、SSH process の接続ライフサイクルである。WebSocket の接続状態とは
// 別物であり、ブラウザが外れても process が生きていれば connected のままである。
type State string

const (
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
	StateReconnecting State = "reconnecting"
	StateExited       State = "exited"
)

// ReconnectView は、次の再接続試行を画面へ説明するための公開情報である。
// raw error は含めない。Problem は画面が翻訳する固定コードだけを持つ。
type ReconnectView struct {
	Attempt int
	Limit   int
	RetryAt time.Time
	Problem string
	// WaitingForNetwork は、回線が無いので待っていることを表す。待つ間は回数に数えない。
	WaitingForNetwork bool
}

// ExitCodeUnknown は、終了コードを得られなかったときの Code である。輸送が
// 落ちたのか、プロセスの待ち方が失敗したのかは TransportLost で区別する。
const ExitCodeUnknown = -1

// Command は、PTY の中で起動するプログラムひとつである。
type Command struct {
	// Path は絶対的なプログラムパス。
	Path  string
	Argv0 string
	// Arguments は argv の残りであり、argv[0] は含まない。
	Arguments []string
	// Env は子プロセスの完全な環境である。nil はこのプロセスの環境の継承を意味する。
	Env []string
	// Dir は作業ディレクトリ。空ならこのプロセスのものを継ぐ。
	Dir string
}

// Process は確保済み PTY と子プロセスを操作する。
type Process interface {
	io.ReadWriter
	// Resize は TIOCSWINSZ を発行する。
	Resize(Size) error
	// Hangup は、このセッションの木に終わってほしいという意思である。
	Hangup() error
	// ForceClose は、相手の応答を待たずにこのセッションの木を止めて閉じる。
	// 利用者が閉じたときと engine の停止は、Hangup ではなくこれで止める。
	ForceClose() error
	// Wait は子プロセスの終了を待ち、その理由を返す。
	Wait() ExitInfo
	// Close は PTY を解放する。
	Close() error
}

// Starter は PTY を確保して子プロセスを起動する。
type Starter interface {
	Start(ctx context.Context, command Command, size Size) (Process, error)
}

var (
	// ErrSessionLimit は、上限に達した状態で開こうとした要求を拒否する。
	ErrSessionLimit = errors.New("the terminal session limit has been reached")
	// ErrNotFound は、そのセッションが存在しないことを報告する。
	ErrNotFound = errors.New("no such terminal session")
	// ErrExited は、終了済みのセッションへ書こうとしたことを報告する。
	ErrExited = errors.New("the terminal session has exited")
	// ErrReconnectUnavailable は、実行中または開き方を保持しないセッションの
	// 明示再接続を拒否したことを報告する。
	ErrReconnectUnavailable = errors.New("the terminal session cannot be reconnected")
	// ErrNoPasswordSaveQuestion は、打ったパスワードを保存するかを尋ねていないセッションに
	// 答えようとしたことを表す。
	ErrNoPasswordSaveQuestion = errors.New("the terminal session is not asking about saving a password")
	// ErrNoHostKeyQuestion は、その鍵についての問いを待っていないセッションに答えようとした
	// ことを表す。答える前に、利用者がターミナルで答えたか、接続が先へ進んだか、終わった。
	ErrNoHostKeyQuestion = errors.New("the terminal session is not asking about this host key")
	// ErrNotReconnecting は、自動再接続の待機中でないセッションを止めようとした
	// ことを報告する。
	ErrNotReconnecting = errors.New("the terminal session is not reconnecting")
	// ErrNoStarter は、PTY を確保する手段が配線されていないことを報告する。
	ErrNoStarter = errors.New("no pseudo-terminal is available")
	// ErrInvalidSize は、TIOCSWINSZ へ渡せない大きさを拒否する。
	ErrInvalidSize = errors.New("the terminal size is out of range")
	// ErrInvalidTitle は、一覧に出せない名前を拒否する。
	ErrInvalidTitle          = errors.New("that is not a usable session name")
	ErrNotConnected          = errors.New("the terminal session is not connected")
	ErrGenerationChanged     = errors.New("the terminal session changed after preview")
	ErrExactInputUnavailable = errors.New("the terminal process cannot accept exact input")
	ErrCommandTooLarge       = errors.New("the terminal command is too large")
	ErrUnsafeInsert          = errors.New("the terminal command contains control input that cannot be inserted safely")
	ErrForwardUnavailable    = errors.New("the terminal process cannot manage port forwards")
	ErrDirectoryUnavailable  = errors.New("the terminal process cannot report its working directory")
	ErrForwardNotFound       = errors.New("no such terminal port forward")
	ErrInvalidForward        = errors.New("the terminal port forward is invalid")
	ErrShuttingDown          = errors.New("the terminal registry is shutting down")
)

// writeExact は、取り消されていなければ input を残らず PTY へ書く。PTY の *os.File は
// 1 回の Write で書き切るか失敗するので、書いている途中で ctx を見直す必要はない。
func writeExact(ctx context.Context, writer io.Writer, input []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return iowrite.WriteAll(writer, input)
}

// MaxCommandBytes は、コマンドを実行する復帰（CR）の1バイトを残した長さである。長さに上限のある SSH の
// 入力の待ち行列にも、ローカルの PTY の小さなまとまりにも収まる。
const MaxCommandBytes = (4 << 10) - 1

// MaxTitle は、一覧に出す名前の長さの上限である。
const MaxTitle = 64

// CleanTitle は、一覧に出してよい名前だけを通す。
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
	// MaxSessions は同時に実行できるセッション数。終了済みは数えない。
	MaxSessions int
	// Scrollback は 1 セッションあたりのリングバッファの大きさ（バイト）。
	Scrollback int
}

const (
	DefaultMaxSessions = 50
	MinMaxSessions     = 1
	MaxMaxSessions     = 200

	DefaultScrollback             = 256 << 10
	MinScrollback                 = 16 << 10
	MaxScrollback                 = 4 << 20
	MinBrowserScrollbackLines     = 1000
	MaxBrowserScrollbackLines     = 100000
	DefaultBrowserScrollbackLines = 5000
	// 裏に残すターミナルの数の範囲と既定。1つごとにブラウザのスクロールバックを
	// メモリに持つ（5,000 行で数 MB）。
	MinRetainedTerminals     = 1
	MaxRetainedTerminals     = 99
	DefaultRetainedTerminals = 3

	// 字の大きさは、このプロセスが何かに使う値ではない。PTY は px を知らない。
	MinFontSize = 8
	MaxFontSize = 32

	// RetainedExited は、終了済みセッションを一覧に残す本数の上限である。
	RetainedExited = 20
)

func DefaultLimits() Limits {
	return Limits{MaxSessions: DefaultMaxSessions, Scrollback: DefaultScrollback}
}

// Normalise は、範囲の外にある値を既定へ戻す。
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
