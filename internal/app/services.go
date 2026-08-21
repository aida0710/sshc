package app

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/application"
	"sshc/internal/diagnostics"
	"sshc/internal/handoff"
	"sshc/internal/keys"
	"sshc/internal/knownhosts"
	"sshc/internal/remotekey"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

// engineServices は、engine がひとつのワークスペースの上に組む部品一式である。
//
// **ここに listener は無い。** ネットワークを開くのと、~/.ssh の上にサービスを組む
// のは別のことで、混ぜると「失敗したときに何を閉じるのか」が局面ごとに変わる。
// build がそれを引き受け、ここは組むだけである。
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
	terminals    *terminal.Registry
	ssh          sshParts
}

// newEngineServices は、~/.ssh をひとつ開いて、その上に engine の部品を組む。
//
// 順に依存していく。設定エンジンが先で、鍵と known_hosts と保管庫がその上に乗り、
// プロセス内 SSH がそれらを束ね、同期と端末が最後に来る。**その順序が、この関数が
// 3 つに割れない理由である。**
func newEngineServices(dependencies Dependencies) (*engineServices, error) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, dependencies.Home)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	// Random は並行利用に耐えなければならない。セッションマネージャと二つの
	// トランザクションマネージャが読むからだ。本番では crypto/rand を渡す。
	transactions := storage.NewManager(workspace, time.Now, dependencies.Random)
	configService := application.NewService(workspace, transactions)
	keyService, keyTransactions := buildKeyService(workspace, dependencies, configService)
	configService.SetKeyPassphraseVerifier(keyService)
	diagnosticsService := diagnostics.NewService(workspace, nil)
	// 生成領域の書式を知っているのは設定エンジンであり、それを尋ねられるのは
	// diagnostics ではなくここである。あちらは internal/application を import
	// しない。これがないと、宣言済みで空のグループのために書かれた Include が
	// include_no_match として報告され、application 層が group_empty を注意として
	// 出さないと決めた判断が、別の名前で破られる。
	diagnosticsService.Resolver.GeneratedRegion = application.GeneratedRegion
	// ユーザーに見せるコマンドはこのバイナリと alias なので、このバイナリがどこに
	// あるかを知る必要がある。アプリケーションの内側でそれを割り出せるものはない。
	// エントリポイントが一度だけ解決して渡す。
	// known_hosts は設定のトランザクションマネージャを共有する。どちらも ~/.ssh 配下の
	// 通常の管理対象ファイルを書くので、ジャーナルはひとつで足りる。
	// 鍵を集めるのはこのプロセスである。ssh-keyscan は起こさない。
	collectHostKeys := dependencies.ScanHostKeys
	if collectHostKeys == nil {
		collectHostKeys = func(ctx context.Context, address string, timeout time.Duration) ([]ssh.PublicKey, error) {
			return sshclient.ScanHostKeys(ctx, nil, address, timeout)
		}
	}
	knownHostsService := knownhosts.NewService(workspace, transactions,
		knownhosts.Scanner{Collect: collectHostKeys})

	// パスワード保管用の vault も設定のトランザクションマネージャを共有する。~/.ssh
	// 配下のもうひとつの通常の管理対象ファイルにすぎず、ジャーナルはひとつで足り、
	// ワークスペースが持つ他のすべてと一緒に移動する。
	passwordService := secret.NewService(workspace, transactions, time.Now)
	if dependencies.Owner == handoff.OwnerDesktop {
		passwordService.SetIdleTimeout(secret.StayOpen)
	}
	// **時計で閉じるのは、OS の境界が無いところだけである。**
	//
	// デスクトップの engine はアプリの子で、アプリを終えれば道連れに死ぬ——vault は
	// メモリの中だけなので、そこで消える。蓋を閉じたノートは、開ければ OS が
	// ログインパスワードを訊く。そこへ 8 時間の時計を重ねても、増える安全は
	// わずかで、確実に増えるのは再入力の回数である。
	//
	// 画面の無い機械は違う。`sshc headless` は systemd の下で何週間も走り、
	// 蓋も画面ロックも無い。**そこでは、これが唯一の歯止めである。**

	// プロセス内で SSH を話すのに要るものは、ここで一度だけ組み立てる。
	// 対話セッションも認証テストも、同じ鍵・同じ known_hosts・同じ解決器を使う。
	ssh := newSSHParts(configService, knownHostsService, workspace.Home(),
		storedPassphrase(passwordService, workspace.Root()), storedPassword(passwordService))
	probe := dependencies.Probe
	if probe == nil {
		probe = ssh.probe()
	}
	diagnosticsService.Authentication.Dial = probe

	// 公開鍵のリモート登録も同じ接続を通る。**外部の ssh は起こさない。**
	remoteRun := dependencies.RemoteRun
	if remoteRun == nil {
		remoteRun = ssh.run()
	}
	remoteKeyService := &remotekey.Service{Resolve: ssh.target, Run: remoteRun}

	// パスフレーズが保存されている鍵は、二段階ではなく一度の操作でエージェントに
	// 追加される。この参照関数を internal/keys に import させず、ここで取り付けるのは、
	// 同パッケージが「グループとは何か」を設定エンジンに尋ねないのと同じく、「秘密が
	// どこにあるか」を secret パッケージに尋ねてはならないからだ。
	keyService.SetStoredPassphrase(passwordService.KeyPassphraseFor)

	// 世代バックアップはすべて暗号文である。このアプリケーションが置き換えるファイル
	// の以前の内容は秘密鍵かもしれない。だからこそ、それを生みうる書き込みは以前は
	// バックアップをまったく取らないよう要求しており、その結果、取り消すことが決して
	// できなかった。封をすることで取り消しを取り戻す。マネージャに渡すのは vault では
	// なく二つの関数なので、ストレージ層は秘密について何も知らないままでいられる。
	// そして、アプリケーションがマスターパスワードの後ろにあることが、その二つの関数を
	// 常に利用可能にしている。
	//
	// **このワークスペースの上で書くマネージャは、ひとつ残らず封をされる。** 鍵 vault
	// のマネージャは設定のそれとは別物で、しかも置き換える対象は秘密鍵そのものである。
	// 封が片方にしか付いていなかった間、パスフレーズの変更は以前の平文の鍵を世代
	// バックアップに残していた。
	for _, manager := range []*storage.Manager{transactions, keyTransactions} {
		manager.Seal = passwordService.SealBackup
		manager.Unseal = passwordService.OpenBackup
	}
	// **錠前は差すだけである。** 保管庫は、預かりが在るかどうかを自分で見る。
	if dependencies.Biometric != nil {
		passwordService.SetGuardian(dependencies.Biometric)
	}

	services := &engineServices{
		workspace: workspace, transactions: transactions,
		config: configService, keys: keyService, diagnostics: diagnosticsService,
		knownHosts: knownHostsService, passwords: passwordService,
		remoteKeys: remoteKeyService, ssh: ssh,
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
	// グラフが答える問いである。答えを渡す形にすれば、依存の向きは正しいまま保たれる
	// ── internal/remotesync は、設定サービスのものを何ひとつ import して
	// いない。
	syncService := remotesync.NewService(workspace, transactions,
		func() ([]string, error) { return configService.WorkspaceFiles() },
		func() string { return time.Now().UTC().Format(time.RFC3339) },
		newOrigin(dependencies.Random),
	)
	// **保管庫は、封ではなく中身として旅をする。** ファイルのまま運べば、それは
	// この端末のマスターパスワードで封じられたものであり、受け取った端末はそれ
	// 以降、送り主のパスワードでしか開けなくなる——マスターパスワードを端末ごとに
	// 変えられない、という詰みの本体はそこだった。両替はここで繋ぐ。同期は
	// secret を import しない。
	syncService.OpenVault = passwordService.TravelDocument
	syncService.SealVault = passwordService.AdoptTravelDocument
	syncService.VaultAdopted = passwordService.Reload

	// 押さなくても進む巡回。**必要なものはすべて保管庫の中にある**ので、
	// 閉じている間は何も読めず、何も起きない。それがこの機能の唯一の条件である。
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
	// 埋め込みターミナルの PTY は、この常駐プロセスの中で存続する。ブラウザの
	// タブを閉じてもリロードしてもセッションは生きており、終わるのは子プロセスが
	// 終了したとき、人が閉じたとき、このプロセスが終了したときだけである。
	//
	// 上限を読むのは開くときだけなので、metadata を書き換えても、すでに開いて
	// いるセッションが閉じられることはない。
	starter := dependencies.TerminalStarter
	if starter == nil {
		starter = terminal.NewStarter()
	}
	terminals := &terminal.Registry{
		Start:  starter,
		Limits: configService.TerminalLimits,
		Random: dependencies.Random,
	}

	return terminals
}
