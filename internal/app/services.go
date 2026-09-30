package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/application"
	"sshc/internal/browserauth"
	"sshc/internal/diagnostics"
	"sshc/internal/keys"
	"sshc/internal/knownhosts"
	"sshc/internal/platform"
	"sshc/internal/recent"
	"sshc/internal/remotekey"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	sshcSFTP "sshc/internal/sftp"
	"sshc/internal/snippets"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/syncrestore"
	"sshc/internal/terminal"
	"sshc/internal/vpn"
	"sshc/internal/vpnprofile"
	terminalworkspace "sshc/internal/workspace"
)

// engineServices は、engine がひとつのワークスペースの上に組む部品一式である。
type engineServices struct {
	workspace    *storage.Workspace
	browserAuth  *browserauth.Store
	transactions *storage.Manager
	config       *application.Service
	keys         *keys.Service
	diagnostics  *diagnostics.Service
	knownHosts   *knownhosts.Service
	vault        *secret.Service
	remoteKeys   *remotekey.Service
	sync         *remotesync.Service
	autoSync     *remotesync.Auto
	recentStore  *recent.Store
	recent       *recent.Service
	sftp         *sshcSFTP.Service
	sftpPool     *sshcSFTP.RemotePool
	workspaces   *terminalworkspace.Service
	snippets     *snippets.Service
	terminals    *terminal.Registry
	vpn          *vpn.Manager
	vpnProfiles  *vpnprofile.Service
	ssh          sshParts
}

// newEngineServices は、~/.ssh をひとつ開いて、その上に engine の部品を組む。
func newEngineServices(dependencies Dependencies) (*engineServices, error) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, dependencies.Home)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	// handoffや履歴などのprivate stateを組み立てる前に、共通path walkerで
	// state directoryを確定する。起動外殻ごとのlockに安全検査を兼務させない。
	if err := workspace.EnsureDirectory(workspace.StateDir()); err != nil {
		return nil, fmt.Errorf("workspace state: %w", err)
	}
	transactions := storage.NewManager(workspace, time.Now, dependencies.Random)
	// 保持の上限を超えた変更の履歴と控えを消す。変更が完了するたびにも消すが、長く
	// 変更の無かったワークスペースで期間を過ぎたものは、ここで消す。消せなくても
	// 起動は止めない。
	if err := transactions.PruneHistory(); err != nil && dependencies.Logger != nil {
		dependencies.Logger.Error("prune the change history", "error", err)
	}
	configService := application.NewService(workspace, transactions)
	keyService, keyTransactions := buildKeyService(workspace, dependencies, configService)
	configService.SetKeyPassphraseVerifier(keyService)
	diagnosticsService := diagnostics.NewService(workspace, nil, application.LocalFactsFor(dependencies.Home))
	diagnosticsService.Resolver.GeneratedRegion = application.GeneratedRegion
	collectHostKeys := dependencies.ScanHostKeys
	if collectHostKeys == nil {
		collectHostKeys = func(ctx context.Context, address string, timeout time.Duration) ([]ssh.PublicKey, error) {
			return sshclient.ScanHostKeys(ctx, nil, address, timeout)
		}
	}
	knownHostsService := knownhosts.NewService(workspace, transactions,
		knownhosts.Scanner{Collect: collectHostKeys})

	// Vault も設定のトランザクションマネージャを共有する。Vault は ~/.ssh の下の
	// 管理対象ファイルのひとつにすぎないので、ジャーナルはひとつで足りる。
	vault := secret.NewService(workspace, transactions, time.Now)
	vault.SetIdleTimeout(configService.EngineSettings().VaultIdleTimeout(secret.IdleTimeout))
	configService.SetVault(vault)
	recentStore := recent.NewStore(workspace, time.Now)

	// ProxyCommand と docker は、ログインシェルの PATH で起動する。launchd や
	// systemd が起動した engine の PATH は短く、利用者がシェルの設定で足した場所を
	// 含まない。nil なら engine の環境をそのまま使う。
	var loginShellEnvironment func(context.Context) ([]string, error)
	if dependencies.Environ != nil {
		loginShellEnvironment = func(ctx context.Context) ([]string, error) {
			return platform.WithLoginShellPath(ctx, dependencies.Environ())
		}
	}
	dockerEnvironment := dependencies.DockerEnvironment
	if dockerEnvironment == nil {
		dockerEnvironment = loginShellEnvironment
	}

	// VPN 経路はこの engine が持つ。コンテナも中継のソケットも、この利用者の
	// ものだけを扱う。
	vpnManager := vpn.New(filepath.Join(workspace.Root(), vpnStateDirectory), os.Getuid(), dockerEnvironment)
	// 設定・秘密・経路をひとつの操作として扱う。engine の接続と HTTP API が同じものを使う。
	vpnProfiles := vpnprofile.New(vpnprofile.Dependencies{
		Configuration: configService, Vault: vault, Routes: vpnManager,
	})

	// プロセス内 SSH クライアントの依存関係をここで一度だけ組み立てる。
	inProcessSSH := newSSHParts(sshDependencies{
		config: configService, knownHosts: knownHostsService, home: workspace.Home(),
		passphrase:  storedPassphrase(vault, workspace.Root()),
		password:    storedPassword(vault),
		oneTimeCode: storedTOTP(vault),
		vpnRoute:    vpnRoute(vpnProfiles, vpnManager),
	})
	inProcessSSH.dialer.ProxyEnvironment = loginShellEnvironment
	recentService := recent.NewService(recentStore, func(alias string) (recent.Target, error) {
		target, err := inProcessSSH.target(alias)
		if err != nil {
			return recent.Target{}, err
		}
		return recent.Target{
			Alias: target.Alias, HostName: target.HostName, User: target.User, Port: target.Port,
		}, nil
	})
	sftpPool := sshcSFTP.NewRemotePool(inProcessSSH.sftp())
	sftpService := &sshcSFTP.Service{Open: sftpPool.Open, ConnectionLimit: sftpConnectionLimit(vault, inProcessSSH.target)}
	workspaceService := terminalworkspace.NewService(terminalworkspace.NewStore(workspace), time.Now, dependencies.Random)
	probe := dependencies.Probe
	if probe == nil {
		probe = inProcessSSH.probe()
	}
	diagnosticsService.Authentication.Dial = probe

	// 公開鍵のリモート登録も同じ接続を通る。外部の ssh は起動しない。
	remoteRun := dependencies.RemoteRun
	if remoteRun == nil {
		remoteRun = inProcessSSH.run()
	}
	remoteKeyService := &remotekey.Service{Resolve: inProcessSSH.target, Run: remoteRun}
	snippetStore := snippets.NewStore(workspace, snippets.Protection{
		Seal: vault.SealDocument,
		Open: func(contents []byte) ([]byte, error) {
			plaintext, err := vault.OpenDocument(contents)
			if errors.Is(err, secret.ErrNotAVault) {
				return nil, snippets.ErrNotEncrypted
			}
			return plaintext, err
		},
		WithMutation: func(mutation func() error) error {
			return vault.WithStableSnapshot(func() error {
				return transactions.WithSnapshot(mutation)
			})
		},
	})
	if err := vault.RegisterProtectedDocument(secret.ProtectedDocument{
		Path: snippetStore.Path(), Validate: snippetStore.ValidateDocument,
	}); err != nil {
		return nil, err
	}
	configService.SetStartupRenamer(snippetStore)
	snippetService := snippets.NewService(snippets.Options{
		Repository: snippetStore,
		Resolve: func(alias string) (snippets.Resolution, error) {
			target, err := inProcessSSH.target(alias)
			if err != nil {
				return snippets.Resolution{}, err
			}
			return snippets.Resolution{
				Target: snippets.Target{
					Alias: target.Alias, HostName: target.HostName, User: target.User, Port: target.Port,
					Route: snippetRoute(target),
				},
				Binding: target.AuthenticationBinding(),
				Run: func(ctx context.Context, command string) (snippets.CommandOutput, error) {
					// Snippet のコマンドは長く走ることがある（apt-get update、バックアップ）。
					// 上限は置かず、job の取り消しと engine の停止だけで止める。
					output, err := remoteRun(ctx, target, sshclient.Command{Line: command})
					return snippets.CommandOutput{
						ExitCode: output.ExitCode, Stdout: output.Stdout, Stderr: output.Stderr, Truncated: output.Truncated,
					}, err
				},
			}, nil
		},
		Now: time.Now, Random: dependencies.Random,
	})

	keyService.SetStoredPassphrase(vault.KeyPassphraseFor)

	reloadVaultAfterRecovery := func(paths []string) {
		if err := vault.ReloadAfterRecovery(paths); err != nil && dependencies.Logger != nil {
			dependencies.Logger.Error("reload vault after recovering a transaction", "error", err)
		}
	}
	for _, manager := range []*storage.Manager{transactions, keyTransactions} {
		manager.Seal = vault.SealBackup
		manager.Unseal = vault.OpenBackup
		manager.AfterRecovery = reloadVaultAfterRecovery
	}
	// 古いバージョンが残したスニペットと控えを移せなくても、ロックの解除は止めない。
	// 何度やっても移せないものに気付けるよう、ログに残す。起動時の自動のロック解除にも
	// 効くよう、AutoUnlock より前に取り付ける。
	if dependencies.Logger != nil {
		vault.SetReportKeyBoundMigrationFailure(func(err error) {
			dependencies.Logger.Error("migrate the snippets and backups left by an older version", "error", err)
		})
	}
	if err := vault.AutoUnlock(); err != nil && !errors.Is(err, secret.ErrUnsupportedVersion) {
		return nil, fmt.Errorf("auto-unlock vault: %w", err)
	}
	// vault のロック状態は secret.Service が管理する。

	services := &engineServices{
		workspace: workspace, transactions: transactions,
		browserAuth: browserauth.NewStore(workspace, dependencies.Random),
		config:      configService, keys: keyService, diagnostics: diagnosticsService,
		knownHosts: knownHostsService, vault: vault,
		remoteKeys: remoteKeyService, recentStore: recentStore, recent: recentService,
		sftp: sftpService, sftpPool: sftpPool, workspaces: workspaceService, snippets: snippetService,
		vpn: vpnManager, vpnProfiles: vpnProfiles, ssh: inProcessSSH,
	}
	services.sync, services.autoSync, err = buildSync(workspace, transactions, vault, snippetStore, dependencies)
	if err != nil {
		return nil, err
	}
	notifyLocalChange := func(operation string) {
		// Remote apply and local synchronization bookkeeping are not user edits.
		// Scheduling them would immediately echo a received snapshot back to S3.
		if !strings.HasPrefix(operation, "sync.") {
			services.autoSync.NotifyLocalChange()
		}
	}
	transactions.AfterCommit = notifyLocalChange
	keyTransactions.AfterCommit = notifyLocalChange
	snippetStore.SetAfterChange(services.autoSync.NotifyLocalChange)
	services.terminals = buildTerminals(configService, dependencies)
	return services, nil
}

func snippetRoute(target sshclient.Target) []snippets.RouteHop {
	route := make([]snippets.RouteHop, 0, len(target.Jump)+1)
	for _, hop := range target.Jump {
		route = append(route, snippetRoute(hop)...)
	}
	return append(route, snippets.RouteHop{
		Alias: target.Alias, HostName: target.HostName, User: target.User, Port: target.Port,
		ProxyCommand: target.ProxyCommand, StrictHostKey: target.Strict,
		Authentication: append([]string(nil), target.Methods.Order()...),
		IdentityFiles:  append([]string(nil), target.Identities...), IdentitiesOnly: target.IdentitiesOnly,
		HostKeyAlgorithms: append([]string(nil), target.HostKeyAlgorithms...),
	})
}

// buildSync は、リモートのスナップショットへ出入りする経路を組む。
func buildSync(
	workspace *storage.Workspace,
	transactions *storage.Manager,
	vault *secret.Service,
	snippetStore *snippets.Store,
	dependencies Dependencies,
) (*remotesync.Service, *remotesync.Auto, error) {
	syncService, err := remotesync.NewIntegratedService(workspace, transactions,
		func() string { return time.Now().UTC().Format(time.RFC3339) },
		newOrigin(dependencies.Random),
		remotesync.IntegrationHooks{
			OpenVault:          vault.TravelDocument,
			SealVault:          vault.AdoptTravelDocument,
			EmptyVaultDocument: vault.EmptyTravelDocument,
			VaultAdopted:       vault.Reload,
			KeyedTravelDigest:  vault.KeyedTravelDigest,
			OpenSnippets:       snippetStore.TravelDocument,
			SealSnippets:       snippetStore.AdoptTravelDocument,
			SecretMutation:     vault.WithStableSnapshot,
			StableSnapshot: func(snapshot func() error) error {
				return vault.WithStableSnapshot(func() error {
					return transactions.WithSnapshot(snapshot)
				})
			},
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("remote sync integration: %w", err)
	}

	// 同期先のアクセスキーとシークレットは Vault の中にある。Vault が閉じたら
	// メモリからも手放す。
	vault.SetAfterLock(syncService.Forget)
	// 同期状態に残す Vault 文書の digest は Vault の鍵で鍵付きにしてある。鍵を変える
	// 変更は、同じトランザクションでその値を新しい鍵へ移す。
	vault.SetTravelDigestRekey(syncService.RekeyTravelDigest)
	// 鍵付きにする前の版が書いた素の digest は、Vault を開いたときに移す。起動時に
	// 自動でロックを解除した Vault は、この配線より前に開いているので、ここで一度移す。
	migrateTravelDigest := func() {
		if err := syncService.MigrateTravelDigest(); err != nil && !errors.Is(err, secret.ErrLocked) && dependencies.Logger != nil {
			dependencies.Logger.Error("migrate the vault digest in the synchronization state", "error", err)
		}
	}
	vault.SetAfterUnlock(migrateTravelDigest)
	migrateTravelDigest()

	autoSync := remotesync.NewAuto(syncService, remotesync.AutoInterval,
		func() string { return time.Now().UTC().Format(time.RFC3339) })
	if dependencies.Logger != nil {
		autoSync.ReportFailure = func(stage string, err error) {
			dependencies.Logger.Error("automatic synchronization failed",
				"stage", stage, "code", remotesync.FailureCode(err), "error", err)
		}
	}
	autoSync.Enabled = func() bool {
		settings, err := vault.SyncSettings()
		return err == nil && settings.Auto
	}
	autoSync.Unattended = vault.Unattended
	autoSync.Key = func() (string, bool) {
		settings, err := vault.SyncSettings()
		if err != nil || settings.Key == "" {
			return "", false
		}
		return settings.Key, true
	}
	autoSync.Prepare = func() { syncrestore.FromVault(syncService, vault, nil) }

	return syncService, autoSync, nil
}

// buildTerminals は、埋め込みターミナルのセッション台帳を組む。
func buildTerminals(configService *application.Service, dependencies Dependencies) *terminal.Registry {
	starter := dependencies.TerminalStarter
	if starter == nil {
		starter = terminal.NewStarter()
	}
	terminals := &terminal.Registry{
		Start:          starter,
		Limits:         configService.TerminalLimits,
		ReconnectLimit: configService.TerminalReconnects,
		Random:         dependencies.Random,
	}

	return terminals
}
