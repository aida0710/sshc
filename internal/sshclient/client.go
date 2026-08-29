package sshclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/terminal"
)

// DefaultTimeout は、ConnectTimeout が書かれていないときの上限である。
//
// 上限を持たないと、応答しないアドレスへの接続が一覧に居座り続ける。
const DefaultTimeout = 30 * time.Second

// Dialer は、ひとつの接続を開く。
type Dialer struct {
	Auth     Auth
	HostKeys HostKeys
	// Dial は TCP を開く。nil なら net.Dialer。テストと、将来の別の輸送のためにある。
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
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
	session := newSession(size, cancel)
	go d.connect(ctx, target, session)
	return session, nil
}

func (d Dialer) connect(ctx context.Context, target Target, session *Session) {
	prompt := session.Prompter()
	level := Quiet
	if d.Verbosity != nil {
		level = d.Verbosity()
	}
	trace := newTracer(level, session.writer)
	trace.progress = session.setProgress
	started := trace.now()

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
	hops := len(target.JumpRoute()) + 1
	trace.stage(terminal.ConnectionOpeningSession, target, hops, hops)
	trace.say(Brief, "認証できました。セッションを開きます。")

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
		session.forwarded.note(forwardAgent(client, remote, d.Auth.AgentSocket, session.writer))
	}

	if err := d.start(remote, target, size, session); err != nil {
		session.fail(fmt.Errorf("sshc: %w", err))
		closeAll(closers)
		return
	}
	session.markReady(nil)
	trace.say(Brief, "開きました。")
	trace.say(Full, "ここまで %s。", trace.since(started).Round(time.Millisecond))
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
	remote.Stdin = streams.In
	remote.Stdout = streams.Out
	// stderr を同じ道へ流すのは、端末がひとつだからである。分けて運んでも
	// 出す場所が無い。
	remote.Stderr = streams.Err

	for _, variable := range target.SetEnv {
		// 拒否されても続ける。サーバーが AcceptEnv を絞っているのは普通のことで、
		// それを理由に接続を諦める必要はない。
		_ = remote.Setenv(variable.Name, variable.Value)
	}

	if !strings.EqualFold(target.RequestTTY, "no") {
		modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
		if err := remote.RequestPty(TermName, int(size.Rows), int(size.Cols), modes); err != nil {
			return err
		}
	}
	if target.RemoteCommand != "" {
		return remote.Start(target.RemoteCommand)
	}
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

	if len(route) > 0 && trace.enabled(Detailed) {
		hops := make([]string, 0, len(route))
		for _, hop := range route {
			hops = append(hops, hop.Address())
		}
		trace.say(Detailed, "経由地 %d か所: %s", len(hops), strings.Join(hops, " → "))
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
		trace.say(Brief, "%s へ、%s を経由して繋ぎます。", target.Address(), "手前のホップ")
	} else {
		trace.say(Brief, "%s へ繋ぎます（利用者 %s）。", target.Address(), target.User)
	}
	trace.say(Detailed, "上限は %s です。", timeout)

	trace.stage(terminal.ConnectionDialing, target, hop, hops)
	conn, err := d.open(ctx, target, through, trace)
	if err != nil {
		trace.say(Brief, "繋がりませんでした: %v", err)
		return nil, err
	}
	trace.say(Detailed, "TCP が開きました（%s）。", trace.since(started).Round(time.Millisecond))

	authMethods, closeAuth := d.Auth.methodsWithCleanup(target, prompt)
	defer closeAuth()
	verifyHostKey := d.HostKeys.Callback(target, prompt)
	config := &ssh.ClientConfig{
		User: target.User,
		Auth: authMethods,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			trace.stage(terminal.ConnectionHostKey, target, hop, hops)
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
		Timeout:           timeout,
	}
	if trace.enabled(Detailed) {
		trace.say(Detailed, "試せる認証は %d 通りです。", len(config.Auth))
	}
	connection, channels, requests, err := newClientConn(ctx, conn, target.Address(), config)
	if err != nil {
		trace.say(Brief, "握手が通りませんでした: %v", err)
		return nil, err
	}
	trace.say(Full, "相手が名乗った版は %s です。", connection.ServerVersion())
	trace.say(Detailed, "握手が通りました（%s）。", trace.since(started).Round(time.Millisecond))
	trace.say(Brief, "%s の接続完了（%d/%d）。", connectionTarget(target), hop, hops)
	trace.stage(terminal.ConnectionAuthenticated, target, hop, hops)
	return ssh.NewClient(connection, channels, requests), nil
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
	if target.ProxyCommand != "" {
		// そのプログラムはこの機械で走る。手前のホップの中ではない。
		// 踏み台の向こうのホップに書かれていたら、走らせても設定が言っている
		// 場所には届かない。
		if through != nil {
			return nil, ErrProxyCommandThroughJump
		}
		trace.announce("ProxyCommand を起こします: %s", target.ProxyCommand)
		return startProxyCommand(target.ProxyCommand)
	}
	if through != nil {
		return through.DialContext(ctx, "tcp", target.Address())
	}
	if d.Dial != nil {
		return d.Dial(ctx, "tcp", target.Address())
	}
	return (&net.Dialer{}).DialContext(ctx, "tcp", target.Address())
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
