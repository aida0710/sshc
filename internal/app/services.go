package app

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/application"
	"sshc/internal/diagnostics"
	"sshc/internal/keys"
	"sshc/internal/knownhosts"
	"sshc/internal/recent"
	"sshc/internal/remotekey"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

// engineServices は、engine がひとつのワークスペースの上に組む部品一式である。
type engineServices struct {
	workspace    *storage.Workspace
	transactions *storage.Manager
	config       *application.Service
	keys         *keys.Service
	diagnostics  *diagnostics.Service
	knownHosts   *knownhosts.Service
	passwords    *secret.Service
	remoteKeys   *remotekey.Service
	sync         *remotesync.Service
	autoSync     *remotesync.Auto
	recentStore  *recent.Store
	recent       *recent.Service
	terminals    *terminal.Registry
	ssh          sshParts
}

// newEngineServices は、~/.ssh をひとつ開いて、その上に engine の部品を組む。
func newEngineServices(dependencies Dependencies) (*engineServices, error) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, dependencies.Home)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	transactions := storage.NewManager(workspace, time.Now, dependencies.Random)
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

	// パスワード保管用の vault も設定のトランザクションマネージャを共有する。~/.ssh
	passwordService := secret.NewService(workspace, transactions, time.Now)
	recentStore := recent.NewStore(workspace, time.Now)

	// プロセス内 SSH クライアントの依存関係をここで一度だけ組み立てる。
	ssh := newSSHParts(configService, knownHostsService, workspace.Home(),
		storedPassphrase(passwordService, workspace.Root()), storedPassword(passwordService))
	recentService := recent.NewService(recentStore, func(alias string) (recent.Target, error) {
		target, err := ssh.target(alias)
		if err != nil {
			return recent.Target{}, err
		}
		return recent.Target{
			Alias: target.Alias, HostName: target.HostName, User: target.User, Port: target.Port,
		}, nil
	})
	probe := dependencies.Probe
	if probe == nil {
		probe = ssh.probe()
	}
	diagnosticsService.Authentication.Dial = probe

	// 公開鍵のリモート登録も同じ接続を通る。外部の ssh は起動しない。
	remoteRun := dependencies.RemoteRun
	if remoteRun == nil {
		remoteRun = ssh.run()
	}
	remoteKeyService := &remotekey.Service{Resolve: ssh.target, Run: remoteRun}

	keyService.SetStoredPassphrase(passwordService.KeyPassphraseFor)

	for _, manager := range []*storage.Manager{transactions, keyTransactions} {
		manager.Seal = passwordService.SealBackup
		manager.Unseal = passwordService.OpenBackup
	}
	// vault のロック状態は secret.Service が管理する。

	services := &engineServices{
		workspace: workspace, transactions: transactions,
		config: configService, keys: keyService, diagnostics: diagnosticsService,
		knownHosts: knownHostsService, passwords: passwordService,
		remoteKeys: remoteKeyService, recentStore: recentStore, recent: recentService, ssh: ssh,
	}
	services.sync, services.autoSync = buildSync(workspace, transactions, configService, passwordService, dependencies)
	services.terminals = buildTerminals(configService, dependencies)
	return services, nil
}

// buildSync は、リモートのスナップショットへ出入りする経路を組む。
func buildSync(
	workspace *storage.Workspace,
	transactions *storage.Manager,
	configService *application.Service,
	passwordService *secret.Service,
	dependencies Dependencies,
) (*remotesync.Service, *remotesync.Auto) {
	// スナップショットは、どのファイルが設定なのかを知る必要がある。それは Include
	syncService := remotesync.NewService(workspace, transactions,
		func() ([]string, error) { return configService.WorkspaceFiles() },
		func() string { return time.Now().UTC().Format(time.RFC3339) },
		newOrigin(dependencies.Random),
	)
	syncService.OpenVault = passwordService.TravelDocument
	syncService.SealVault = passwordService.AdoptTravelDocument
	syncService.VaultAdopted = passwordService.Reload

	autoSync := remotesync.NewAuto(syncService, remotesync.AutoInterval,
		func() string { return time.Now().UTC().Format(time.RFC3339) })
	autoSync.Enabled = func() bool {
		settings, err := passwordService.SyncSettings()
		return err == nil && settings.Auto
	}
	autoSync.Unattended = passwordService.Unattended
	autoSync.Key = func() (string, bool) {
		settings, err := passwordService.SyncSettings()
		if err != nil || settings.Key == "" {
			return "", false
		}
		return settings.Key, true
	}

	return syncService, autoSync
}

// buildTerminals は、埋め込みターミナルのセッション台帳を組む。
func buildTerminals(configService *application.Service, dependencies Dependencies) *terminal.Registry {
	starter := dependencies.TerminalStarter
	if starter == nil {
		starter = terminal.NewStarter()
	}
	terminals := &terminal.Registry{
		Start:      starter,
		Limits:     configService.TerminalLimits,
		Reconnects: configService.TerminalReconnects,
		Random:     dependencies.Random,
	}

	return terminals
}
