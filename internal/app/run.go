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
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/keys"
	"sshc/internal/platform"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/selfupdate"
	"sshc/internal/session"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
	"sshc/internal/validate"
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
	UI         fs.FS
	Logger     *slog.Logger
	Home       string
	Owner      handoff.Owner
	PID        int
	// Toolchain と KeyAgent は、鍵 vault とオペレーティングシステムとの境界。
	Toolchain    platform.Toolchain
	KeyAgent     platform.KeyAgent
	ScanHostKeys func(ctx context.Context, address string, timeout time.Duration) ([]ssh.PublicKey, error)
	Probe        func(ctx context.Context, alias string) (sshclient.Probe, error)
	RemoteRun    func(ctx context.Context, target sshclient.Target, command string, stdin []byte) (sshclient.Output, error)
	Updates      *selfupdate.Checker
	// Lookup は親の環境を読み、利用者のログインシェルを見つけるために使う。
	Lookup func(string) (string, bool)
	// TerminalStarter は PTY を確保する。nil の場合は既定実装を使用する。
	TerminalStarter terminal.Starter
	Environ         func() []string
	// SessionNow は、セッションマネージャがアクショントークンの失効に使う時計。
	SessionNow      func() time.Time
	ShutdownTimeout time.Duration
}

// DefaultShutdownTimeout は、承認済みの内側の締切である。
const DefaultShutdownTimeout = 4 * time.Second

// Readiness は、受け付けを始めた常駐がどんな状態かを述べる。
type Readiness struct {
	Owner handoff.Owner
	// Entrance は起動時に使用する UI URL である。
	Entrance    string
	VaultExists bool
}

func buildKeyService(workspace *storage.Workspace, dependencies Dependencies, configuration *application.Service) (*keys.Service, *storage.Manager) {
	transactions := storage.NewManager(workspace, time.Now, dependencies.Random)
	return keys.NewService(keys.ServiceOptions{
		Workspace:     workspace,
		Transactions:  transactions,
		Resolver:      storage.NewResolver(workspace),
		Catalogue:     keys.CatalogueReader{Toolchain: dependencies.Toolchain},
		Agent:         dependencies.KeyAgent,
		Now:           time.Now,
		Random:        dependencies.Random,
		ValidateGroup: configuration.ValidateDeclaredGroup,
	}), transactions
}

func Build(dependencies Dependencies, version string) (*httpserver.Server, string, error) {
	built, err := build(dependencies, version)
	return built.server, built.bootstrap, err
}

// runtime は、build が組み立てたもののうち、寿命の管理に要るものである。
type runtime struct {
	server     *httpserver.Server
	bootstrap  string
	document   handoff.Handoff
	terminals  *terminal.Registry
	passwords  *secret.Service
	autoSync   *remotesync.Auto
	autoCancel context.CancelFunc
	autoDone   chan struct{}
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

	listener, err := listenLoopback(dependencies.Listen, wanted, randomBelow)
	if err != nil {
		return runtime{}, fmt.Errorf("%w: %w", ErrListen, err)
	}

	sessions, bootstrap, err := session.NewManager(dependencies.Random)
	if err != nil {
		listener.Close()
		return runtime{}, fmt.Errorf("session: %w", err)
	}
	if dependencies.SessionNow != nil {
		sessions.Now = dependencies.SessionNow
	}

	configService := services.config
	keyService := services.keys
	diagnosticsService := services.diagnostics
	knownHostsService := services.knownHosts
	passwordService := services.passwords
	remoteKeyService := services.remoteKeys
	syncService := services.sync
	autoSync := services.autoSync
	terminals := services.terminals
	ssh := services.ssh
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
		ConnectAliases:    ssh.aliases,
		Sessions:          sessions,
		UI:                dependencies.UI,
		Version:           version,
		Owner:             dependencies.Owner,
		StopEngine:        dependencies.StopEngine,
		ProtocolVersion:   handoff.ProtocolVersion,
		Logger:            dependencies.Logger,
		Config:            configService,
		Keys:              keyService,
		Diagnostics:       diagnosticsService,
		KnownHosts:        knownHostsService,
		RemoteKeys:        remoteKeyService,
		Recent:            services.recent,
		SFTP:              services.sftp,
		Workspaces:        services.workspaces,
		Snippets:          services.snippets,
		Passwords:         passwordService,
		Connect:           ssh.connector(),
		ConnectAgent:      ssh.agentConnector(),
		ConnectionBinding: ssh.connectionBinding,
		ConnectionOpened: func(alias string) {
			if err := services.recentStore.Record(alias); err != nil && dependencies.Logger != nil {
				dependencies.Logger.Warn("record recent SSH connection", "alias", alias, "error", err)
			}
		},
		Sync:      syncService,
		AutoSync:  autoSync,
		Terminals: terminals,
		// SSH のプログラムはもう要らない。接続はこのプロセスの中で通信する。
		TerminalStartDirectory:    configService.TerminalStartDirectory,
		LoginShell:                func() (string, error) { return platform.LoginShell(dependencies.Lookup) },
		LocalShellProfiles:        func() []platform.ShellProfile { return platform.ShellProfiles(dependencies.Lookup) },
		TerminalLocalShellProfile: configService.TerminalLocalShellProfile,
		TerminalEnvironment: func() []string {
			if dependencies.Environ == nil {
				return nil
			}
			return platform.LoginEnvironment(dependencies.Environ())
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
	// handoff を書けないことは致命である。書けなかった常駐は、`sshc ssh <alias>`
	if err := handoff.Write(HandoffDir(dependencies.Home), document); err != nil {
		if removeErr := handoff.Remove(HandoffDir(dependencies.Home), document.Secret); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove the possibly published handoff: %w", removeErr))
		}
		listener.Close()
		return runtime{}, fmt.Errorf("publish the command-line handoff: %w", err)
	}
	return runtime{
		server:    server,
		bootstrap: bootstrap,
		document:  document,
		terminals: terminals,
		passwords: passwordService,
		autoSync:  autoSync,
	}, nil
}

func HandoffDir(home string) string {
	return filepath.Join(home, ".ssh", "sshc")
}

func Run(ctx context.Context, dependencies Dependencies, version string) error {
	asked, stopAsked := context.WithCancel(ctx)
	defer stopAsked()
	dependencies.StopEngine = stopAsked

	built, err := build(dependencies, version)
	if err != nil {
		return err
	}

	serveErrors := make(chan error, 1)
	go func() { serveErrors <- built.server.Serve() }()

	built.startAutoSync(asked)

	// すべての経路で HTTP サーバーの停止完了を待つ。
	stop := func(reason error) error {
		unwound := built.unwind(dependencies)
		return errors.Join(reason, unwound, <-serveErrors)
	}

	if dependencies.Announce != nil {
		exists := false
		if built.passwords != nil {
			if exists, err = built.passwords.Exists(); err != nil {
				return stop(fmt.Errorf("read the vault state: %w", err))
			}
		}
		readiness := Readiness{
			Owner:       dependencies.Owner,
			Entrance:    built.server.URL() + "/#bootstrap=" + built.bootstrap,
			VaultExists: exists,
		}
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
	forced := make(chan struct{})
	deadline := time.AfterFunc(timeout, func() {
		go r.terminals.ForceClose()
		go r.server.ForceClose()
		close(forced)
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
	if err := handoff.Remove(HandoffDir(dependencies.Home), r.document.Secret); err != nil {
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

	if r.passwords != nil {
		r.passwords.Lock()
	}
	return errors.Join(joined...)
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
