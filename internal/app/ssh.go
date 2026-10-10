package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"time"

	pkgsftp "github.com/pkg/sftp"

	"sshc/internal/application"
	"sshc/internal/connectionlog"
	"sshc/internal/effective"
	"sshc/internal/httpserver"
	"sshc/internal/keys"
	"sshc/internal/knownhosts"
	"sshc/internal/platform/nativepath"
	"sshc/internal/secret"
	sshcSFTP "sshc/internal/sftp"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
	"sshc/internal/textencoding"
	"sshc/internal/totp"
)

// errNoConfiguration は、設定を読む手段が配線されていないことを報告する。
var errNoConfiguration = errors.New("no configuration service is available")

// sshParts は、プロセス内 SSH クライアントに必要な依存関係を保持する。
type sshParts struct {
	dialer   sshclient.Dialer
	resolve  sshclient.Resolver
	encoding func(string) (textencoding.Name, error)
	// vpnProfile は、この alias が通るVPNプロファイルの名前と識別子を返す。
	vpnProfile func(string) (application.AttachedVPNProfile, error)
	// facts は、IdentityFile の ~ とトークンを展開するこのマシンの事実である。
	facts effective.LocalFacts
}

// sshDependencies は、プロセス内 SSH クライアントが要るものである。
//
// 数が多いので名前で渡す。位置で渡すと、同じ形の関数がいくつも並ぶ呼び出しに
// なり、取り違えてもコンパイルが通る。
type sshDependencies struct {
	config      *application.Service
	knownHosts  *knownhosts.Service
	home        string
	passphrase  func(absolute string) (string, bool)
	password    func(target sshclient.Target) (string, bool)
	oneTimeCode func(target sshclient.Target, question string) (string, bool)
	// vpnRoute は、名前の付いたVPN経路を通して接続先へ繋ぐ。nil なら、VPNを
	// 指定した接続は断る。経路を作れない機械で素の回線へ落とさないためである。
	vpnRoute func(ctx context.Context, profile, address string) (net.Conn, error)
}

// newSSHParts は、プロセス内 SSH の部品一式を組む。
func newSSHParts(dependencies sshDependencies) sshParts {
	config, hosts, home := dependencies.config, dependencies.knownHosts, dependencies.home
	passphrase, password, oneTimeCode := dependencies.passphrase, dependencies.password, dependencies.oneTimeCode
	return sshParts{
		dialer: sshclient.Dialer{
			ObserveOS: func(target sshclient.Target) func(string) {
				if config == nil {
					return nil
				}
				return config.ObserveConnectionOS(target)
			},
			// 接続のたびに読む。 設定は走っているあいだに変えられる。
			Verbosity: func() connectionlog.Level {
				return connectionlog.Level(config.TerminalSettings().Verbosity)
			},
			Auth: sshclient.Auth{
				// Keys 画面と同じ keys.NewAgent で、このプロセスから見た agent の宛先を
				// 決める。Windows は SSH_AUTH_SOCK ではなく OpenSSH の固定の named pipe
				// である。Keys 画面の値そのものは受け取らない。`sshc ssh` はこの部品を
				// CLI のプロセスで組み、CLI を起動したシェルの agent を使うからである。
				Agent:    keys.NewAgent(os.LookupEnv),
				Stored:   passphrase,
				Password: password,
				TOTP:     oneTimeCode,
			},
			HostKeys: sshclient.HostKeys{
				Read: readKnownHosts(hosts),
				Add:  addKnownHost(hosts),
			},
			DialVPN: dependencies.vpnRoute,
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
		vpnProfile: func(alias string) (application.AttachedVPNProfile, error) {
			if config == nil {
				return application.AttachedVPNProfile{}, errNoConfiguration
			}
			return config.ConnectionVPNProfile(alias)
		},
		facts: application.LocalFactsFor(home),
	}
}

// target は、alias ひとつ分の接続を組み立てる。
func (p sshParts) target(alias string) (sshclient.Target, error) {
	target, err := sshclient.NewTarget(alias, p.resolve, p.facts)
	if err != nil {
		return sshclient.Target{}, err
	}
	encoding, err := p.encoding(alias)
	if err != nil {
		return sshclient.Target{}, err
	}
	target.Encoding = encoding
	profile, err := p.vpnProfile(alias)
	if err != nil {
		return sshclient.Target{}, err
	}
	target.VPN, target.VPNProfileID = profile.Name, profile.ID
	return target, nil
}

// aliases は、接続に現れる alias を ProxyJump の手前から順に返す。
// 接続を組み立てられない alias は、それ自身だけを返す。
func (p sshParts) aliases(alias string) []string {
	target, err := p.target(alias)
	if err != nil {
		return []string{alias}
	}
	return appendAliases(nil, target)
}

// routeBindings は、alias への接続に現れる alias ごとの認証先の digest を、接続が
// 組み立てるとおりに返す。踏み台はホップとしての値で、単独で繋ぐときの値とは違いうる
// （VPN は行き先にだけ付く）。埋め込みターミナルも同じ Target の値で照合する
// （storedPassword、storedTOTP）。同じ alias が2度現れれば後の値にする。行き先は最後に
// 現れる。
func (p sshParts) routeBindings(alias string) (map[string]string, error) {
	target, err := p.target(alias)
	if err != nil {
		return nil, err
	}
	route := append(target.JumpRoute(), target)
	bindings := make(map[string]string, len(route))
	for _, hop := range route {
		bindings[hop.Alias] = hop.AuthenticationBinding()
	}
	return bindings, nil
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
func (p sshParts) run() func(ctx context.Context, target sshclient.Target, command sshclient.Command) (sshclient.Output, error) {
	return func(ctx context.Context, target sshclient.Target, command sshclient.Command) (sshclient.Output, error) {
		return p.dialer.Run(ctx, target, command)
	}
}

// sftp resolves each operation before the pool selects a connection. Identity
// includes all resolved settings, while Open captures the exact resolved target.
func (p sshParts) sftp() sshcSFTP.ResolveRemote {
	return func(_ context.Context, alias string) (sshcSFTP.RemoteTarget, error) {
		target, err := p.target(alias)
		if err != nil {
			return sshcSFTP.RemoteTarget{}, err
		}
		encoded, err := json.Marshal(target)
		if err != nil {
			return sshcSFTP.RemoteTarget{}, err
		}
		digest := sha256.Sum256(encoded)
		return sshcSFTP.RemoteTarget{
			Identity: hex.EncodeToString(digest[:]),
			Open: func(ctx context.Context) (sshcSFTP.Remote, error) {
				return p.openSFTP(ctx, target)
			},
		}, nil
	}
}

func (p sshParts) openSFTP(ctx context.Context, target sshclient.Target) (sshcSFTP.Remote, error) {
	connection, err := p.dialer.Connect(ctx, target)
	if err != nil {
		return nil, err
	}
	// Reads are pipelined by pkg/sftp on their own; writes are not unless
	// asked, and one 32 KiB request per round trip made every upload crawl
	// on a distant host. A failed pipelined write can leave the file longer
	// than what arrived, so the upload plane truncates a part back to its
	// acknowledged offset after an error and verifies the whole part before
	// publishing it.
	client, err := sshcSFTP.NewSSHClient(connection.Client(), pkgsftp.UseConcurrentWrites(true))
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	return newSFTPRemote(client, connection), nil
}

// sftpTransport is the SSH transport an SFTP session runs on. Wait reports its
// end; Close releases it together with every ProxyJump hop.
type sftpTransport interface {
	Wait() error
	Close() error
}

// newSFTPRemote wraps one SFTP session so that the pool can ask whether it is
// still usable before lending it to the next operation.
func newSFTPRemote(session *sshcSFTP.Client, transport sftpTransport) *sftpRemote {
	remote := &sftpRemote{Remote: session, transport: transport, dead: make(chan struct{})}
	// A server can end the SFTP subsystem while the transport stays up: the
	// sftp-server exits, sshd's ChannelTimeout closes the channel, or a reply
	// cannot be parsed. Every later request then fails as a lost connection,
	// so the transport has nothing left to carry and is closed as well.
	go func() {
		_ = session.Wait()
		_ = transport.Close()
	}()
	// The transport reports its end through Wait, whoever ended it.
	go func() {
		_ = transport.Wait()
		close(remote.dead)
	}()
	return remote
}

type sftpRemote struct {
	sshcSFTP.Remote
	transport sftpTransport
	dead      chan struct{}
}

func (remote *sftpRemote) Alive() bool {
	select {
	case <-remote.dead:
		return false
	default:
		return true
	}
}

func (remote *sftpRemote) UnderlyingRemote() sshcSFTP.Remote { return remote.Remote }

func (remote *sftpRemote) OpenRange(candidate string, offset int64) (io.ReadCloser, error) {
	ranged, ok := remote.Remote.(sshcSFTP.RangeRemote)
	if !ok {
		return nil, sshcSFTP.ErrInvalidTransfer
	}
	return ranged.OpenRange(candidate, offset)
}

func (remote *sftpRemote) Close() error {
	return errors.Join(remote.Remote.Close(), remote.transport.Close())
}

// storedPassphrase は、鍵の絶対パスを vault の保存値へ対応づける。
func storedPassphrase(vault *secret.Service, root string) func(string) (string, bool) {
	if vault == nil || root == "" {
		return nil
	}
	return func(absolute string) (string, bool) {
		relative, inside := nativepath.RelativeSlash(root, absolute)
		if !inside {
			return "", false
		}
		return vault.KeyPassphraseFor(relative)
	}
}

// storedPassword は、alias について保存されたアカウントパスワードを返す。
func storedPassword(vault *secret.Service) func(sshclient.Target) (string, bool) {
	if vault == nil {
		return nil
	}
	return func(target sshclient.Target) (string, bool) {
		password := vault.BoundFor(secret.KindPassword, target.Alias, target.AuthenticationBinding())
		return password, password != ""
	}
}

// sftpConnectionLimit says how many SFTP connections may be open to a host at
// once. A host, or a ProxyJump hop on the way to it, that is answered with a
// one-time code allows one: every connection opened in the same window would
// present the same code, and PAM's TOTP module refuses a code it has already
// accepted, so a ranged download would fail on its second connection.
func sftpConnectionLimit(vault *secret.Service, target func(string) (sshclient.Target, error)) func(alias string) int {
	return func(alias string) int {
		if vault == nil {
			return 0
		}
		resolved, err := target(alias)
		if err != nil {
			return 0
		}
		for _, hop := range append(resolved.JumpRoute(), resolved) {
			if vault.BoundFor(secret.KindTOTP, hop.Alias, hop.AuthenticationBinding()) != "" {
				return 1
			}
		}
		return 0
	}
}

// storedTOTP generates a code only for an explicit one-time-password prompt.
// The seed remains inside the unlocked vault for embedded sessions.
func storedTOTP(vault *secret.Service) func(sshclient.Target, string) (string, bool) {
	if vault == nil {
		return nil
	}
	return func(target sshclient.Target, question string) (string, bool) {
		if !totp.MatchesPrompt(question) {
			return "", false
		}
		provisioning := vault.BoundFor(secret.KindTOTP, target.Alias, target.AuthenticationBinding())
		if provisioning == "" {
			return "", false
		}
		configuration, err := totp.Parse(provisioning)
		if err != nil {
			return "", false
		}
		code, err := configuration.Code(time.Now())
		return code, err == nil
	}
}

func readKnownHosts(hosts *knownhosts.Service) func(string) ([]byte, error) {
	if hosts == nil {
		return nil
	}
	// 接続ログは一致した行を「<ファイル>の何行目」と書く。known_hosts画面やエディタと同じ
	// 物理行で数えるため、エントリだけに詰め直さずファイルの原文を渡す。
	return hosts.ReadFile
}

// addKnownHost は、受け入れた鍵を UserKnownHostsFile の最初のファイルへ書く。
func addKnownHost(hosts *knownhosts.Service) func(string, knownhosts.Candidate) error {
	if hosts == nil {
		return nil
	}
	return hosts.Remember
}

// CLIConnection は、`sshc ssh <alias>` が使うプロセス内 SSH である。
type CLIConnection struct{ parts sshParts }

// CLIConnectionOptions は、コマンドライン用の接続が要るものである。
//
// 数が多いので名前で渡す。位置で渡すと、同じ形の関数がいくつも並ぶ呼び出しに
// なり、取り違えてもコンパイルが通る。
type CLIConnectionOptions struct {
	Home        string
	Passphrase  func(relativePath string) (string, bool)
	Password    func(target sshclient.Target) (string, bool)
	OneTimeCode func(target sshclient.Target, question string) (string, bool)
	// VPNRoute は、名前の付いたVPN経路を通して接続先へ繋ぐ。nil なら、VPNを
	// 指定した接続は素の回線へ落とさずに断る。
	VPNRoute func(ctx context.Context, profile, address string) (net.Conn, error)
}

// NewCLIConnection は、ホームディレクトリひとつからコマンドライン用の接続を組む。
func NewCLIConnection(options CLIConnectionOptions) (CLIConnection, error) {
	home, passphrase := options.Home, options.Passphrase
	password, oneTimeCode := options.Password, options.OneTimeCode
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
		relative, inside := nativepath.RelativeSlash(workspace.Root(), absolute)
		if !inside {
			return "", false
		}
		return passphrase(relative)
	}

	// コマンドラインの接続は、engine が解決した接続情報を受け取って自分で繋ぐ。
	// VPN 経路もコンテナも engine が持つので、ここは engine が差し出す中継へ
	// 繋ぐだけである。渡されていなければ、VPN を指定した接続は断る。
	parts := newSSHParts(sshDependencies{
		config: config, knownHosts: hosts, home: workspace.Home(),
		passphrase: stored, password: password, oneTimeCode: oneTimeCode,
		vpnRoute: options.VPNRoute,
	})
	configuredVerbosity := parts.dialer.Verbosity
	// `sshc ssh` はブラウザの接続中表示を持たないため、最低限の接続段階を
	// 常に端末へ残す。詳細度を上げた設定はそのまま尊重する。
	parts.dialer.Verbosity = func() connectionlog.Level {
		level := connectionlog.Notice
		if configuredVerbosity != nil {
			level = configuredVerbosity()
		}
		if level < connectionlog.Brief {
			return connectionlog.Brief
		}
		return level
	}
	return CLIConnection{parts: parts}, nil
}

// Resolve は Open と Run が接続に使う target を、接続を始めずに返す。
// 設定の診断用に別の解決経路を作ると、表示と実接続が分岐するため、同じ parts.target
// だけを公開する。
func (c CLIConnection) Resolve(alias string) (sshclient.Target, error) {
	return c.parts.target(alias)
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
