package sshclient_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// testServer は、このプロセスの中で回る本物の SSH サーバーである。
//
// 偽物に置き換えない。このパッケージの検査対象はプロトコルそのものなので、
// ハンドシェイクを模したものと突き合わせても何も確かめられない。
//
// 待ち受けるのは 127.0.0.1 の任意ポートである。net.Pipe では動かない。
// あれは同期的で、書き込みが対応する読み取りまで返らない。SSH の版文字列の
// 交換は両側が先に書くので、その場で相互に固まる。x/crypto/ssh 自身の検査が
// ループバックのソケットを使っているのは同じ理由である。リモートにも外部
// バイナリにも触れないという約束は、これで守られている。
type testServer struct {
	t          *testing.T
	config     *ssh.ServerConfig
	HostKey    ssh.Signer
	ExitCode   int
	Banner     string
	OnShell    func(channel ssh.Channel)
	acceptKeys []ssh.PublicKey
	password   string
	keyboard   map[string]string
	options    serverOptions
	listener   net.Listener

	mutex     sync.Mutex
	ptyTerm   string
	ptySize   [2]uint32
	sizes     [][2]uint32
	env       []string
	command   string
	shellRan  bool
	usedKey   ssh.PublicKey
	dialed    []string
	connected int
	// attempts は、認証がこのサーバーへ届いた回数である。鍵を集めるだけの
	// 操作が資格情報を差し出していないことを、これで言う。
	attempts int
	// keepAlives は、接続そのものへ届いた keepalive の回数である。
	keepAlives int
}

type serverOptions struct {
	// AcceptKeys は公開鍵認証で通す鍵。空なら公開鍵認証を拒む。
	AcceptKeys []ssh.PublicKey
	// Password は password 認証で通す文字列。空なら拒む。
	Password string
	// Keyboard は keyboard-interactive の質問と正解。
	Keyboard map[string]string
	// ExitCode はシェルの終了コード。
	ExitCode int
	// AllowDirectTCPIP は direct-tcpip チャンネルを通すか。ProxyJump の手前側で要る。
	AllowDirectTCPIP bool
	// Reached は、direct-tcpip の行き先ごとに返す接続である。
	Reached map[string]func() net.Conn
	// OnShell は、シェルが開いたあとにサーバー側が行うことである。
	OnShell func(channel ssh.Channel)
	// OnAgentChannel は、借りた agent へリモート側から話しかける。
	OnAgentChannel func(conn net.Conn)
	// Banner は、認証の前にサーバーが送る文言である。
	Banner string
	// ECDSAHostKey は、ed25519 に加えて ECDSA のホスト鍵も持たせる。普通の
	// Ubuntu が三種類持っているのと同じ状況であり、どれを名乗るかを決めるのは
	// クライアントの優先順である。
	ECDSAHostKey bool
}

func newTestServer(t *testing.T, options serverOptions) *testServer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	server := &testServer{
		t: t, HostKey: signer, ExitCode: options.ExitCode,
		acceptKeys: options.AcceptKeys, password: options.Password,
		keyboard: options.Keyboard, OnShell: options.OnShell,
		listener: listener,
	}
	config := &ssh.ServerConfig{}
	config.AddHostKey(signer)
	if options.ECDSAHostKey {
		private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		other, err := ssh.NewSignerFromKey(private)
		if err != nil {
			t.Fatal(err)
		}
		config.AddHostKey(other)
	}
	if len(options.AcceptKeys) > 0 {
		config.PublicKeyCallback = func(_ ssh.ConnMetadata, offered ssh.PublicKey) (*ssh.Permissions, error) {
			server.noteAttempt()
			for _, accepted := range server.acceptKeys {
				if string(accepted.Marshal()) == string(offered.Marshal()) {
					server.mutex.Lock()
					server.usedKey = offered
					server.mutex.Unlock()
					return &ssh.Permissions{}, nil
				}
			}
			return nil, errors.New("unknown public key")
		}
	}
	if options.Password != "" {
		config.PasswordCallback = func(_ ssh.ConnMetadata, offered []byte) (*ssh.Permissions, error) {
			server.noteAttempt()
			if string(offered) == server.password {
				return &ssh.Permissions{}, nil
			}
			return nil, errors.New("wrong password")
		}
	}
	if len(options.Keyboard) > 0 {
		config.KeyboardInteractiveCallback = func(
			_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge,
		) (*ssh.Permissions, error) {
			server.noteAttempt()
			questions := make([]string, 0, len(server.keyboard))
			for question := range server.keyboard {
				questions = append(questions, question)
			}
			echos := make([]bool, len(questions))
			answers, err := challenge("", "", questions, echos)
			if err != nil {
				return nil, err
			}
			for index, question := range questions {
				if index >= len(answers) || answers[index] != server.keyboard[question] {
					return nil, errors.New("wrong answer")
				}
			}
			return &ssh.Permissions{}, nil
		}
	}
	if options.Banner != "" {
		config.BannerCallback = func(ssh.ConnMetadata) string { return options.Banner }
	}
	server.config = config
	server.options = options
	go server.accept()
	return server
}

// allow は、direct-tcpip でこの宛先へ通してよいと登録する。
//
// 本物の TCP へ繋ぐので、転送されたバイト列が端から端まで届くことを見られる。
func (s *testServer) allow(address string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.options.Reached == nil {
		s.options.Reached = map[string]func() net.Conn{}
	}
	s.options.Reached[address] = func() net.Conn {
		conn, err := net.Dial("tcp", address)
		if err != nil {
			return nil
		}
		return conn
	}
}

// Address は、このサーバーが待ち受けている 127.0.0.1 の宛先である。
func (s *testServer) Address() string { return s.listener.Addr().String() }

// Host と Port は、Target を組み立てるためのものである。
func (s *testServer) Host() string {
	host, _, _ := net.SplitHostPort(s.Address())
	return host
}

func (s *testServer) Port() string {
	_, port, _ := net.SplitHostPort(s.Address())
	return port
}

func (s *testServer) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.serve(conn)
	}
}

// Dial は、このサーバーへの接続をひとつ開く。
func (s *testServer) Dial() net.Conn {
	s.t.Helper()
	conn, err := net.Dial("tcp", s.Address())
	if err != nil {
		s.t.Fatal(err)
	}
	s.t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (s *testServer) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	connection, channels, requests, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		return
	}
	s.mutex.Lock()
	s.connected++
	s.mutex.Unlock()
	defer func() { _ = connection.Close() }()
	go func() {
		for request := range requests {
			if request.Type == "keepalive@openssh.com" {
				s.mutex.Lock()
				s.keepAlives++
				s.mutex.Unlock()
			}
			if request.WantReply {
				_ = request.Reply(false, nil)
			}
		}
	}()

	for newChannel := range channels {
		switch newChannel.ChannelType() {
		case "session":
			channel, channelRequests, err := newChannel.Accept()
			if err != nil {
				return
			}
			go s.session(connection, channel, channelRequests)
		case "direct-tcpip":
			s.forward(newChannel)
		default:
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported")
		}
	}
}

// forward は direct-tcpip を、Reached に登録された接続へ繋ぐ。
//
// これが ProxyJump の手前側である。奥のサーバーへの net.Conn を、この
// チャンネルの上に載せる。
func (s *testServer) forward(newChannel ssh.NewChannel) {
	var payload struct {
		Host string
		Port uint32
		Orig string
		Ourm uint32
	}
	if err := ssh.Unmarshal(newChannel.ExtraData(), &payload); err != nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, "bad payload")
		return
	}
	address := net.JoinHostPort(payload.Host, strconv.FormatUint(uint64(payload.Port), 10))
	s.mutex.Lock()
	s.dialed = append(s.dialed, address)
	open := s.options.Reached[address]
	s.mutex.Unlock()
	if !s.options.AllowDirectTCPIP || open == nil {
		_ = newChannel.Reject(ssh.ConnectionFailed, "no route")
		return
	}
	channel, requests, err := newChannel.Accept()
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)
	remote := open()
	if remote == nil {
		_ = channel.Close()
		return
	}
	go func() { _, _ = io.Copy(remote, channel); _ = remote.Close() }()
	go func() { _, _ = io.Copy(channel, remote); _ = channel.Close() }()
}

// channelConn は、SSH のチャンネルを net.Conn として見せる。agent の
// クライアントがそれを求めるからである。
type channelConn struct{ ssh.Channel }

func (channelConn) LocalAddr() net.Addr              { return dummyAddr{} }
func (channelConn) RemoteAddr() net.Addr             { return dummyAddr{} }
func (channelConn) SetDeadline(time.Time) error      { return nil }
func (channelConn) SetReadDeadline(time.Time) error  { return nil }
func (channelConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr struct{}

func (dummyAddr) Network() string { return "ssh" }
func (dummyAddr) String() string  { return "channel" }

func (s *testServer) session(connection ssh.Conn, channel ssh.Channel, requests <-chan *ssh.Request) {
	for request := range requests {
		switch request.Type {
		case "pty-req":
			term, width, height := parsePTY(request.Payload)
			s.mutex.Lock()
			s.ptyTerm, s.ptySize = term, [2]uint32{width, height}
			s.mutex.Unlock()
			s.reply(request, true)
		case "window-change":
			width, height := parseWindowChange(request.Payload)
			s.mutex.Lock()
			s.sizes = append(s.sizes, [2]uint32{width, height})
			s.mutex.Unlock()
		case "env":
			name, value := parseEnv(request.Payload)
			s.mutex.Lock()
			s.env = append(s.env, name+"="+value)
			s.mutex.Unlock()
			s.reply(request, true)
		case "auth-agent-req@openssh.com":
			s.reply(request, true)
			// agent のチャンネルはサーバーが開く。借りた側から
			// 話しかけるものなので、向きはこちらからである。
			if s.options.OnAgentChannel != nil {
				go s.openAgent(connection)
			}
		case "shell":
			s.mutex.Lock()
			s.shellRan = true
			s.mutex.Unlock()
			s.reply(request, true)
			go s.run(channel)
		case "exec":
			s.mutex.Lock()
			s.command = string(request.Payload[4:])
			s.mutex.Unlock()
			s.reply(request, true)
			go s.run(channel)
		default:
			s.reply(request, false)
		}
	}
}

// openAgent は、借りた agent へリモート側から繋ぐ。
func (s *testServer) openAgent(connection ssh.Conn) {
	channel, requests, err := connection.OpenChannel("auth-agent@openssh.com", nil)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)
	s.options.OnAgentChannel(channelConn{channel})
	_ = channel.Close()
}

func (s *testServer) run(channel ssh.Channel) {
	if s.OnShell != nil {
		s.OnShell(channel)
	}
	status := struct{ Status uint32 }{Status: uint32(s.ExitCode)}
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(&status))
	_ = channel.Close()
}

func (s *testServer) reply(request *ssh.Request, ok bool) {
	if request.WantReply {
		_ = request.Reply(ok, nil)
	}
}

// 観測したものを読む。

func (s *testServer) PTY() (string, [2]uint32) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.ptyTerm, s.ptySize
}

func (s *testServer) Sizes() [][2]uint32 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([][2]uint32(nil), s.sizes...)
}

func (s *testServer) Env() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]string(nil), s.env...)
}

func (s *testServer) Command() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.command
}

func (s *testServer) ShellRan() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.shellRan
}

func (s *testServer) noteAttempt() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.attempts++
}

// Attempts は、認証がこのサーバーへ届いた回数である。
func (s *testServer) KeepAlives() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.keepAlives
}

func (s *testServer) Attempts() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.attempts
}

func (s *testServer) Dialed() []string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]string(nil), s.dialed...)
}

func parsePTY(payload []byte) (term string, width, height uint32) {
	if len(payload) < 4 {
		return "", 0, 0
	}
	length := binary.BigEndian.Uint32(payload)
	if uint32(len(payload)) < 4+length+16 {
		return "", 0, 0
	}
	term = string(payload[4 : 4+length])
	rest := payload[4+length:]
	return term, binary.BigEndian.Uint32(rest), binary.BigEndian.Uint32(rest[4:])
}

func parseWindowChange(payload []byte) (width, height uint32) {
	if len(payload) < 8 {
		return 0, 0
	}
	return binary.BigEndian.Uint32(payload), binary.BigEndian.Uint32(payload[4:])
}

func parseEnv(payload []byte) (name, value string) {
	if len(payload) < 4 {
		return "", ""
	}
	nameLength := binary.BigEndian.Uint32(payload)
	if uint32(len(payload)) < 4+nameLength+4 {
		return "", ""
	}
	name = string(payload[4 : 4+nameLength])
	rest := payload[4+nameLength:]
	valueLength := binary.BigEndian.Uint32(rest)
	if uint32(len(rest)) < 4+valueLength {
		return name, ""
	}
	return name, string(rest[4 : 4+valueLength])
}

// この検査が、テストサーバー自身が本物のハンドシェイクを終えることを言う。
// これが無いと、以下のすべての検査は「繋がらないので何も起きなかった」を
// 通過として数えうる。
func TestTheTestServerCompletesAHandshake(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{signer.PublicKey()}})

	conn := server.Dial()
	client, channels, requests, err := ssh.NewClientConn(conn, "bastion:22", &ssh.ClientConfig{
		User:            "ops",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.FixedHostKey(server.HostKey.PublicKey()),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("handshake = %v", err)
	}
	defer func() { _ = client.Close() }()

	session, err := ssh.NewClient(client, channels, requests).NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	if !server.ShellRan() {
		t.Error("the server never saw a shell request")
	}
}
