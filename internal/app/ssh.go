package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/application"
	"sshc/internal/effective"
	"sshc/internal/httpserver"
	"sshc/internal/knownhosts"
	"sshc/internal/secret"
	"sshc/internal/sshclient"
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

func newSSHParts(
	config *application.Service,
	passwords *secret.Service,
	hosts *knownhosts.Service,
	home, root string,
) sshParts {
	return sshParts{
		dialer: sshclient.Dialer{
			Auth: sshclient.Auth{
				AgentSocket: os.Getenv("SSH_AUTH_SOCK"),
				Stored:      storedPassphrase(passwords, root),
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
