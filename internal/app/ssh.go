package app

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgsftp "github.com/pkg/sftp"

	"sshc/internal/application"
	"sshc/internal/effective"
	"sshc/internal/httpserver"
	"sshc/internal/knownhosts"
	"sshc/internal/secret"
	sshcSFTP "sshc/internal/sftp"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
	"sshc/internal/textencoding"
)

// errNoConfiguration は、設定を読む手段が配線されていないことを報告する。
var errNoConfiguration = errors.New("no configuration service is available")

// sshParts は、プロセス内 SSH クライアントに必要な依存関係を保持する。
type sshParts struct {
	dialer   sshclient.Dialer
	resolve  sshclient.Resolver
	encoding func(string) (textencoding.Name, error)
	home     string
}

// newSSHParts は、プロセス内 SSH の部品一式を組む。
func newSSHParts(
	config *application.Service,
	hosts *knownhosts.Service,
	home string,
	passphrase func(absolute string) (string, bool),
	password func(target sshclient.Target) (string, bool),
) sshParts {
	return sshParts{
		dialer: sshclient.Dialer{
			// 接続のたびに読む。 設定は走っているあいだに変えられる。
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
		encoding: func(alias string) (textencoding.Name, error) {
			if config == nil {
				return "", errNoConfiguration
			}
			return config.ConnectionEncoding(alias)
		},
		home: home,
	}
}

// target は、alias ひとつ分の接続を組み立てる。
func (p sshParts) target(alias string) (sshclient.Target, error) {
	target, err := sshclient.NewTarget(alias, p.resolve, p.home)
	if err != nil {
		return sshclient.Target{}, err
	}
	encoding, err := p.encoding(alias)
	if err != nil {
		return sshclient.Target{}, err
	}
	target.Encoding = encoding
	return target, nil
}

// connector は、埋め込みターミナルが開く対話セッションである。
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

func (p sshParts) agentConnector() httpserver.AgentConnector {
	return func(ctx context.Context, alias string, kind terminal.AgentKind, reference string, size terminal.Size) (terminal.Process, error) {
		command, err := terminal.AgentResumeCommand(kind, reference)
		if err != nil {
			return nil, err
		}
		target, err := p.target(alias)
		if err != nil {
			return nil, err
		}
		target.RemoteCommand = command
		target.RequestTTY = "yes"
		return p.dialer.Open(ctx, target, size)
	}
}

func (p sshParts) connectionBinding(alias string) (string, error) {
	target, err := p.target(alias)
	if err != nil {
		return "", err
	}
	return target.AuthenticationBinding(), nil
}

// probe は、認証テストである。何も尋ねない。
func (p sshParts) probe() func(ctx context.Context, alias string) (sshclient.Probe, error) {
	return func(ctx context.Context, alias string) (sshclient.Probe, error) {
		target, err := p.target(alias)
		if err != nil {
			return sshclient.Probe{}, err
		}
		return p.dialer.Probe(ctx, target)
	}
}

// run は、決まった接続でコマンドを 1 本走らせる。何も尋ねない。
func (p sshParts) run() func(ctx context.Context, target sshclient.Target, command string, stdin []byte) (sshclient.Output, error) {
	return func(ctx context.Context, target sshclient.Target, command string, stdin []byte) (sshclient.Output, error) {
		return p.dialer.Run(ctx, target, command, stdin)
	}
}

// sftp opens a non-interactive SFTP channel using the same target resolution,
// credentials, known-host policy and ProxyJump transport as terminal sessions.
func (p sshParts) sftp() sshcSFTP.OpenRemote {
	return func(ctx context.Context, alias string) (sshcSFTP.Remote, error) {
		target, err := p.target(alias)
		if err != nil {
			return nil, err
		}
		connection, err := p.dialer.Connect(ctx, target)
		if err != nil {
			return nil, err
		}
		client, err := pkgsftp.NewClient(connection.Client())
		if err != nil {
			_ = connection.Close()
			return nil, err
		}
		return &sftpRemote{Remote: sshcSFTP.NewClient(client), transport: connection}, nil
	}
}

type sftpRemote struct {
	sshcSFTP.Remote
	transport *sshclient.Connection
}

func (remote *sftpRemote) Close() error {
	return errors.Join(remote.Remote.Close(), remote.transport.Close())
}

// storedPassphrase は、鍵の絶対パスを vault の保存値へ対応づける。
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
func storedPassword(passwords *secret.Service) func(sshclient.Target) (string, bool) {
	if passwords == nil {
		return nil
	}
	return func(target sshclient.Target) (string, bool) {
		password := passwords.BoundPasswordFor(target.Alias, target.AuthenticationBinding())
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
func addKnownHost(hosts *knownhosts.Service) func(knownhosts.Candidate) error {
	if hosts == nil {
		return nil
	}
	return func(candidate knownhosts.Candidate) error {
		// フィンガープリントは、いま握手した鍵そのものから計算されている。
		_, err := hosts.Add(candidate, candidate.Fingerprint, false)
		return err
	}
}

// CLIConnection は、`sshc <接続先>` が使うプロセス内 SSH である。
type CLIConnection struct{ parts sshParts }

// NewCLIConnection は、ホームディレクトリひとつからコマンドライン用の接続を組む。
func NewCLIConnection(
	home string,
	passphrase func(relativePath string) (string, bool),
	password func(target sshclient.Target) (string, bool),
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

	parts := newSSHParts(config, hosts, workspace.Home(), stored, password)
	configuredVerbosity := parts.dialer.Verbosity
	// `sshc ssh` はブラウザの接続中表示を持たないため、最低限の接続段階を
	// 常に端末へ残す。詳細度を上げた設定はそのまま尊重する。
	parts.dialer.Verbosity = func() sshclient.Verbosity {
		level := sshclient.Quiet
		if configuredVerbosity != nil {
			level = configuredVerbosity()
		}
		if level < sshclient.Brief {
			return sshclient.Brief
		}
		return level
	}
	return CLIConnection{parts: parts}, nil
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
func (c CLIConnection) Run(
	ctx context.Context, alias, command string, streams sshclient.Streams,
) (int, error) {
	target, err := c.parts.target(alias)
	if err != nil {
		return sshclient.RemoteFailureExit, err
	}
	return c.parts.dialer.Stream(ctx, target, command, streams)
}
