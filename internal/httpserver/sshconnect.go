package httpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/application"
	"sshc/internal/effective"
	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// Connector は、alias ひとつ分の対話セッションを開く。
//
// **外部の ssh は起こさない。** 設定を読むのは internal/effective、SSH を話すのは
// internal/sshclient であり、この関数はその二つを繋ぐだけである。
type Connector func(ctx context.Context, alias string, size terminal.Size) (terminal.Process, error)

// newConnector は、走っているサービスからプロセス内 SSH を組み立てる。
//
// 部品をひとつずつ TerminalHandlers へ渡さないのは、あそこが「セッションを
// 開く」以上のことを知る必要が無いからである。鍵も vault も known_hosts も、
// この関数の内側で閉じる。
func newConnector(options Options, home, root string) Connector {
	if options.Config == nil {
		return nil
	}

	auth := sshclient.Auth{
		AgentSocket: os.Getenv("SSH_AUTH_SOCK"),
		Stored:      storedPassphrase(options, root),
	}
	dialer := sshclient.Dialer{
		Auth: auth,
		HostKeys: sshclient.HostKeys{
			Read: readKnownHosts(options),
			Add:  addKnownHost(options),
		},
	}
	resolve := func(alias string) (effective.Values, error) {
		return options.Config.ResolveConnection(alias)
	}

	return func(ctx context.Context, alias string, size terminal.Size) (terminal.Process, error) {
		target, _, err := sshclient.NewTarget(alias, resolve, home)
		if err != nil {
			return nil, err
		}
		return dialer.Open(ctx, target, size)
	}
}

// storedPassphrase は、鍵の絶対パスを vault の保存値へ対応づける。
//
// vault が知っているのはワークスペース相対のパスである。~/.ssh の外にある鍵は
// そこに現れないので、答えは「持っていない」——そのときは端末で尋ねる。
func storedPassphrase(options Options, root string) func(string) (string, bool) {
	if options.Passwords == nil || root == "" {
		return nil
	}
	return func(absolute string) (string, bool) {
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", false
		}
		return options.Passwords.KeyPassphraseFor(filepath.ToSlash(relative))
	}
}

func readKnownHosts(options Options) func() ([]byte, error) {
	if options.KnownHosts == nil {
		return nil
	}
	return func() ([]byte, error) {
		listing, err := options.KnownHosts.Listing("")
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
func addKnownHost(options Options) func(knownhosts.Candidate) error {
	if options.KnownHosts == nil {
		return nil
	}
	return func(candidate knownhosts.Candidate) error {
		// フィンガープリントは、いま握手した鍵そのものから計算されている。
		// 人が端末でそれを見て受け入れたので、確認済みとして書ける。
		_, err := options.KnownHosts.Add(candidate, candidate.Fingerprint, false)
		return err
	}
}

// connectProblem は、接続を組み立てられなかった理由を通信形式に変える。
func connectProblem(err error) (string, bool) {
	var unresolvable *application.ErrUnresolvable
	switch {
	case errors.As(err, &unresolvable):
		return "alias_unresolvable", true
	case errors.Is(err, sshclient.ErrProxyCommand):
		return "proxy_command_refused", true
	case errors.Is(err, sshclient.ErrJumpDepth):
		return "jump_depth_exceeded", true
	case errors.Is(err, sshclient.ErrNoHostName):
		return "alias_unresolvable", true
	}
	return "", false
}
