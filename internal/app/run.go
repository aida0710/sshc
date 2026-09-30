package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/application"
	"sshc/internal/browserauth"
	"sshc/internal/configresolver"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/keys"
	"sshc/internal/platform"
	"sshc/internal/releasecheck"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
	"sshc/internal/validate"
	"sshc/internal/vpn"
)

type ListenFunc func(network, address string) (net.Listener, error)

// ErrListen はengineがloopback listenerを確保できなかったことを表す。
// mobile外殻はこのsentinelだけをport failureとして利用者へ案内する。
var ErrListen = errors.New("listen")

// unsafeAliasWarning は alias を拒否する理由を示す。
const unsafeAliasWarning = "This alias contains characters that could change the meaning of a command line, so this connection will be refused."

type Dependencies struct {
	Random io.Reader
	// Announce は、この常駐が受け付けられる状態になったことを伝える。
	Announce   func(Readiness) error
	Listen     ListenFunc
	StopEngine func()
	Port       int
	// DefaultPort is used only when neither --port nor saved settings choose a
	// port. Desktop keeps a stable origin for bookmarks and installed web apps;
	// mobile uses its own value so the native WebView keeps its storage.
	DefaultPort int
	UI          fs.FS
	Logger      *slog.Logger
	Home        string
	Owner       handoff.Owner
	PID         int
	// Toolchain と KeyAgent は、鍵 vault とオペレーティングシステムとの境界。
	Toolchain    platform.Toolchain
	KeyAgent     platform.KeyAgent
	ScanHostKeys func(ctx context.Context, address string, timeout time.Duration) ([]ssh.PublicKey, error)
	Probe        func(ctx context.Context, alias string) (sshclient.Probe, error)
	RemoteRun    func(ctx context.Context, target sshclient.Target, command sshclient.Command) (sshclient.Output, error)
	Updates      *releasecheck.Checker
	// Lookup は親の環境を読み、利用者のログインシェルを見つけるために使う。
	Lookup func(string) (string, bool)
	// TerminalStarter は PTY を確保する。nil の場合は既定実装を使用する。
	TerminalStarter terminal.Starter
	Environ         func() []string
	// DockerEnvironment は、VPN 経路の docker を探して起動する環境を返す。nil なら、
	// Environ から求めたログインシェルの環境を使う。受け入れテストは docker の無い
	// 環境を渡し、テストを動かすマシンの docker に触れない。
	DockerEnvironment vpn.Environment
	// SessionNow は、セッションマネージャがアクショントークンの失効に使う時計。
	SessionNow      func() time.Time
	ShutdownTimeout time.Duration
	// SFTPDownloadSpoolRoot is where SFTP downloads are prepared, a directory
	// only this user can write. The desktop engine passes its user cache and
	// Android the app's cache; empty leaves downloads unavailable.
	SFTPDownloadSpoolRoot string
}

// DefaultShutdownTimeout は、停止を始めた engine が、ターミナルと HTTP の要求が
// 自分から終わるのを待つ上限である。過ぎると両方を強制的に閉じる（unwind）。
// 停止は Ctrl+C、サービスの停止、sshc engine --replace で利用者が待っているときに
// 起きるので、応答しない接続先のターミナル1つがそれを止めないよう秒の単位で打ち切る。
// そのうえで、応答する接続先には切断を送り終えるだけの猶予を残す長さにしてある。
const DefaultShutdownTimeout = 4 * time.Second

// MaxShutdownDuration は、停止を始めた engine が engine lock を手放すまでの上限である
// （ShutdownTimeout が既定のとき）。ターミナルと HTTP を強制で閉じるまでの
// DefaultShutdownTimeout のあとに、VPN の経路を畳む vpnStopTimeout が続く。
// engine の外で終わりを待つ側（sshc engine --replace）は、待ち時間をここから導く。
const MaxShutdownDuration = DefaultShutdownTimeout + vpnStopTimeout

// Readiness は、受け付けを始めた常駐がどんな状態かを述べる。
type Readiness struct {
	Owner handoff.Owner
	// Entrance は起動時に使用する UI URL である。
	Entrance    string
	VaultExists bool
	// VaultUnlocked は、受け付けを始めた時点で Vault のロックが解除されているかを述べる。
	// パスワードなしの Vault は起動時にロックが解除されるので、利用者に解除を求めない。
	VaultUnlocked bool
	// BrowserRegistrationRequired is true only before any browser profile has
	// enrolled on this device. Native runners may open Entrance once in this case.
	BrowserRegistrationRequired bool
}

func buildKeyService(workspace *storage.Workspace, dependencies Dependencies, configService *application.Service) (*keys.Service, *storage.Manager) {
	transactions := storage.NewManager(workspace, time.Now, dependencies.Random)
	return keys.NewService(keys.ServiceOptions{
		Workspace:     workspace,
		Transactions:  transactions,
		Resolver:      configresolver.ForWorkspace(workspace),
		Catalogue:     keys.CatalogueReader{Toolchain: dependencies.Toolchain},
		Agent:         dependencies.KeyAgent,
		Now:           time.Now,
		Random:        dependencies.Random,
		ValidateGroup: configService.ValidateDeclaredGroup,
	}), transactions
}

// Build は、Run と同じ組み立てを通った HTTP サーバーと bootstrap を返す。
// Serve も、VPN の経路の監視と自動同期の開始も、停止の後始末もしない。
//
// 製品は Run を使い、これを呼ばない。受け入れテスト（internal/acceptance）が、
// 製品と同じ配線のサーバーを自分で Serve して要求を送るための入口である。
func Build(dependencies Dependencies, version string) (*httpserver.Server, string, error) {
	built, err := build(dependencies, version)
	return built.server, built.bootstrap, err
}

// runtime は、build が組み立てたもののうち、寿命の管理に要るものである。
type runtime struct {
	server      *httpserver.Server
	bootstrap   string
	document    handoff.Handoff
	stateDir    string
	terminals   *terminal.Registry
	vault       *secret.Service
	autoSync    *remotesync.Auto
	autoCancel  context.CancelFunc
	autoDone    chan struct{}
	browserAuth *browserauth.Store
	sftpPool    *sshcSFTP.RemotePool
	vpn         *vpn.Manager
	vpnCancel   context.CancelFunc
	vpnDone     chan struct{}
}

func build(dependencies Dependencies, version string) (runtime, error) {
	services, err := newEngineServices(dependencies)
	if err != nil {
		return runtime{}, err
	}
	wanted := dependencies.Port
	if wanted == 0 {
		wanted = services.config.EngineSettings().Port
	}
	strictPort := wanted != 0
	persistBrowserPort := !strictPort && dependencies.DefaultPort != 0
	if persistBrowserPort {
		storedPort, portErr := services.browserAuth.Port()
		if portErr != nil {
			return runtime{}, fmt.Errorf("browser origin: %w", portErr)
		}
		wanted = storedPort
		if wanted == 0 {
			wanted = dependencies.DefaultPort
		}
	}

	listener, err := listenLoopback(dependencies.Listen, wanted, randomBelow)
	// A stable default may already belong to another OS user. Select one fallback
	// once and persist it as device-local browser origin state. Explicit --port
	// and saved user settings remain strict and never silently move.
	if err != nil && persistBrowserPort {
		listener, err = listenLoopback(dependencies.Listen, 0, randomBelow)
	}
	if err != nil {
		return runtime{}, fmt.Errorf("%w: %w", ErrListen, err)
	}
	// 登録は origin（port）に束縛する。既定 port の fallback だけでなく、--port や
	// 保存設定で port を固定した場合も同じである。ここを飛ばすと、以前の port で
	// 発行した登録が別の port の engine でも通り、空いた旧 port を占有した process が
	// bookmark から token を集められる。
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok || tcpAddress.Port < 1 {
		listener.Close()
		return runtime{}, fmt.Errorf("%w: browser origin listener has no TCP port", ErrListen)
	}
	if err := services.browserAuth.SetPort(tcpAddress.Port); err != nil {
		listener.Close()
		return runtime{}, fmt.Errorf("browser origin: %w", err)
	}

	sessions, bootstrap, err := session.NewManager(dependencies.Random)
	if err != nil {
		listener.Close()
		return runtime{}, fmt.Errorf("session: %w", err)
	}
	if dependencies.SessionNow != nil {
		sessions.Now = dependencies.SessionNow
	}

	cliSecret, err := handoff.Mint(dependencies.Random)
	if err != nil {
		listener.Close()
		return runtime{}, err
	}

	server, err := httpserver.New(httpserver.Options{
		Listener:  listener,
		CLISecret: cliSecret,
		Updates:   dependencies.Updates,
		ConnectWarnings: func(alias string) []string {
			if err := validate.Alias(alias); err != nil {
				return []string{unsafeAliasWarning}
			}
			return nil
		},
		ConnectAliases:        services.ssh.aliases,
		Sessions:              sessions,
		BrowserAuth:           services.browserAuth,
		UI:                    dependencies.UI,
		Version:               version,
		Owner:                 dependencies.Owner,
		StopEngine:            dependencies.StopEngine,
		ProtocolVersion:       handoff.ProtocolVersion,
		Logger:                dependencies.Logger,
		Config:                services.config,
		Keys:                  services.keys,
		Diagnostics:           services.diagnostics,
		KnownHosts:            services.knownHosts,
		RemoteKeys:            services.remoteKeys,
		Recent:                services.recent,
		SFTP:                  services.sftp,
		SFTPTransferStatePath: filepath.Join(services.workspace.StateDir(), sftpTransferStateName),
		SFTPDownloadSpoolRoot: dependencies.SFTPDownloadSpoolRoot,
		Workspaces:            services.workspaces,
		Snippets:              services.snippets,
		Vault:                 services.vault,
		Connect:               services.ssh.connector(),
		ConnectionOpened: func(alias string) {
			if err := services.recentStore.Record(alias); err != nil && dependencies.Logger != nil {
				dependencies.Logger.Warn("record recent SSH connection", "alias", alias, "error", err)
			}
		},
		Sync:                      services.sync,
		AutoSync:                  services.autoSync,
		Terminals:                 services.terminals,
		VPN:                       services.vpn,
		VPNProfiles:               services.vpnProfiles,
		TerminalStartDirectory:    services.config.TerminalStartDirectory,
		LoginShell:                func() (string, error) { return platform.LoginShell(dependencies.Lookup) },
		LocalShellProfiles:        func() []platform.ShellProfile { return platform.ShellProfiles(dependencies.Lookup) },
		TerminalLocalShellProfile: services.config.TerminalLocalShellProfile,
		TerminalEnvironment: func() []string {
			var environment []string
			if dependencies.Environ != nil {
				environment = dependencies.Environ()
			}
			return platform.LoginEnvironment(environment)
		},
	})
	if err != nil {
		listener.Close()
		return runtime{}, err
	}

	document := handoff.Handoff{
		SchemaVersion:   handoff.SchemaVersion,
		URL:             server.URL(),
		Secret:          cliSecret,
		Owner:           dependencies.Owner,
		PID:             dependencies.PID,
		Version:         version,
		ProtocolVersion: handoff.ProtocolVersion,
	}
	// handoff を書けないことは致命である。書けなかった常駐は `sshc ssh <alias>` から
	// 見えないまま動き続け、2 台目の engine が同じ handoff を書きに来る。
	// 書き込みは rename のあとのディレクトリの同期で失敗しうるので、公開されたかは
	// 不定である。どの失敗のあとでも自分の秘密で Remove を試みる。置き換わって
	// いなければ、または別の秘密の handoff があれば、Remove は何も消さない。
	stateDir := services.workspace.StateDir()
	if err := handoff.Write(stateDir, document); err != nil {
		if removeErr := handoff.Remove(stateDir, document.Secret); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove the possibly published handoff: %w", removeErr))
		}
		listener.Close()
		return runtime{}, fmt.Errorf("publish the command-line handoff: %w", err)
	}
	return runtime{
		server:      server,
		bootstrap:   bootstrap,
		document:    document,
		stateDir:    stateDir,
		terminals:   services.terminals,
		vault:       services.vault,
		autoSync:    services.autoSync,
		browserAuth: services.browserAuth,
		sftpPool:    services.sftpPool,
		vpn:         services.vpn,
	}, nil
}

// sftpTransferStateName は、転送の一覧と再開の位置を置く、状態ディレクトリの中の
// ファイル名。このマシンの engine だけのものなので、同期では運ばない。
const sftpTransferStateName = "transfers.json"

// StateDir は、engine lock、handoff、SFTP の転送キューを置く sshc の state
// directory を返す。設定を書く Workspace と同じく、解決した ~/.ssh の下にする。
// ~/.ssh や $HOME が symlink でも、lock や handoff をたどる側と設定を書く側で
// 答えが分かれず、symlink を拒む no-follow の歩き方でも開ける。
func StateDir(home string) (string, error) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		return "", err
	}
	return workspace.StateDir(), nil
}

func Run(ctx context.Context, dependencies Dependencies, version string) error {
	asked, stopAsked := context.WithCancel(ctx)
	defer stopAsked()
	dependencies.StopEngine = stopAsked

	built, err := build(dependencies, version)
	if err != nil {
		return err
	}

	// 経路の監視は HTTP を受け付ける前に始める。前回の engine のコンテナを回収し
	// 終えるまで経路の起動を待たせる予告を、最初の要求より先に済ませるためである。
	built.startVPNSupervisor(asked, dependencies.Logger)

	serveErrors := make(chan error, 1)
	go func() { serveErrors <- built.server.Serve() }()

	built.startAutoSync(asked)

	// すべての経路で HTTP サーバーの停止完了を待つ。
	stop := func(reason error) error {
		unwound := built.unwind(dependencies)
		return errors.Join(reason, unwound, <-serveErrors)
	}

	if dependencies.Announce != nil {
		var vault secret.State
		if built.vault != nil {
			if vault, err = built.vault.State(); err != nil {
				return stop(fmt.Errorf("read the vault state: %w", err))
			}
		}
		readiness := Readiness{
			Owner:         dependencies.Owner,
			Entrance:      built.server.URL() + "/#bootstrap=" + built.bootstrap,
			VaultExists:   vault.Exists,
			VaultUnlocked: vault.Unlocked,
		}
		registered, registrationErr := built.browserAuth.HasRegistrations()
		if registrationErr != nil {
			return stop(fmt.Errorf("read browser registration state: %w", registrationErr))
		}
		readiness.BrowserRegistrationRequired = !registered
		if err := dependencies.Announce(readiness); err != nil {
			return stop(fmt.Errorf("announce the entrance: %w", err))
		}
	}

	select {
	case err := <-serveErrors:
		// Serve が自分から戻った。後始末はそれでも全部通る。
		return errors.Join(err, built.unwind(dependencies))
	case <-asked.Done():
		// シグナルと API 要求には同じ停止処理を使用する。
		return stop(nil)
	}
}

// unwind は engine lock の解放前に停止処理を完了する。
func (r runtime) unwind(dependencies Dependencies) error {
	timeout := dependencies.ShutdownTimeout
	if timeout <= 0 {
		timeout = DefaultShutdownTimeout
	}
	deadline := time.AfterFunc(timeout, func() {
		go r.terminals.ForceClose()
		go r.server.ForceClose()
	})
	defer deadline.Stop()

	// AutoSync may finish a context-free local commit after its last remote
	// request. Cancel it before any other service starts unwinding, and retain
	// its completion as an engine-lifetime barrier so an Android Stop→Start
	// cannot overlap two generations of local writes.
	if r.autoCancel != nil {
		r.autoCancel()
	}

	r.server.BeginStopping()

	var joined []error
	if err := handoff.Remove(r.stateDir, r.document.Secret); err != nil {
		joined = append(joined, fmt.Errorf("remove the command-line handoff: %w", err))
	}

	r.terminals.BeginShutdown()
	r.server.BeginShutdown()

	barrierCount := 2
	if r.autoDone != nil {
		barrierCount++
	}
	barriers := make(chan error, barrierCount)
	go func() { barriers <- r.terminals.Wait() }()
	go func() { barriers <- r.server.Wait() }()
	if r.autoDone != nil {
		go func() {
			<-r.autoDone
			barriers <- nil
		}()
	}
	for range barrierCount {
		if err := <-barriers; err != nil {
			joined = append(joined, err)
		}
	}

	// The server has drained its requests and closed the transfer manager, so
	// every SFTP connection is back in the pool or already closed.
	if r.sftpPool != nil {
		if err := r.sftpPool.Close(); err != nil {
			joined = append(joined, fmt.Errorf("close the idle SFTP connections: %w", err))
		}
	}
	// 経路は engine のものである。engine が終わったあとも動いているコンテナは、
	// 誰も面倒を見ないまま残り、次の起動で回収されるまでトンネルを張り続ける。
	if r.vpn != nil {
		// 監視を先に止める。止める前に終わりを待つと、engine を畳む側と
		// 待たれる側が互いを待つ。
		if r.vpnCancel != nil {
			r.vpnCancel()
		}
		if r.vpnDone != nil {
			<-r.vpnDone
		}
		// 用意の途中の経路を打ち切る。打ち切らないと、StopAll がその起動を
		// 分単位で待つ。
		r.vpn.Close()
		stopping, cancel := context.WithTimeout(context.Background(), vpnStopTimeout)
		r.vpn.StopAll(stopping)
		cancel()
	}
	if r.vault != nil {
		r.vault.Lock()
	}
	return errors.Join(joined...)
}

// startVPNSupervisor は、経路の寿命を見る仕事を engine の寿命に結び付ける。
func (r *runtime) startVPNSupervisor(parent context.Context, logger *slog.Logger) {
	if r.vpn == nil {
		return
	}
	r.vpn.ExpectOrphanDiscard()
	watching, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	r.vpnCancel = cancel
	r.vpnDone = done
	go func() {
		defer close(done)
		superviseVPNSessions(watching, r.vpn, logger)
	}()
}

func (r *runtime) startAutoSync(parent context.Context) {
	if r.autoSync == nil {
		return
	}
	loop, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	r.autoCancel = cancel
	r.autoDone = done
	go func() {
		defer close(done)
		r.autoSync.Run(loop)
	}()
}

// newOrigin は、このインストールの不透明な識別子を発行する。
func newOrigin(random io.Reader) func() (string, error) {
	return func() (string, error) {
		if random == nil {
			random = rand.Reader
		}
		raw := make([]byte, 16)
		if _, err := io.ReadFull(random, raw); err != nil {
			return "", err
		}
		return hex.EncodeToString(raw), nil
	}
}
