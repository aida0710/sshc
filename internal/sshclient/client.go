package sshclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/commandconn"
	"sshc/internal/connectionlog"
	"sshc/internal/platform"
	"sshc/internal/terminal"
	"sshc/internal/textencoding"
)

// DefaultTimeout は、ConnectTimeout が書かれていないときの上限である。
//
// 上限を持たないと、応答しないアドレスへの接続が一覧に居座り続ける。
const DefaultTimeout = 30 * time.Second

// Dialer は、ひとつの接続を開く。
type Dialer struct {
	// ObserveOS prepares an optional presentation-only OS observer for a target.
	ObserveOS func(Target) func(string)
	Auth      Auth
	HostKeys  HostKeys
	// Dial は TCP を開く。nil なら net.Dialer。テストと、将来の別の輸送のためにある。
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
	// DialVPN は、名前の付いた VPN 経路を通して接続先へ繋ぐ。
	//
	// nil なら、VPN を指定した接続は断る。Docker が無い機械の engine は、
	// この輸送を持たない。
	DialVPN func(ctx context.Context, profile, address string) (net.Conn, error)
	// ProxyEnvironmentはProxyCommandを起動するときだけ呼ぶ。nilなら親の環境を使う。
	// 取得に失敗した場合は返された環境を使い、理由を接続ログへ表示する。
	ProxyEnvironment func(context.Context) ([]string, error)
	// Verbosity は、接続ログをどの深さまで端末へ書くかを、接続のたびに返す。
	// その深さ以下の行を書く。nil なら connectionlog.Notice で、`[sshc]` の行だけを書く。
	//
	// 一度だけ読まない。設定は走っているあいだに変えられる。捕まえて
	// しまうと、変えたユーザーは engine を起動し直すまで何も変わらないと感じる。
	Verbosity func() connectionlog.Level
}

// Open は、この alias のセッションをひとつ返す。
//
// 握手を待たずに返る。接続の途中でユーザーに尋ねることがあり、その問いは
// この Process の出力を通って端末へ出るからである。握手を終えてから返すと、
// 誰も繋がっていない出力へ問いを書くことになる。
//
// 接続できなかった理由は、端末へ書かれて終了済みセッションとして残る。
// 理由が読めるのはそこだけである。
func (d Dialer) Open(ctx context.Context, target Target, size terminal.Size) (terminal.Process, error) {
	ctx, cancel := context.WithCancel(ctx)
	var observed func(string)
	if d.ObserveOS != nil && target.RemoteCommand == "" {
		observed = d.ObserveOS(target)
	}
	session := newSession(size, cancel)
	go d.connect(ctx, target, session, observed)
	return session, nil
}

func (d Dialer) connect(ctx context.Context, target Target, session *Session, observed func(string)) {
	prompt := session.Prompter()
	level := connectionlog.Notice
	if d.Verbosity != nil {
		level = d.Verbosity()
	}
	trace := newTracer(level, session.writer)
	trace.progress = session.setProgress
	session.trace = trace
	session.closingTrace = trace.writingTo(&session.closingLog)
	started := trace.now()
	trace.say(connectionlog.Full, "接続ログ：すべて（-vvv）")

	client, closers, err := d.chain(ctx, target, prompt, trace)
	if err != nil {
		session.fail(fmt.Errorf("sshc: %w", err))
		return
	}
	closers = append(closers, client)
	// handshake が通った時点で輸送の所有権を Session へ渡す。NewSession の
	// 応答を待っている隙に Close されても、client と ProxyJump を残さない。
	if !session.attach(nil, closers) {
		return
	}
	if !session.attachClient(client) {
		closeAll(closers)
		return
	}
	hops := len(target.JumpRoute()) + 1
	trace.stage(terminal.ConnectionOpeningSession, target, hops, hops)
	trace.say(connectionlog.Brief, "認証に成功しました。セッションを開始します。")

	channelStarted := trace.now()
	trace.say(connectionlog.Full, "SSHセッションチャンネルを要求します。")
	remote, err := client.NewSession()
	if err != nil {
		trace.say(connectionlog.Detailed, "SSHセッションチャンネルの要求が失敗しました（%s）：%v", connectionlog.Elapsed(trace.since(channelStarted)), err)
		session.fail(fmt.Errorf("sshc: %w", err))
		closeAll(closers)
		return
	}
	trace.say(connectionlog.Full, "SSHセッションチャンネルが開きました（%s）。", connectionlog.Elapsed(trace.since(channelStarted)))

	if !session.attach(remote, closers) {
		return
	}

	// 転送はチャンネルを開いたあと、シェルを起動する前に開く。開いていることを
	// 端末の一行目に書くためであり、失敗しても接続は続ける。
	session.forwarded.open(client, target.Forwards, trace)
	if target.AgentForward {
		session.forwarded.forwardAgent(client, remote, d.Auth.Agent, trace)
	}

	if err := d.start(remote, target, session); err != nil {
		session.fail(fmt.Errorf("sshc: %w", err))
		closeAll(closers)
		return
	}
	if observed != nil {
		go func() {
			if name := detectRemoteOS(ctx, client); name != "" {
				observed(name)
			}
		}()
	}
	session.markReady(nil)
	trace.say(connectionlog.Brief, "セッションを開始しました。")
	if target.KeepAlive > 0 {
		trace.say(connectionlog.Detailed, "keepalive：%sごとに送り、%d回続けて応答が無ければ切断します。",
			target.KeepAlive, keepAliveCount(target.KeepAliveMax))
	} else {
		trace.say(connectionlog.Detailed, "keepalive：送りません（ServerAliveInterval 0）。")
	}
	trace.say(connectionlog.Full, "接続完了まで%sかかりました。", connectionlog.Elapsed(trace.since(started)))
	session.run(remote, client, keepAliveSettings{
		interval: target.KeepAlive, count: target.KeepAliveMax, done: session.done,
		trace: trace, closingTrace: session.closingTrace,
	})
}

// start は、チャンネルの上で端末を要求し、シェルかコマンドを起動する。
func (d Dialer) start(remote *ssh.Session, target Target, session *Session) error {
	streams, _, err := encodeStreams(Streams{
		In: session.input, Out: session.writer, Err: session.writer,
	}, target.Encoding)
	if err != nil {
		return err
	}
	if target.Encoding != "" && target.Encoding != textencoding.UTF8 {
		session.trace.say(connectionlog.Detailed, "文字エンコーディング：%s", target.Encoding)
	}
	remote.Stdin = streams.In
	remote.Stdout = streams.Out
	// stderr を同じ道へ流すのは、端末がひとつだからである。分けて運んでも
	// 出す場所が無い。
	remote.Stderr = streams.Err

	for _, variable := range target.SetEnv {
		// 拒否されても続ける。サーバーが AcceptEnv を絞っているのは普通のことで、
		// それを理由に接続を諦める必要はない。断られたことは接続ログにだけ残す。
		// 値は書かない。SetEnv にトークンを置く人がいる。
		if err := remote.Setenv(variable.Name, variable.Value); err != nil {
			session.trace.say(connectionlog.Detailed, "環境変数%sは受け入れられませんでした（サーバーのAcceptEnvを確認してください）。", variable.Name)
			continue
		}
		session.trace.say(connectionlog.Detailed, "環境変数%sを送りました。", variable.Name)
	}

	if strings.EqualFold(target.RequestTTY, "no") {
		session.trace.say(connectionlog.Detailed, "PTYは要求しません（RequestTTY no）。")
	} else {
		// xterm.js is not a local TTY, so there is no real input or output baud
		// rate to forward. Inventing one can leave the remote PTY with a speed
		// that its termios implementation cannot apply again when programs enter
		// raw mode. Let the server keep its native PTY speeds and only request the
		// interactive echo behaviour the browser terminal expects.
		modes := ssh.TerminalModes{ssh.ECHO: 1}
		// 寸法は送る直前に読む。SetEnv や agent 転送の往復のあいだに届いた
		// Resize を、pty-req に載せるためである。
		size := session.currentSize()
		session.trace.say(connectionlog.Detailed, "PTYを要求します：%d列 × %d行（TERM=%s）。", size.Cols, size.Rows, terminal.DefaultTerminalType)
		started := session.trace.now()
		if err := remote.RequestPty(terminal.DefaultTerminalType, int(size.Rows), int(size.Cols), modes); err != nil {
			session.trace.say(connectionlog.Detailed, "PTY要求が失敗しました（%s）：%v", connectionlog.Elapsed(session.trace.since(started)), err)
			return err
		}
		session.trace.say(connectionlog.Full, "PTY要求が受け入れられました（%s）。", connectionlog.Elapsed(session.trace.since(started)))
		if err := session.markPtyReady(size); err != nil {
			return err
		}
	}
	started := session.trace.now()
	if target.RemoteCommand != "" {
		session.trace.say(connectionlog.Detailed, "リモートコマンドを実行します：%s", target.RemoteCommand)
		err = remote.Start(target.RemoteCommand)
	} else {
		session.trace.say(connectionlog.Detailed, "シェルを起動します。")
		err = remote.Shell()
	}
	if err != nil {
		session.trace.say(connectionlog.Detailed, "起動要求が失敗しました（%s）：%v", connectionlog.Elapsed(session.trace.since(started)), err)
		return err
	}
	session.trace.say(connectionlog.Full, "起動要求が受け入れられました（%s）。", connectionlog.Elapsed(session.trace.since(started)))
	return nil
}

// chain は、ProxyJump を手前から順に繋ぎ、最後の接続を返す。
//
// プログラムは一つも起動しない。各ホップの SSH チャンネルの上に、次の
// ホップへの TCP を載せるだけである。ProxyCommand との違いはそこにある。
func (d Dialer) chain(ctx context.Context, target Target, prompt Prompter, trace *tracer) (*ssh.Client, []io.Closer, error) {
	var closers []io.Closer
	var through *ssh.Client
	route := target.JumpRoute()

	// 読みはするが従わない設定は、`sshc info` の他にここでしか言えない。
	// 適用しない設定は、CLIとWebの接続ログでも理由を確認できるようにする。
	for _, notice := range target.Notices {
		trace.say(connectionlog.Detailed, "設定%sは適用しません：%s", notice.Keyword, notice.Detail)
	}

	if len(route) > 0 && trace.enabled(connectionlog.Detailed) {
		hops := make([]string, 0, len(route))
		for _, hop := range route {
			hops = append(hops, hop.Address())
		}
		trace.say(connectionlog.Detailed, "ProxyJump：%dホップ（%s）", len(hops), strings.Join(hops, " → "))
	}

	hops := len(route) + 1
	for index, hop := range route {
		client, err := d.connectOne(ctx, hopConnection{
			hopDial: hopDial{target: hop, through: through, trace: trace}, prompt: prompt, number: index + 1, count: hops,
		})
		if err != nil {
			closeAll(closers)
			return nil, nil, err
		}
		closers = append(closers, client)
		through = client
	}

	client, err := d.connectOne(ctx, hopConnection{
		hopDial: hopDial{target: target, through: through, trace: trace}, prompt: prompt, number: hops, count: hops,
	})
	if err != nil {
		closeAll(closers)
		return nil, nil, err
	}
	return client, closers, nil
}

// hopConnection は、経路のホップひとつへ繋ぐのに要るものである。
type hopConnection struct {
	hopDial
	prompt Prompter
	// number は経路の中でこのホップが何番目か（1 始まり）、count は経路のホップ数。
	number, count int
}

// connectOne は、ホップひとつへ繋ぐ。through が非 nil なら、その接続の上を通る。
func (d Dialer) connectOne(ctx context.Context, request hopConnection) (*ssh.Client, error) {
	target, through, prompt, trace := request.target, request.through, request.prompt, request.trace
	hop, hops := request.number, request.count
	started := trace.now()
	timeout := target.connectTimeout()

	if through != nil {
		trace.say(connectionlog.Brief, "%sへProxyJump経由で接続します（ユーザー：%s）。", target.Address(), target.User)
	} else {
		trace.say(connectionlog.Brief, "%sへ接続します（ユーザー：%s）。", target.Address(), target.User)
	}
	trace.say(connectionlog.Detailed, "接続タイムアウト：%s", timeout)
	describeHop(trace, target)

	trace.stage(terminal.ConnectionDialing, target, hop, hops)
	conn, deadline, err := d.openWithTimeout(trace.withLog(ctx), request.hopDial, timeout)
	if err != nil {
		trace.say(connectionlog.Brief, "%s", connectionFailureMessage("接続", err))
		explainFailure(trace, err)
		return nil, err
	}
	defer deadline.stop()
	prompt = deadline.pausing(prompt)
	if target.ProxyCommand != "" {
		trace.say(connectionlog.Detailed, "ProxyCommandのプロセスを起動しました（%s）。SSHの応答を待ちます。", connectionlog.Elapsed(trace.since(started)))
	} else {
		trace.say(connectionlog.Detailed, "TCP接続を確立しました（%s）。", connectionlog.Elapsed(trace.since(started)))
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		// 素の TCP のときだけ言う。ProxyJump の上のチャンネルや ProxyCommand の
		// パイプが名乗るアドレスは、どこを通ったかを表さない。
		trace.say(connectionlog.Full, "ローカル：%s → リモート：%s", tcp.LocalAddr(), tcp.RemoteAddr())
	}

	auth, lastTriedMethod := d.authWithTrace(trace)
	auth.Observe = deadline.stopAtAuthentication(auth.Observe)
	authMethods, closeAuth := auth.methodsWithCleanup(target, prompt)
	defer closeAuth()
	hostKeys := d.HostKeys.lookup(target)
	verifyHostKey := hostKeys.callback(prompt, trace)
	config := &ssh.ClientConfig{
		User:         target.User,
		Auth:         authMethods,
		AuthCallback: traceAuthentication(trace),
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			trace.stage(terminal.ConnectionHostKey, target, hop, hops)
			trace.say(connectionlog.Full, "鍵交換が終わり、ホスト鍵を受け取りました（%s）。", connectionlog.Elapsed(trace.since(started)))
			err := verifyHostKey(hostname, remote, key)
			if err == nil {
				trace.stage(terminal.ConnectionAuthenticating, target, hop, hops)
			}
			return err
		},
		// すでに持っている鍵の種類を先に名乗る。既定の順序に任せると、
		// 三種類の鍵を持つホストが known_hosts にある 1 行とは違う種類を出し、
		// 正しい鍵が「一致しない鍵」として現れる。
		HostKeyAlgorithms: hostKeys.algorithms(),
		BannerCallback:    func(message string) error { trace.banner(message); return nil },
		Timeout:           timeout,
	}
	trace.say(connectionlog.Full, "提示するホスト鍵アルゴリズム：%s", strings.Join(config.HostKeyAlgorithms, ", "))
	describeAlgorithmOffers(trace, config)
	handshakeStarted := trace.now()
	trace.say(connectionlog.Full, "SSHハンドシェイクを開始します。")
	connection, channels, requests, err := newClientConn(deadline.ctx, conn, target.Address(), config)
	if err != nil {
		trace.say(connectionlog.Detailed, "SSHハンドシェイクが停止しました（%s）。", connectionlog.Elapsed(trace.since(handshakeStarted)))
		describeProxyExit(trace, conn)
		trace.say(connectionlog.Brief, "%s", connectionFailureMessage("SSHハンドシェイク", err))
		explainFailure(trace, err)
		return nil, err
	}
	if method := lastTriedMethod(); method != "" {
		trace.say(connectionlog.Detailed, "認証方式%sで認証されました。", method)
	} else {
		trace.say(connectionlog.Detailed, "サーバーは認証を求めませんでした。")
		if metadata, ok := connection.(ssh.AlgorithmsConnMetadata); ok {
			describeNegotiatedAlgorithms(trace, metadata.Algorithms())
		}
	}
	trace.say(connectionlog.Detailed, "SSHハンドシェイクが完了しました（%s）。", connectionlog.Elapsed(trace.since(started)))
	trace.say(connectionlog.Full, "サーバーのSSHバージョン：%s", connection.ServerVersion())
	trace.say(connectionlog.Brief, "%sに接続しました（%d/%d）。", connectionTarget(target), hop, hops)
	trace.stage(terminal.ConnectionAuthenticated, target, hop, hops)
	return ssh.NewClient(connection, channels, requests), nil
}

// authWithTrace は共有DialerのAuthを接続単位で複製し、安全な診断だけを
// tracerへ流す。共有値を書き換えないため、同時接続でもobserverが混ざらない。
//
// 返す関数は、最後に試した方式の名前である。x/crypto/ssh は通った方式を
// 教えないが、方式は順に試され、通った瞬間に握手が終わるので、最後に
// 試したものが通ったものである。
func (d Dialer) authWithTrace(trace *tracer) (Auth, func() string) {
	auth := d.Auth
	auth.trace = trace
	var lastTried string
	observeMethod := auth.Observe
	auth.Observe = func(method string) {
		if observeMethod != nil {
			observeMethod(method)
		}
		lastTried = method
		trace.say(connectionlog.Detailed, "認証方式を試します：%s", method)
	}
	observeCredential := auth.ObserveCredential
	auth.ObserveCredential = func(target Target, event CredentialEvent, echoed bool) {
		if observeCredential != nil {
			observeCredential(target, event, echoed)
		}
		switch event {
		case CredentialTOTPUsed:
			trace.say(connectionlog.Detailed, "保存済みTOTPを%sの認証コードのプロンプトへ入力しました。", connectionTarget(target))
		case CredentialTOTPUnavailable:
			trace.say(connectionlog.Detailed, "明示的な認証コードのプロンプトを検出しましたが、%sに利用できる保存済みTOTPがありません。Connectionsで割り当てを確認してください。", connectionTarget(target))
		}
	}
	return auth, func() string { return lastTried }
}

func connectionTarget(target Target) string {
	if target.Alias != "" {
		return target.Alias
	}
	return target.Address()
}

// open は、この接続先までの輸送をひとつ用意する。
//
// 輸送を選ぶのはここだけである。手前のホップの上か、ProxyCommand の
// 標準入出力か、素の TCP か。
func (d Dialer) open(ctx context.Context, target Target, through *ssh.Client, trace *tracer) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if target.VPN != "" {
		// VPN もこの機械から出る経路である。ProxyCommand と同じく、踏み台の
		// 向こうのホップには効かない。
		if target.ProxyCommand != "" {
			return nil, ErrVPNWithProxyCommand
		}
		if through != nil {
			return nil, ErrVPNThroughJump
		}
		if d.DialVPN == nil {
			return nil, ErrVPNUnavailable
		}
		trace.announce("VPNプロファイル「%s」の経路で接続します。", target.VPN)
		return d.DialVPN(ctx, target.VPN, target.Address())
	}
	if target.ProxyCommand != "" {
		// そのプログラムはこの機械で走る。手前のホップの中ではない。
		// 踏み台の向こうのホップに書かれていたら、走らせても設定が言っている
		// 場所には届かない。
		if through != nil {
			return nil, ErrProxyCommandThroughJump
		}
		trace.announce("ProxyCommandを実行します：%s", target.ProxyCommand)
		var environment []string
		if d.ProxyEnvironment != nil {
			var err error
			environment, err = d.ProxyEnvironment(ctx)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if err != nil {
				trace.announce("ログインシェルのPATHを取得できないため、起動元のPATHを使います：%v", err)
			} else {
				trace.say(connectionlog.Detailed, "ログインシェルからPATHを取得しました。")
				path, _ := platform.LookupEnvironment(environment, "PATH")
				trace.say(connectionlog.Full, "PATH：%s", path)
			}
		}
		return startProxyCommand(target.ProxyCommand, environment, trace)
	}
	if through != nil {
		trace.say(connectionlog.Detailed, "%sへ、手前のホップの中からTCPで接続します。", target.Address())
		return through.DialContext(ctx, "tcp", target.Address())
	}
	if d.Dial != nil {
		return d.Dial(ctx, "tcp", target.Address())
	}
	describeResolution(ctx, trace, target.HostName)
	return (&net.Dialer{}).DialContext(ctx, "tcp", target.Address())
}

// describeResolution は、HostName の名前解決の結果を debug3 で言う。
//
// 接続そのものは net.Dialer がもう一度名前解決する。ここで引くのは、どのアドレス
// へ繋ぎに行くのかを見せるためだけである。
func describeResolution(ctx context.Context, trace *tracer, host string) {
	if !trace.enabled(connectionlog.Full) || net.ParseIP(host) != nil {
		return
	}
	started := trace.now()
	addresses, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		trace.say(connectionlog.Full, "%sの名前解決に失敗しました：%v", host, err)
		return
	}
	trace.say(connectionlog.Full, "%sを名前解決しました：%s（%s）", host, strings.Join(addresses, ", "),
		connectionlog.Elapsed(trace.since(started)))
}

// hopDial は、ホップひとつへ輸送を開くのに要るものである。
type hopDial struct {
	target  Target
	through *ssh.Client
	trace   *tracer
}

// openWithTimeout は、ホップへの輸送を開き、そのあとの SSH のやり取りの期限を返す。
//
// 接続のタイムアウトは、ふつうは TCP の接続から数える。VPN の経路は、経路の起動
// （イメージの作成、スマートフォンでの承認）に接続のタイムアウトより長くかかる
// ことがあり、経路の側が自分の上限で待つ。そこで VPN のときだけ、経路が用意
// できてから数え始める。
//
// 接続には期限付きの ctx を渡す。止められる期限（connectDeadline）の取り消しでは、
// net.Dialer がタイムアウトではなく取り消しとして失敗を返すからである。
func (d Dialer) openWithTimeout(
	ctx context.Context, hop hopDial, timeout time.Duration,
) (net.Conn, *connectDeadline, error) {
	if hop.target.VPN != "" {
		conn, err := d.open(ctx, hop.target, hop.through, hop.trace)
		if err != nil {
			return nil, nil, err
		}
		return conn, startConnectDeadline(ctx, timeout, timeout), nil
	}
	started := time.Now()
	dialing, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := d.open(dialing, hop.target, hop.through, hop.trace)
	if err != nil {
		return nil, nil, err
	}
	return conn, startConnectDeadline(ctx, timeout-time.Since(started), timeout), nil
}

// ExplainedError は、利用者向けの文を持つ失敗である。接続ログには、元の失敗の
// 文ではなくこの文を出す。
//
// DialVPN のように、この package の外で輸送を開く部品が使う。元の失敗の文は
// 英語の実装の言葉で、Terminal の画面にそのまま出すと何を直せばよいか分からない。
type ExplainedError struct {
	Sentence string
	// Details は、原因を調べるための行（コマンドの出力など）である。接続ログの
	// debug2 から出す。
	Details []string
	Err     error
}

func (failure *ExplainedError) Error() string { return failure.Sentence }

func (failure *ExplainedError) Unwrap() error { return failure.Err }

// explainFailure は、接続の失敗の詳細を debug2 から出す。元の失敗の文は、利用者
// 向けの文に置き換えたものでも、原因を調べるために debug2 に残す。
func explainFailure(trace *tracer, err error) {
	var explained *ExplainedError
	if !errors.As(err, &explained) {
		return
	}
	for _, line := range explained.Details {
		trace.say(connectionlog.Detailed, "  %s", line)
	}
	if explained.Err != nil {
		trace.say(connectionlog.Detailed, "失敗の詳細：%v", explained.Err)
	}
}

// describeHop は、このホップで使う設定を debug2 から言う。ssh -G で見るものの
// うち、接続の成否に関わるものである。
func describeHop(trace *tracer, target Target) {
	if !trace.enabled(connectionlog.Detailed) {
		return
	}
	identities := "なし"
	if len(target.Identities) > 0 {
		identities = strings.Join(target.Identities, ", ")
	}
	trace.say(connectionlog.Detailed, "設定：HostName %s、Port %s、User %s、IdentityFile %s、IdentitiesOnly %t",
		target.HostName, target.Port, target.User, identities, target.IdentitiesOnly)
	switch {
	case target.VPN != "":
		trace.say(connectionlog.Detailed, "経路：VPNプロファイル「%s」", target.VPN)
	case target.ProxyCommand != "":
		trace.say(connectionlog.Detailed, "経路：ProxyCommand")
	default:
		trace.say(connectionlog.Detailed, "経路：直接TCPで接続")
	}
}

// connectionFailureMessage は、どの段階で失敗したかを言う。理由は言わない。
// 理由は、接続の最後に `sshc:` の行（Session.fail）で必ず書くので、ここでも書くと
// 同じ文が2回並ぶ。
func connectionFailureMessage(action string, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return action + "がタイムアウトしました。"
	}
	if errors.Is(err, context.Canceled) {
		return action + "をキャンセルしました。"
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return action + "がタイムアウトしました。"
	}
	return action + "に失敗しました。"
}

// newClientConn は SSH handshake を ctx の所有下で行う。
//
// net.Conn の deadline だけでは、親 context の途中 cancel は期限まで反映されない。
// handshake がまだ raw transport を所有している間は、cancel 時にそれを閉じて
// NewClientConn を直ちに解く。
//
// 取り消されたときは、取り消しの理由（context.Cause）を返す。connectDeadline は
// 期限を持たない ctx を context.DeadlineExceeded を理由に取り消すので、ctx.Err()
// ではタイムアウトが取り消しに見えてしまう。
func newClientConn(
	ctx context.Context, conn net.Conn, address string, config *ssh.ClientConfig,
) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	cancelled := make(chan struct{})
	stopClose := context.AfterFunc(ctx, func() {
		_ = conn.Close()
		close(cancelled)
	})

	connection, channels, requests, err := ssh.NewClientConn(conn, address, config)
	if !stopClose() {
		<-cancelled
	}
	if err != nil {
		err = withComplaints(err, conn)
		_ = conn.Close()
		if ctx.Err() != nil {
			return nil, nil, nil, context.Cause(ctx)
		}
		return nil, nil, nil, classifyHandshakeFailure(err)
	}
	if ctx.Err() != nil {
		_ = connection.Close()
		return nil, nil, nil, context.Cause(ctx)
	}
	_ = conn.SetDeadline(time.Time{})
	return connection, channels, requests, nil
}

// withComplaints は、プログラムが標準エラーへ書いたものを理由に足す。
//
// 繋がらなかった理由は、たいていそこにしかない。`ssh -W` は
// "Connection refused" を stderr へ書き、こちらから見えるのは「握手が
// 通らなかった」だけになる。
func withComplaints(err error, conn net.Conn) error {
	command, ok := conn.(*commandconn.Conn)
	if !ok {
		return err
	}
	said := command.Complaints()
	if said == "" {
		return err
	}
	return proxyFailure(err, said)
}

// closeAll は、開いた順に並んだ closers を奥（最後に開いたもの）から閉じる。
// 手前のホップを先に閉じると、その上に載っている奥の接続は閉じ方が「切断」になる。
func closeAll(closers []io.Closer) {
	for index := len(closers) - 1; index >= 0; index-- {
		_ = closers[index].Close()
	}
}
