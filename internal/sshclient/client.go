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
	// Verbosity は、接続の途中経過をどこまで端末へ書くかを、接続のたびに
	// 返す。nil なら無言である。
	//
	// 一度だけ読まない。設定は走っているあいだに変えられる。捕まえて
	// しまうと、変えたユーザーは engine を起動し直すまで何も変わらないと感じる。
	Verbosity func() Verbosity
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
	level := Quiet
	if d.Verbosity != nil {
		level = d.Verbosity()
	}
	trace := newTracer(level, session.writer)
	trace.progress = session.setProgress
	session.trace = trace
	started := trace.now()
	trace.say(Full, "接続ログ：すべて（-vvv）")

	client, closers, err := d.chain(ctx, target, prompt, trace)
	if err != nil {
		session.fail(fmt.Errorf("sshc: %w", err))
		return
	}
	closers = append(closers, client)
	// handshake が通った時点で輸送の所有権を Session へ渡す。NewSession の
	// 応答を待っている隙に Close されても、client と ProxyJump を残さない。
	if _, attached := session.attach(nil, closers); !attached {
		return
	}
	if !session.attachClient(client) {
		closeAll(closers)
		return
	}
	hops := len(target.JumpRoute()) + 1
	trace.stage(terminal.ConnectionOpeningSession, target, hops, hops)
	trace.say(Brief, "認証に成功しました。セッションを開始します。")

	remote, err := client.NewSession()
	if err != nil {
		session.fail(fmt.Errorf("sshc: %w", err))
		closeAll(closers)
		return
	}

	size, attached := session.attach(remote, closers)
	if !attached {
		return
	}

	// 転送はチャンネルを開いたあと、シェルを起動する前に開く。開いていることを
	// 端末の一行目に書くためであり、失敗しても接続は続ける。
	session.forwarded.open(client, target.Forwards, session.writer)
	if target.AgentForward {
		session.forwarded.forwardAgent(client, remote, d.Auth.AgentSocket, session.writer)
	}

	if err := d.start(remote, target, size, session); err != nil {
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
	trace.say(Brief, "セッションを開始しました。")
	if target.KeepAlive > 0 {
		trace.say(Detailed, "keepalive：%s ごとに送り、%d 回続けて応答が無ければ切断します。",
			target.KeepAlive, keepAliveCount(target.KeepAliveMax))
	} else {
		trace.say(Detailed, "keepalive：送りません（ServerAliveInterval 0）。")
	}
	trace.say(Full, "接続完了まで %s かかりました。", trace.since(started).Round(time.Millisecond))
	session.run(remote, keepAliveLoop(client, target.KeepAlive, target.KeepAliveMax, session.done))
}

// start は、チャンネルの上で端末を要求し、シェルかコマンドを起動する。
func (d Dialer) start(remote *ssh.Session, target Target, size terminal.Size, session *Session) error {
	streams, _, err := encodeStreams(Streams{
		In: session.input, Out: session.writer, Err: session.writer,
	}, target.Encoding)
	if err != nil {
		return err
	}
	if target.Encoding != "" && target.Encoding != textencoding.UTF8 {
		session.trace.say(Detailed, "文字エンコーディング：%s", target.Encoding)
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
			session.trace.say(Detailed, "環境変数 %s は受け入れられませんでした（サーバーの AcceptEnv を確認してください）。", variable.Name)
			continue
		}
		session.trace.say(Detailed, "環境変数 %s を送りました。", variable.Name)
	}

	if strings.EqualFold(target.RequestTTY, "no") {
		session.trace.say(Detailed, "端末は要求しません（RequestTTY no）。")
	} else {
		// xterm.js is not a local TTY, so there is no real input or output baud
		// rate to forward. Inventing one can leave the remote PTY with a speed
		// that its termios implementation cannot apply again when programs enter
		// raw mode. Let the server keep its native PTY speeds and only request the
		// interactive echo behaviour the browser terminal expects.
		modes := ssh.TerminalModes{ssh.ECHO: 1}
		session.trace.say(Detailed, "端末を要求します：%d 列 × %d 行（TERM=%s）。", size.Cols, size.Rows, TermName)
		if err := remote.RequestPty(TermName, int(size.Rows), int(size.Cols), modes); err != nil {
			return err
		}
	}
	if target.RemoteCommand != "" {
		session.trace.say(Detailed, "リモートコマンドを実行します：%s", target.RemoteCommand)
		return remote.Start(target.RemoteCommand)
	}
	session.trace.say(Detailed, "シェルを起動します。")
	return remote.Shell()
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
	// 効かない RemoteForward を書いて繋いだ人が、なぜ効かないかを知る場所である。
	for _, notice := range target.Notices {
		trace.say(Detailed, "設定 %s は適用しません：%s", notice.Keyword, notice.Detail)
	}

	if len(route) > 0 && trace.enabled(Detailed) {
		hops := make([]string, 0, len(route))
		for _, hop := range route {
			hops = append(hops, hop.Address())
		}
		trace.say(Detailed, "ProxyJump：%d ホップ（%s）", len(hops), strings.Join(hops, " → "))
	}

	hops := len(route) + 1
	for index, hop := range route {
		client, err := d.connectOne(ctx, hop, through, prompt, trace, index+1, hops)
		if err != nil {
			closeAll(closers)
			return nil, nil, err
		}
		closers = append(closers, client)
		through = client
	}

	client, err := d.connectOne(ctx, target, through, prompt, trace, hops, hops)
	if err != nil {
		closeAll(closers)
		return nil, nil, err
	}
	return client, closers, nil
}

// connectOne は、ホップひとつへ繋ぐ。through が非 nil なら、その接続の上を通る。
func (d Dialer) connectOne(
	ctx context.Context, target Target, through *ssh.Client, prompt Prompter, trace *tracer,
	hop, hops int,
) (*ssh.Client, error) {
	started := trace.now()
	timeout := target.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if through != nil {
		trace.say(Brief, "%s へ ProxyJump 経由で接続します（ユーザー：%s）。", target.Address(), target.User)
	} else {
		trace.say(Brief, "%s へ接続します（ユーザー：%s）。", target.Address(), target.User)
	}
	trace.say(Detailed, "接続タイムアウト：%s", timeout)

	trace.stage(terminal.ConnectionDialing, target, hop, hops)
	conn, err := d.open(ctx, target, through, trace)
	if err != nil {
		trace.say(Brief, "%s", connectionFailureMessage("接続", err))
		return nil, err
	}
	trace.say(Detailed, "TCP 接続を確立しました（%s）。", trace.since(started).Round(time.Millisecond))
	if tcp, ok := conn.(*net.TCPConn); ok {
		// 素の TCP のときだけ言う。ProxyJump の上のチャンネルや ProxyCommand の
		// パイプが名乗るアドレスは、どこを通ったかを表さない。
		trace.say(Full, "ローカル %s → リモート %s", tcp.LocalAddr(), tcp.RemoteAddr())
	}

	auth, lastTriedMethod := d.authWithTrace(trace)
	authMethods, closeAuth := auth.methodsWithCleanup(target, prompt)
	defer closeAuth()
	verifyHostKey := d.HostKeys.callback(target, prompt, trace)
	config := &ssh.ClientConfig{
		User: target.User,
		Auth: authMethods,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			trace.stage(terminal.ConnectionHostKey, target, hop, hops)
			trace.say(Full, "鍵交換が終わり、ホスト鍵を受け取りました（%s）。", trace.since(started).Round(time.Millisecond))
			err := verifyHostKey(hostname, remote, key)
			if err == nil {
				trace.stage(terminal.ConnectionAuthenticating, target, hop, hops)
			}
			return err
		},
		// すでに持っている鍵の種類を先に名乗る。既定の順序に任せると、
		// 三種類の鍵を持つホストが known_hosts にある 1 行とは違う種類を出し、
		// 正しい鍵が「一致しない鍵」として現れる。
		HostKeyAlgorithms: d.HostKeys.Algorithms(target),
		BannerCallback:    func(message string) error { trace.banner(message); return nil },
		Timeout:           timeout,
	}
	trace.say(Full, "名乗るホスト鍵アルゴリズム：%s", strings.Join(config.HostKeyAlgorithms, ", "))
	connection, channels, requests, err := newClientConn(ctx, conn, target.Address(), config)
	if err != nil {
		trace.say(Brief, "%s", connectionFailureMessage("SSH ハンドシェイク", err))
		return nil, err
	}
	if method := lastTriedMethod(); method != "" {
		trace.say(Detailed, "認証方式 %s で認証されました。", method)
	} else {
		trace.say(Detailed, "サーバーは認証を求めませんでした。")
	}
	trace.say(Detailed, "SSH ハンドシェイクが完了しました（%s）。", trace.since(started).Round(time.Millisecond))
	trace.say(Full, "サーバーの SSH バージョン：%s", connection.ServerVersion())
	trace.say(Brief, "%s に接続しました（%d/%d）。", connectionTarget(target), hop, hops)
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
		trace.say(Detailed, "認証方式を試します：%s", method)
	}
	observeCredential := auth.ObserveCredential
	auth.ObserveCredential = func(target Target, event CredentialEvent, echoed bool) {
		if observeCredential != nil {
			observeCredential(target, event, echoed)
		}
		switch event {
		case CredentialTOTPUsed:
			trace.say(Detailed, "保存済みTOTPを%sの認証コード質問へ入力しました。", connectionTarget(target))
		case CredentialTOTPUnavailable:
			trace.say(Detailed, "明示的な認証コード質問を検出しましたが、%sに利用できる保存済みTOTPがありません。Connectionsで割り当てを確認してください。", connectionTarget(target))
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
		trace.announce("VPN %s を通して接続します。", target.VPN)
		return d.DialVPN(ctx, target.VPN, target.Address())
	}
	if target.ProxyCommand != "" {
		// そのプログラムはこの機械で走る。手前のホップの中ではない。
		// 踏み台の向こうのホップに書かれていたら、走らせても設定が言っている
		// 場所には届かない。
		if through != nil {
			return nil, ErrProxyCommandThroughJump
		}
		trace.announce("ProxyCommand を実行します：%s", target.ProxyCommand)
		var environment []string
		if d.ProxyEnvironment != nil {
			var err error
			environment, err = d.ProxyEnvironment(ctx)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if err != nil {
				trace.announce("ログインシェルのPATHを取得できないため、起動元のPATHを使います：%v", err)
			}
		}
		return startProxyCommand(target.ProxyCommand, environment)
	}
	if through != nil {
		return through.DialContext(ctx, "tcp", target.Address())
	}
	if d.Dial != nil {
		return d.Dial(ctx, "tcp", target.Address())
	}
	return (&net.Dialer{}).DialContext(ctx, "tcp", target.Address())
}

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
	return fmt.Sprintf("%sに失敗しました：%v", action, err)
}

// newClientConn は SSH handshake を ctx の所有下で行う。
//
// net.Conn の deadline だけでは、親 context の途中 cancel は期限まで反映されない。
// handshake がまだ raw transport を所有している間は、cancel 時にそれを閉じて
// NewClientConn を直ちに解く。
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
		if cause := ctx.Err(); cause != nil {
			return nil, nil, nil, cause
		}
		return nil, nil, nil, err
	}
	if cause := ctx.Err(); cause != nil {
		_ = connection.Close()
		return nil, nil, nil, cause
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
	command, ok := conn.(*commandConn)
	if !ok {
		return err
	}
	said := command.Complaints()
	if said == "" {
		return err
	}
	return fmt.Errorf("%w: ProxyCommand said: %s", err, said)
}

func closeAll(closers []io.Closer) {
	for index := len(closers) - 1; index >= 0; index-- {
		_ = closers[index].Close()
	}
}
