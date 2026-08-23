package app

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sshc/internal/application"
	"sshc/internal/effective"
	"sshc/internal/httpserver"
	"sshc/internal/knownhosts"
	"sshc/internal/secret"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

// errNoConfiguration は、設定を読む手段が配線されていないことを報告する。
var errNoConfiguration = errors.New("no configuration service is available")

// sshParts は、プロセス内で SSH を話すのに要るものの全体である。
//
// **組み立てる場所はここひとつである。** 対話セッションも、認証テストも、
// 公開鍵のリモート登録も、同じ鍵・同じ known_hosts・同じ解決器を使う。二箇所で
// 組み立てると、片方だけが vault を見る日が来る。
type sshParts struct {
	dialer  sshclient.Dialer
	resolve sshclient.Resolver
	home    string
}

// newSSHParts は、プロセス内 SSH の部品一式を組む。
//
// **保存済みの答えがどこから来るかは、ここでは決めない。** engine は vault に
// 訊き、`sshc <接続先>` は engine に訊いた結果を持って来る——出どころは違うが、
// 組み上がるものは同じでなければならない。渡された 2 つの関数を Auth に差すこと
// で、この形の宣言は 1 箇所に留まる。
//
// nil の関数は「保存済みを持たない」であり、そのときは端末で尋ねる形になる。
func newSSHParts(
	config *application.Service,
	hosts *knownhosts.Service,
	home string,
	passphrase func(absolute string) (string, bool),
	password func(alias string) (string, bool),
) sshParts {
	return sshParts{
		dialer: sshclient.Dialer{
			// **接続のたびに読む。** 設定は走っているあいだに変えられる。
			Verbosity: func() sshclient.Verbosity {
				return sshclient.Verbosity(config.TerminalSettings().Verbosity)
			},
			Auth: sshclient.Auth{
				AgentSocket: os.Getenv("SSH_AUTH_SOCK"),
				Stored:      passphrase,
				Password:    password,
			},
			HostKeys: sshclient.HostKeys{
				Read: readKnownHosts(hosts),
				Add:  addKnownHost(hosts),
			},
		},
		resolve: func(alias string) (effective.Values, error) {
			if config == nil {
				return effective.Values{}, errNoConfiguration
			}
			return config.ResolveConnection(alias)
		},
		home: home,
	}
}

// target は、alias ひとつ分の接続を組み立てる。
func (p sshParts) target(alias string) (sshclient.Target, error) {
	target, _, err := sshclient.NewTarget(alias, p.resolve, p.home)
	return target, err
}

// connector は、埋め込みターミナルが開く対話セッションである。
// aliases は、この接続に現れる alias を、手前から順に返す。
//
// **ProxyJump の手前に立つホストは、それ自身が alias である。** そこに保存された
// パスワードを知らないまま繋ぎに行けば、連鎖の途中で手入力を求めることになる。
// 解決できない設定では行き先だけを返す——その失敗を報告するのは接続の側である。
func (p sshParts) aliases(alias string) []string {
	target, err := p.target(alias)
	if err != nil {
		return []string{alias}
	}
	return appendAliases(nil, target)
}

func appendAliases(listed []string, target sshclient.Target) []string {
	for _, hop := range target.Jump {
		listed = appendAliases(listed, hop)
	}
	if target.Alias == "" {
		return listed
	}
	return append(listed, target.Alias)
}

func (p sshParts) connector() httpserver.Connector {
	return func(ctx context.Context, alias string, size terminal.Size) (terminal.Process, error) {
		target, err := p.target(alias)
		if err != nil {
			return nil, err
		}
		return p.dialer.Open(ctx, target, size)
	}
}

// probe は、認証テストである。**何も尋ねない。**
func (p sshParts) probe() func(ctx context.Context, alias string) (sshclient.Probe, error) {
	return func(ctx context.Context, alias string) (sshclient.Probe, error) {
		target, err := p.target(alias)
		if err != nil {
			return sshclient.Probe{}, err
		}
		return p.dialer.Probe(ctx, target)
	}
}

// run は、決まった接続でコマンドを 1 本走らせる。**何も尋ねない。**
func (p sshParts) run() func(ctx context.Context, target sshclient.Target, command string, stdin []byte) (sshclient.Output, error) {
	return func(ctx context.Context, target sshclient.Target, command string, stdin []byte) (sshclient.Output, error) {
		return p.dialer.Run(ctx, target, command, stdin)
	}
}

// storedPassphrase は、鍵の絶対パスを vault の保存値へ対応づける。
//
// vault が知っているのはワークスペース相対のパスである。~/.ssh の外にある鍵は
// そこに現れないので、答えは「持っていない」——そのときは端末で尋ねる。
func storedPassphrase(passwords *secret.Service, root string) func(string) (string, bool) {
	if passwords == nil || root == "" {
		return nil
	}
	return func(absolute string) (string, bool) {
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", false
		}
		return passwords.KeyPassphraseFor(filepath.ToSlash(relative))
	}
}

// storedPassword は、alias について保存されたアカウントパスワードを返す。
//
// **鍵のパスフレーズとは別の名前空間である。** vault が閉じていれば
// PasswordFor は空を返し、そのときは保存されていないのと同じに扱う——施錠中の
// 保管庫は「知らない」のであって、「無い」のではないが、接続の側から見れば
// どちらも尋ねるしかない。
func storedPassword(passwords *secret.Service) func(string) (string, bool) {
	if passwords == nil {
		return nil
	}
	return func(alias string) (string, bool) {
		password := passwords.PasswordFor(alias)
		return password, password != ""
	}
}

func readKnownHosts(hosts *knownhosts.Service) func() ([]byte, error) {
	if hosts == nil {
		return nil
	}
	return func() ([]byte, error) {
		listing, err := hosts.Listing("")
		if err != nil {
			return nil, err
		}
		var contents strings.Builder
		for _, line := range listing.Lines {
			contents.WriteString(line.Raw)
			contents.WriteString("\n")
		}
		return []byte(contents.String()), nil
	}
}

// addKnownHost は、受け入れた鍵を known_hosts へ書く。
//
// 通るのは Service であって、こちらがファイルを開くのではない。**書く場所が
// 二つある状態を作らない**——あの画面と同じトランザクションを通す。
func addKnownHost(hosts *knownhosts.Service) func(knownhosts.Candidate) error {
	if hosts == nil {
		return nil
	}
	return func(candidate knownhosts.Candidate) error {
		// フィンガープリントは、いま握手した鍵そのものから計算されている。
		// 人が端末でそれを見て受け入れたので、確認済みとして書ける。
		_, err := hosts.Add(candidate, candidate.Fingerprint, false)
		return err
	}
}

// CLIConnection は、`sshc <接続先>` が使うプロセス内 SSH である。
//
// 常駐しているアプリケーションとは別のプロセスなので、vault は開けない——
// あれの鍵は向こうのメモリにしかない。保存済みパスフレーズは向こうへ尋ね、
// 届かなければ端末で尋ねる。**尋ねられる端末がここにはある**ので、届かない
// ことは失敗ではない。問いは開いたセッションの出力を通って端末へ出る。
type CLIConnection struct{ parts sshParts }

// NewCLIConnection は、ホームディレクトリひとつからコマンドライン用の接続を組む。
//
// passphrase は、鍵のワークスペース相対パスに対する保存済みの答えを返す。
// password は、alias に対する保存済みのアカウントパスワードを返す。**この二つは
// 別の名前空間である。** どちらも nil でよく、そのときは端末で尋ねる。
func NewCLIConnection(
	home string,
	passphrase func(relativePath string) (string, bool),
	password func(alias string) (string, bool),
) (CLIConnection, error) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		return CLIConnection{}, err
	}
	transactions := storage.NewManager(workspace, time.Now, rand.Reader)
	config := application.NewService(workspace, transactions)
	hosts := knownhosts.NewService(workspace, transactions, knownhosts.Scanner{})

	stored := func(absolute string) (string, bool) {
		if passphrase == nil {
			return "", false
		}
		relative, err := filepath.Rel(workspace.Root(), absolute)
		if err != nil || strings.HasPrefix(relative, "..") {
			return "", false
		}
		return passphrase(filepath.ToSlash(relative))
	}

	return CLIConnection{parts: newSSHParts(config, hosts, workspace.Home(), stored, password)}, nil
}

// Open は、この alias のセッションをひとつ開く。
func (c CLIConnection) Open(ctx context.Context, alias string, size terminal.Size) (terminal.Process, error) {
	target, err := c.parts.target(alias)
	if err != nil {
		return nil, err
	}
	return c.parts.dialer.Open(ctx, target, size)
}

// Run は、この alias の相手でコマンドをひとつ走らせ、その終了状態を返す。
//
// **打たれたコマンドが設定より強い。** 設定の `RemoteCommand` は「この接続先へ
// 繋いだら常にこれを走らせる」という指示だが、ここで渡されたものは、いま一度
// これを走らせろという指示である。後者を無視して前者を走らせるのは、頼まれて
// いないことを実行することになるので、Stream は渡された方だけを見る。
//
// **端末も要求しない。** 設定の `RequestTTY` がどうであれ、この入口が返すのは
// 集めて読める出力であり、画面制御の混ざったものではない。
func (c CLIConnection) Run(
	ctx context.Context, alias, command string, streams sshclient.Streams,
) (int, error) {
	target, err := c.parts.target(alias)
	if err != nil {
		return sshclient.RemoteFailureExit, err
	}
	return c.parts.dialer.Stream(ctx, target, command, streams)
}
