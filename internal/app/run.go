package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	// Announce は、この常駐への入口を人へ伝える。
	//
	// **ブラウザは開かない。** 画面はデスクトップの外殻が出すので、この
	// プロセスが既定のブラウザを起こす理由はもう無い。書き出す先は、
	// `sshc` を打った人の端末である。nil なら何も言わない——ログへ
	// トークンを落とさないための、自動化からの明示的な選択である。
	Announce func(entrance string) error
	Listen   ListenFunc
	UI       fs.FS
	Logger   *slog.Logger
	// Home はユーザーのホームディレクトリ。オペレーティングシステムから読んでよいのは
	// cmd/sshc だけで、テストはいずれも一時ディレクトリを注入する。
	Home string
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
	// ScanHostKeys と Probe は、ネットワークへ出る 2 つの継ぎ目である。nil なら
	// internal/sshclient がこのプロセスの中で話す。Runner と同じ性質のものであり、
	// **検査がネットワークへ出ないようにするためにここにある。**
	ScanHostKeys func(ctx context.Context, address string, timeout time.Duration) ([]ssh.PublicKey, error)
	Probe        func(ctx context.Context, alias string) (sshclient.Probe, error)
	RemoteRun    func(ctx context.Context, target sshclient.Target, command string, stdin []byte) (sshclient.Output, error)
	// LoginItem は「ログイン時に起動」を切り替える。既定はオフで、ここでそれを変える
	// ことはない。保存済みのあらゆる秘密の鍵を握るバックグラウンドプロセスは、他人に
	// 代わって勝手に用意してよいものではないからだ。nil の場合、この設定は未対応だと
	// 報告する。
	LoginItem httpserver.LoginItemController
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
	listener, err := dependencies.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("listen: %w", err)
	}

	sessions, bootstrap, err := session.NewManager(dependencies.Random)
	if err != nil {
		listener.Close()
		return nil, "", fmt.Errorf("session: %w", err)
	}
	if dependencies.SessionNow != nil {
		sessions.Now = dependencies.SessionNow
	}

	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, dependencies.Home)
	if err != nil {
		listener.Close()
		return nil, "", fmt.Errorf("workspace: %w", err)
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

	// スナップショットは、どのファイルが設定なのかを知る必要がある。それは Include
	// グラフが答える問いである。答えを渡す形にすれば、依存の向きは正しいまま保たれる
	// ── internal/remotesync は、設定サービスのものを何ひとつ import して
	// いない。
	syncService := remotesync.NewService(workspace, transactions,
		func() ([]string, error) { return configService.WorkspaceFiles() },
		func() string { return time.Now().UTC().Format(time.RFC3339) },
		newOrigin(dependencies.Random),
	)

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
		return nil, "", err
	}

	server, err := httpserver.New(httpserver.Options{
		Listener:  listener,
		CLISecret: cliSecret,
		LoginItem: dependencies.LoginItem,
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
		Sessions:    sessions,
		UI:          dependencies.UI,
		Version:     version,
		Logger:      dependencies.Logger,
		Config:      configService,
		Keys:        keyService,
		Diagnostics: diagnosticsService,
		KnownHosts:  knownHostsService,
		RemoteKeys:  remoteKeyService,
		Passwords:   passwordService,
		Connect:     ssh.connector(),
		Sync:        syncService,
		Terminals:   terminals,
		// SSH のプログラムはもう要らない。**接続はこのプロセスの中で話す。**
		// PTY を確保するのはローカルシェルだけである。
		TerminalStartDirectory: configService.TerminalStartDirectory,
		LoginShell:             func() (string, error) { return platform.LoginShell(dependencies.Lookup) },
		TerminalEnvironment: func() []string {
			if dependencies.Environ == nil {
				return nil
			}
			return dependencies.Environ()
		},
	})
	if err != nil {
		listener.Close()
		return nil, "", err
	}

	// プロセスがサーブを始める場所ではなくここで書くのは、ここで URL が判明するから
	// であり、また、構築済みのサーバーは何かが接続できるサーバーだからである。
	// 強制終了されたプロセスが残していったコピーは、何も待ち受けていないポートを、
	// 誰も受け付けない秘密とともに指しているだけなので、これを取り除くのは保証では
	// なく後片付けである。
	if _, err := handoff.Write(HandoffDir(dependencies.Home), server.URL(), cliSecret); err != nil {
		dependencies.Logger.Warn(
			"write the command-line handoff; sshc <alias> will connect without a saved key passphrase",
			"error", err)
	}
	return server, bootstrap, nil
}

// HandoffDir は、動作中のアプリケーションが `sshc <alias>` の読むファイルを置く
// 場所。このアプリケーション自身のものが置かれる、他と同じ状態ディレクトリで
// ある。
func HandoffDir(home string) string {
	return filepath.Join(home, ".ssh", "sshc")
}

func Run(ctx context.Context, dependencies Dependencies, version string) error {
	server, bootstrap, err := Build(dependencies, version)
	if err != nil {
		return err
	}
	// 埋め込みターミナルのセッションは、このプロセスが終わるときに終わる。
	// 待たないのは、応答しないリモートに向いた ssh のために終了が引き延ばされて
	// はならないからである。
	defer server.CloseTerminals()

	target := server.URL() + "/#bootstrap=" + bootstrap
	serverCtx, stopServer := context.WithCancel(ctx)
	defer stopServer()

	// ハンドオフは URL が判明した時点で書かれ、終了時に取り除かれる。強制終了された
	// プロセスが残していったコピーは、何も待ち受けていないポートを、誰も受け付け
	// ない秘密とともに指しているだけなので、この削除は何かの拠り所となる保証では
	// なく後片付けである。
	defer func() {
		if err := handoff.Remove(HandoffDir(dependencies.Home)); err != nil {
			dependencies.Logger.Warn("remove the command-line handoff", "error", err)
		}
	}()

	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(serverCtx) }()

	if dependencies.Announce != nil {
		if err := dependencies.Announce(target); err != nil {
			stopServer()
			<-serveErrors
			return fmt.Errorf("announce the entrance: %w", err)
		}
	}

	select {
	case err := <-serveErrors:
		return err
	case <-server.Stopped():
		// デスクトップの外殻が終了を頼んだ。**ウィンドウを閉じることとは別の意思で
		// ある**ので、頼まれたときだけここへ来る。
		stopServer()
		return <-serveErrors
	case <-ctx.Done():
		stopServer()
		return <-serveErrors
	}
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
