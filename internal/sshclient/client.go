package sshclient

import (
	"context"
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
}

// Open は、この alias のセッションをひとつ返す。
//
// **握手を待たずに返る。** 接続の途中で人に尋ねることがあり、その問いは
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

	client, closers, err := d.chain(ctx, target, prompt)
	if err != nil {
		session.fail("sshc: " + err.Error())
		return
	}
	closers = append(closers, client)

	remote, err := client.NewSession()
	if err != nil {
		session.fail("sshc: " + err.Error())
		closeAll(closers)
		return
	}

	size := session.attach(remote, closers)

	// 転送はチャンネルを開いたあと、シェルを起こす前に開く。**開いていることを
	// 端末の一行目に書く**ためであり、失敗しても接続は続ける。
	session.forwarded.open(client, target.Forwards, session.writer)
	if target.AgentForward {
		session.forwarded.note(forwardAgent(client, remote, d.Auth.AgentSocket, session.writer))
	}

	if err := d.start(remote, target, size, session); err != nil {
		session.fail("sshc: " + err.Error())
		closeAll(closers)
		return
	}
	session.run(remote, keepAliveLoop(client, target.KeepAlive, target.KeepAliveMax, session.done))
}

// start は、チャンネルの上で端末を要求し、シェルかコマンドを起こす。
func (d Dialer) start(remote *ssh.Session, target Target, size terminal.Size, session *Session) error {
	remote.Stdin = session.input
	remote.Stdout = session.writer
	// stderr を同じ道へ流すのは、端末がひとつだからである。分けて運んでも
	// 出す場所が無い。
	remote.Stderr = session.writer

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
// **プログラムは一つも起こさない。** 各ホップの SSH チャンネルの上に、次の
// ホップへの TCP を載せるだけである。ProxyCommand との違いはそこにある。
func (d Dialer) chain(ctx context.Context, target Target, prompt Prompter) (*ssh.Client, []io.Closer, error) {
	var closers []io.Closer
	var through *ssh.Client

	for _, hop := range target.Jump {
		client, err := d.connectOne(ctx, hop, through, prompt)
		if err != nil {
			closeAll(closers)
			return nil, nil, err
		}
		closers = append(closers, client)
		through = client
	}

	client, err := d.connectOne(ctx, target, through, prompt)
	if err != nil {
		closeAll(closers)
		return nil, nil, err
	}
	return client, closers, nil
}

// connectOne は、ホップひとつへ繋ぐ。through が非 nil なら、その接続の上を通る。
func (d Dialer) connectOne(
	ctx context.Context, target Target, through *ssh.Client, prompt Prompter,
) (*ssh.Client, error) {
	timeout := target.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := d.open(ctx, target, through)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User:            target.User,
		Auth:            d.Auth.Methods(target, prompt),
		HostKeyCallback: d.HostKeys.Callback(target, prompt),
		Timeout:         timeout,
	}
	// 握手そのものにも締め切りを掛ける。応答を返さないまま繋いだままの相手が、
	// この goroutine を保持し続けないようにするためである。
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	connection, channels, requests, err := ssh.NewClientConn(conn, target.Address(), config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(connection, channels, requests), nil
}

func (d Dialer) open(ctx context.Context, target Target, through *ssh.Client) (net.Conn, error) {
	if through != nil {
		return through.DialContext(ctx, "tcp", target.Address())
	}
	if d.Dial != nil {
		return d.Dial(ctx, "tcp", target.Address())
	}
	return (&net.Dialer{}).DialContext(ctx, "tcp", target.Address())
}

func closeAll(closers []io.Closer) {
	for index := len(closers) - 1; index >= 0; index-- {
		_ = closers[index].Close()
	}
}
