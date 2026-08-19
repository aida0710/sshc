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
	"sshc/internal/diagnostics"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/keys"
	"sshc/internal/knownhosts"
	"sshc/internal/platform"
	"sshc/internal/remotekey"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/selfupdate"
	"sshc/internal/session"
	"sshc/internal/sshclient"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

type ListenFunc func(network, address string) (net.Listener, error)

// unsafeAliasWarning は、`sshc <alias>` がこの先で止まる理由を先に言う。
//
// 安全な文字集合の外にある alias は、ssh を起こす直前に platform 層が拒否する。
// その拒否だけを見ると、打った本人には何が悪かったのか分からない。
const unsafeAliasWarning = "This alias contains characters that could change the meaning of a command line, so this connection will be refused."

type Dependencies struct {
	Random io.Reader
	// Announce は、この常駐が受け付けられる状態になったことを伝える。
	//
	// **ブラウザは開かない。** 画面はデスクトップの外殻が出すので、この
	// プロセスが既定のブラウザを起こす理由はもう無い。nil なら何も言わない
	// ——ログへトークンを落とさないための、自動化からの明示的な選択である。
	//
	// 入口の URL を運ぶのは desktop の owner のときだけである。headless に
	// 渡す Readiness は URL を持たない——**持たせれば、それを出さない約束は
	// 呼び出し側の注意深さに委ねられる。**
	Announce func(Readiness) error
	Listen   ListenFunc
	UI       fs.FS
	Logger   *slog.Logger
	// Home はユーザーのホームディレクトリ。オペレーティングシステムから読んでよいのは
	// cmd/sshc だけで、テストはいずれも一時ディレクトリを注入する。
	Home string
	// Owner と PID は handoff を発行した engine を特定する。ここで OS を読むと、
	// テストや将来の埋め込み実行が意図しないプロセスを名乗るため、呼び出し元が
	// 正しい実行主体を明示する。
	Owner handoff.Owner
	PID   int
	// Toolchain と KeyAgent は、鍵 vault とオペレーティングシステムとの境界。
	//
	// **このアプリケーションは OpenSSH のプログラムを一つも実行しない。**
	// Toolchain に残っているのは ssh-keygen だけで、それも走らせるのは利用者で
	// ある——見つかるかどうかで、ハードウェア鍵の項目を一覧に出してよいかを
	// 決める。KeyAgent が nil の場合、エージェント登録は到達できるエージェントが
	// ないと報告する。どちらも致命的ではないので、プロセスは他のすべての面を
	// 提供し続ける。
	Toolchain platform.Toolchain
	KeyAgent  platform.KeyAgent
	// Biometric は、この OS の錠前である。nil なら、この端末に生体認証の道は
	// 無い——画面はトグルを出さず、解錠は今まで通りパスワードだけになる。
	Biometric secret.Guardian
	// ScanHostKeys と Probe は、ネットワークへ出る 2 つの継ぎ目である。nil なら
	// internal/sshclient がこのプロセスの中で話す。Runner と同じ性質のものであり、
	// **検査がネットワークへ出ないようにするためにここにある。**
	ScanHostKeys func(ctx context.Context, address string, timeout time.Duration) ([]ssh.PublicKey, error)
	Probe        func(ctx context.Context, alias string) (sshclient.Probe, error)
	RemoteRun    func(ctx context.Context, target sshclient.Target, command string, stdin []byte) (sshclient.Output, error)
	// Updates はプロジェクトのリリースを調べる。nil なら何も提示しない。リリースで
	// ないビルドはそうあるべきである。
	Updates *selfupdate.Checker
	// Lookup は親の環境を読み、利用者のログインシェルを見つけるために使う。
	// os.LookupEnv を渡してよいのは cmd/sshc だけ。nil ならこのプロセスの環境を
	// 継ぐ形になり、テストにはそれが向く。
	Lookup func(string) (string, bool)
	// TerminalStarter は PTY を確保する継ぎ目である。nil なら本物を確保する。
	// ハードニングのスイートはここを差し替え、プロセスを一つも起こさずに
	// 「拒否された要求が端末を開いていないこと」を表明する。
	TerminalStarter terminal.Starter
	// Environ は、埋め込みターミナルのセッションが継ぐ環境である。これは利用者が
	// 自分で行ったであろう接続なので、検査が使う最小環境ではなく本人の環境を継ぐ。
	// 本番では os.Environ で、nil ならセッションはこのプロセスの環境を継承する。
	Environ func() []string
	// SessionNow は、セッションマネージャがアクショントークンの失効に使う時計。
	// 本番では nil で、time.Now が使われる。ハードニングのスイートはこれを設定し、
	// sleep せずにトークンを老化させる。
	SessionNow func() time.Time
	// ShutdownTimeout は、graceful な後始末に与える猶予である。0 なら
	// defaultShutdownTimeout。
	//
	// **外側の強制終了より確実に短くなければならない。** desktop の外殻は
	// stdin を閉じた時点から 5 秒で数え始めるので、Go 側のタイマーは必ずその
	// 後に始まる。等しい値を置けば、内側の強制停止に到達する前に外から殺される。
	// テストは秒を待たずに済むよう、ここへ短い値を注入する。
	ShutdownTimeout time.Duration
}

// defaultShutdownTimeout は、承認済みの内側の締切である。
const defaultShutdownTimeout = 4 * time.Second

// Readiness は、受け付けを始めた常駐がどんな状態かを述べる。
type Readiness struct {
	Owner handoff.Owner
	// DesktopURL は OwnerDesktop のときだけ空でない。
	DesktopURL string
	// VaultExists は、まだ作られていない vault と、作られていて施錠されている
	// vault を分ける。headless の案内はこれだけで決まる。
	VaultExists bool
}

// buildKeyService は、設定エンジンが使うのと同じワークスペース上に鍵 vault を
// 用意する。
//
// これは意図的に自前の storage.Manager を取る。application.NewService は渡された
// マネージャに設定バリデータを取り付け、そのバリデータはトランザクションが書く
// すべてのファイルを ssh_config として解析する。鍵 vault が書くのは秘密鍵、公開鍵、
// そして JSON のごみ箱マニフェストで、どれも設定ではなく、マニフェストは構文エラー
// として即座に拒否されてしまう。ひとつのワークスペースに二つのマネージャを置いても
// 安全である。Manager は呼び出しをまたいで可変状態を持たず、各トランザクションは
// 自身のタイムスタンプとランダムな接尾辞で識別されるため、ジャーナルと履歴はひと
// 続きの一貫したストリームであり続ける。
// インターフェースではなく具体型を返すのは、vault ができたあとに配線側が保存済み
// パスフレーズの参照関数を取り付けるためである。それでも、必要な場所では
// httpserver.KeyService を満たす。
func buildKeyService(workspace *storage.Workspace, dependencies Dependencies, configuration *application.Service) *keys.Service {
	transactions := storage.NewManager(workspace, time.Now, dependencies.Random)
	return keys.NewService(keys.ServiceOptions{
		Workspace:    workspace,
		Transactions: transactions,
		Resolver:     storage.NewResolver(workspace),
		Catalogue:    keys.CatalogueReader{Toolchain: dependencies.Toolchain},
		Agent:        dependencies.KeyAgent,
		Now:          time.Now,
		Random:       dependencies.Random,
		// グループとは何かは設定エンジンの領分であり、その宣言は ~/.ssh/config から
		// 読まれる。鍵 vault は自分で決めずに尋ねる。そのため鍵は、存在するグループへ
		// しか生成できない。
		ValidateGroup: configuration.ValidateDeclaredGroup,
	})
}

// Build はすべての依存を HTTP サーバーへ配線するが、サーブはしない。UI が提示
// しなければならないワンタイムのブートストラップトークンを返す。
//
// Run は Build を呼んでからサーブする。ハードニングのスイートは Build を直接
// 呼ぶので、その表明は、出荷されるバイナリが使うのと同じルーティング表・同じ
// ミドルウェア・同じハンドラ構築に対して走る。ずれていきかねない手作りの部分
// 集合に対してではない。
func Build(dependencies Dependencies, version string) (*httpserver.Server, string, error) {
	built, err := build(dependencies, version)
	return built.server, built.bootstrap, err
}

// runtime は、build が組み立てたもののうち、寿命の管理に要るものである。
type runtime struct {
	server    *httpserver.Server
	bootstrap string
	document  handoff.Handoff
	terminals *terminal.Registry
	passwords *secret.Service
	autoSync  *remotesync.Auto
}

// build は Run が後片付けに使う秘密も返す。ファイルを読み直すと、その間に別の
// 実行が公開した秘密を自分のものと誤認して消せるためである。
func build(dependencies Dependencies, version string) (runtime, error) {
	listener, err := dependencies.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return runtime{}, fmt.Errorf("listen: %w", err)
	}

	sessions, bootstrap, err := session.NewManager(dependencies.Random)
	if err != nil {
		listener.Close()
		return runtime{}, fmt.Errorf("session: %w", err)
	}
	if dependencies.SessionNow != nil {
		sessions.Now = dependencies.SessionNow
	}

	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, dependencies.Home)
	if err != nil {
		listener.Close()
		return runtime{}, fmt.Errorf("workspace: %w", err)
	}
	// Random は並行利用に耐えなければならない。セッションマネージャと二つの
	// トランザクションマネージャが読むからだ。本番では crypto/rand を渡す。
	transactions := storage.NewManager(workspace, time.Now, dependencies.Random)
	configService := application.NewService(workspace, transactions)
	keyService := buildKeyService(workspace, dependencies, configService)
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

	// プロセス内で SSH を話すのに要るものは、ここで一度だけ組み立てる。
	// 対話セッションも認証テストも、同じ鍵・同じ known_hosts・同じ解決器を使う。
	ssh := newSSHParts(configService, passwordService, knownHostsService,
		workspace.Home(), workspace.Root())
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
	transactions.Seal = passwordService.SealBackup
	transactions.Unseal = passwordService.OpenBackup
	// **錠前は差すだけである。** 保管庫は、預かりが在るかどうかを自分で見る。
	if dependencies.Biometric != nil {
		passwordService.SetGuardian(dependencies.Biometric)
	}

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

	// `sshc <alias>` は、動作中のアプリケーションを見つけるためにこれを読む。秘密は
	// ここで実行ごとに発行され、リスナーが立ち上がったあとに書かれる。そのため、
	// 書かれた URL は実際に応答する URL になる。
	cliSecret, err := handoff.Mint(dependencies.Random)
	if err != nil {
		listener.Close()
		return runtime{}, err
	}

	server, err := httpserver.New(httpserver.Options{
		Listener:  listener,
		CLISecret: cliSecret,
		Updates:   dependencies.Updates,
		// alias はコマンドラインでも検査されるが、その拒否が起きるのは ssh を
		// 起こす直前である。ここで先に一言置くのは、何が理由で止まるのかを
		// 打った本人に伝えるためだ。
		ConnectWarnings: func(alias string) []string {
			if err := platform.ValidateAlias(alias); err != nil {
				return []string{unsafeAliasWarning}
			}
			return nil
		},
		// 保存済みパスワードを渡す相手を、この接続に現れる alias に限るための
		// 一覧である。ProxyJump の手前に立つホストもそこに入る。
		ConnectAliases:  ssh.aliases,
		Sessions:        sessions,
		UI:              dependencies.UI,
		Version:         version,
		Owner:           dependencies.Owner,
		ProtocolVersion: handoff.ProtocolVersion,
		Logger:          dependencies.Logger,
		Config:          configService,
		Keys:            keyService,
		Diagnostics:     diagnosticsService,
		KnownHosts:      knownHostsService,
		RemoteKeys:      remoteKeyService,
		Passwords:       passwordService,
		Connect:         ssh.connector(),
		Sync:            syncService,
		AutoSync:        autoSync,
		Terminals:       terminals,
		// SSH のプログラムはもう要らない。**接続はこのプロセスの中で話す。**
		// PTY を確保するのはローカルシェルだけである。
		TerminalStartDirectory: configService.TerminalStartDirectory,
		LoginShell:             func() (string, error) { return platform.LoginShell(dependencies.Lookup) },
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

	// プロセスがサーブを始める場所ではなくここで書くのは、ここで URL が判明するから
	// であり、また、構築済みのサーバーは何かが接続できるサーバーだからである。
	// 強制終了されたプロセスが残していったコピーは、何も待ち受けていないポートを、
	// 誰も受け付けない秘密とともに指しているだけなので、これを取り除くのは保証では
	// なく後片付けである。
	document := handoff.Handoff{
		SchemaVersion:   handoff.SchemaVersion,
		URL:             server.URL(),
		Secret:          cliSecret,
		Owner:           dependencies.Owner,
		PID:             dependencies.PID,
		Version:         version,
		ProtocolVersion: handoff.ProtocolVersion,
	}
	// **handoff を書けないことは致命である。** 書けなかった常駐は、`sshc <alias>`
	// から見えないまま生きていることになり、その状態を警告ひとつで通り過ぎると、
	// 2 台目が同じ名簿を書きに来る。
	//
	// そして書き込みの結果は不定でありうる——原子的な置き換えが成功したあとで
	// ディレクトリの同期に失敗しうる。だから**どの失敗のあとでも**、自分の秘密で
	// 認証する削除を試みる。置き換えが起きていなければ、あるいは別の秘密の名簿が
	// 置かれていれば、その削除は何も消さない。
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

// HandoffDir は、動作中のアプリケーションが `sshc <alias>` の読むファイルを置く
// 場所。このアプリケーション自身のものが置かれる、他と同じ状態ディレクトリで
// ある。
func HandoffDir(home string) string {
	return filepath.Join(home, ".ssh", "sshc")
}

func Run(ctx context.Context, dependencies Dependencies, version string) error {
	built, err := build(dependencies, version)
	if err != nil {
		return err
	}

	serveErrors := make(chan error, 1)
	go func() { serveErrors <- built.server.Serve() }()

	// 巡回は、この実行と同じ寿命を持つ。**止めるのは ctx である**——後始末の
	// 段に足すと、そこへ辿り着くまでのあいだ、畳んでいる最中のワークスペースへ
	// 別のマシンのスナップショットを置きにいくことがありうる。
	loop, stopLoop := context.WithCancel(ctx)
	defer stopLoop()
	go built.autoSync.Run(loop)

	// **どの経路でも、serving が本当に止まるまで返らない。**
	//
	// listener を閉じるのは Serve が戻るときである。ここを待たずに返ると、
	// 呼び出し側は engine lock を手放しにかかり、その一瞬まだポートが握られて
	// いる。次の 1 台がそこで bind に失敗する。
	stop := func(reason error) error {
		unwound := built.unwind(dependencies)
		return errors.Join(reason, unwound, <-serveErrors)
	}

	// listener が bind されていることが受付開始の境界である。Serve を起こしてから
	// announce するまでに待つものは無い。
	if dependencies.Announce != nil {
		exists := false
		if built.passwords != nil {
			if exists, err = built.passwords.Exists(); err != nil {
				return stop(fmt.Errorf("read the vault state: %w", err))
			}
		}
		readiness := Readiness{Owner: dependencies.Owner, VaultExists: exists}
		if dependencies.Owner == handoff.OwnerDesktop {
			readiness.DesktopURL = built.server.URL() + "/#bootstrap=" + built.bootstrap
		}
		if err := dependencies.Announce(readiness); err != nil {
			return stop(fmt.Errorf("announce the entrance: %w", err))
		}
	}

	select {
	case err := <-serveErrors:
		// Serve が自分から戻った。後始末はそれでも全部通る。
		return errors.Join(err, built.unwind(dependencies))
	case <-ctx.Done():
		return stop(nil)
	}
}

// unwind は、engine lock を握ったまま通る唯一の後始末である。
//
// 順序は承認済みのものであり、**どの段が失敗しても後ろの段は走る。**
//
//  1. 内側の締切を、後始末の進み具合とは無関係に張る。
//  2. 新しい変更と Upgrade を断り、配ったリクエスト用 context を取り消す。
//  3. 自分の秘密で認証して handoff を消す。
//  4. terminal と HTTP の両方へ graceful を頼む。どちらも送り出して即座に返る。
//  5. 二つの合流を同時に始める。締切より前に揃えば強制停止は起きない。
//  6. 締切に達したら、terminal と HTTP の強制停止を別々の goroutine で始める。
//     **どちらかを待ってからもう一方、ではない。**
//  7. 強制のあとも、全部が合流するまで待つ。部分的な合流で錠を手放さない。
//  8. vault を施錠する。両方の壁を越えた後だけである。
func (r runtime) unwind(dependencies Dependencies) error {
	timeout := dependencies.ShutdownTimeout
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}
	// **締切は後始末の goroutine から独立している。** 手前の段が予想外に
	// 詰まっても、強制停止の一斉開始はそれでも起きる。
	forced := make(chan struct{})
	deadline := time.AfterFunc(timeout, func() {
		go r.terminals.ForceClose()
		go r.server.ForceClose()
		close(forced)
	})
	defer deadline.Stop()

	r.server.BeginStopping()

	var joined []error
	if err := handoff.Remove(HandoffDir(dependencies.Home), r.document.Secret); err != nil {
		joined = append(joined, fmt.Errorf("remove the command-line handoff: %w", err))
	}

	r.terminals.BeginShutdown()
	r.server.BeginShutdown()

	barriers := make(chan error, 2)
	go func() { barriers <- r.terminals.Wait() }()
	go func() { barriers <- r.server.Wait() }()
	for range 2 {
		if err := <-barriers; err != nil {
			joined = append(joined, err)
		}
	}

	if r.passwords != nil {
		r.passwords.Lock()
	}
	return errors.Join(joined...)
}

// newOrigin は、このインストールの不透明な識別子を発行する。
//
// 乱数であり、マシンに関する何からも導出されない。ホスト名から作った識別子は、
// バケットを読める者すべてが見られるオブジェクトにそのホスト名を書き込むことに
// なり、ランダムな文字列で得られる以上の利点は何もない。
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
