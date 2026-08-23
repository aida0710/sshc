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

// unsafeAliasWarning は、`sshc <alias>` がこの先で止まる理由を先に言う。
//
// 安全な文字集合の外にある alias は、ssh を起こす直前に platform 層が拒否する。
// その拒否だけを見ると、打った本人には何が悪かったのか分からない。
const unsafeAliasWarning = "This alias contains characters that could change the meaning of a command line, so this connection will be refused."

type Dependencies struct {
	Random io.Reader
	// Announce は、この常駐が受け付けられる状態になったことを伝える。
	//
	// **engine 自身はブラウザを開かない。** 開くのは、走っている engine から
	// 入口を貰いに来た裸の `sshc` である。engine は tmux の中や supervisor の
	// 下で走っていることがあり、そこに画面があるとは限らない。
	//
	// **入口の URL を運ぶかどうかは、受け取る側で決まる。** Android の外殻は
	// これをそのまま WebView へ読み込ませるので要るが、端末へ書く実装
	// （announceReadiness）は URL に触れない——**書けばトークンがログに残る。**
	// nil なら何も言わない。自動化からの明示的な選択である。
	Announce func(Readiness) error
	Listen   ListenFunc
	// StopEngine は Run が自分で埋める。**呼び出し側が渡すものではない**
	// ——engine を止められるのは engine 自身だけである。
	StopEngine func()
	// Port は、受け口の番号である。**0 は「決めていない」であり、そのときは
	// 30000 以上から無作為に引く。** 決めた人が居るのに黙って別の番号へ逃げると、
	// その人がブラウザへ打つ綴りが外れるので、0 でなければその番号だけを試す。
	Port   int
	UI     fs.FS
	Logger *slog.Logger
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
	// **このアプリケーションは OpenSSH のプログラムを自分から実行しない。**
	// 例外は ProxyCommand ひとつで、あれは利用者が「これで繋げ」と書いた綴りを
	// そのまま起こす（internal/sshclient/proxycommand.go）。
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
	// DefaultShutdownTimeout。
	//
	// **外側で待っている者より確実に短くなければならない。** 席を空けるのを
	// 待つのは `sshc engine --replace` であり、あちらが諦めた後に engine が
	// 畳み終えても、席は空いたのに起こし直す側は失敗した後、という順序になる。
	// だから外側の上限（engineReleaseTimeout）はこの値から導いてある。
	// テストは秒を待たずに済むよう、ここへ短い値を注入する。
	ShutdownTimeout time.Duration
}

// DefaultShutdownTimeout は、承認済みの内側の締切である。
//
// **これを外へ出しているのは、待つ側に数えさせるためである。** 以前ここは
// 非公開で、外側の猶予は別の場所に別の数として書かれていた。片方だけ動かせば
// 順序は黙って壊れる。
const DefaultShutdownTimeout = 4 * time.Second

// Readiness は、受け付けを始めた常駐がどんな状態かを述べる。
type Readiness struct {
	Owner handoff.Owner
	// Entrance は、この起動ぶんの入口である。
	//
	// **受け取った側が、出してよい相手を知っている。** 画面を抱えている外殻
	// （Android の WebView）はこれをそのまま読み込む——あちらには入口を後から
	// 求める口が無い。端末で走る engine は**これを出さない**。出せば、ログにも
	// 画面にもワンタイムの資格情報が残る。あちらは `sshc` が求めたときに 1 つ
	// ずつ発行する。
	Entrance string
	// VaultExists は、まだ作られていない vault と、作られていて施錠されている
	// vault を分ける。案内はこれだけで決まる。
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
// 返すのはサービスと、それが書くマネージャの両方である。**呼び出し側は封をしなければ
// ならない。** このマネージャが置き換えるのは秘密鍵そのものなので、封が無ければ、
// パスフレーズの変更が以前の平文の鍵を世代バックアップに残す。マネージャを内側に
// 隠していたことが、その配線漏れを見えなくしていた。
func buildKeyService(workspace *storage.Workspace, dependencies Dependencies, configuration *application.Service) (*keys.Service, *storage.Manager) {
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
	}), transactions
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
	// **サービスを先に組む。** 受け口の番号は保存された設定にも書けるので、
	// それを読むにはワークスペースが要る。旗で決めた番号があるなら設定は見ない
	// ——**旗の方が強い**、がこの順の意味である。
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
			if err := validate.Alias(alias); err != nil {
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
		StopEngine:      dependencies.StopEngine,
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
	// **頼まれて終わる道を用意する。** 走っている engine を、どこで起こしたか
	// 分からないまま止められる必要がある——探して回るより、走っているものに
	// 頼む方が短い。信号ではなく取り消しにするのは、Windows に SIGTERM が無く、
	// TerminateProcess では端末も転送も vault も畳まれないまま消えるからである。
	asked, stopAsked := context.WithCancel(ctx)
	defer stopAsked()
	dependencies.StopEngine = stopAsked

	built, err := build(dependencies, version)
	if err != nil {
		return err
	}

	serveErrors := make(chan error, 1)
	go func() { serveErrors <- built.server.Serve() }()

	// 巡回は、この実行と同じ寿命を持つ。**止めるのは ctx である**——後始末の
	// 段に足すと、そこへ辿り着くまでのあいだ、畳んでいる最中のワークスペースへ
	// 別のマシンのスナップショットを置きにいくことがありうる。
	loop, stopLoop := context.WithCancel(asked)
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
		// 取り消しの出どころ（信号か、頼みごとか）は問わない。畳み方は同じである。
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
		timeout = DefaultShutdownTimeout
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
