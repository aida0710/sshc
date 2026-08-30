package sshclient

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"sshc/internal/terminal"
)

// LoopbackHost は、転送が bind する唯一のアドレスである。
//
// OpenSSH は `LocalForward 0.0.0.0:8080` や `GatewayPorts yes` で他の機械へ
// 開けるが、このアプリケーションは開かない。常駐プロセスが持つHTTPサーバーや
// vaultと同様に、外部へ意図せず公開される面を増やさないためである。
const LoopbackHost = "127.0.0.1"

// ErrInvalidForward は、読めない転送の指定を報告する。
var ErrInvalidForward = errors.New("that is not a forwarding specification this client understands")

// ForwardSpec は、設定に書かれた転送ひとつである。
type ForwardSpec struct {
	Kind string
	// ListenPort は、このマシンで開くポート。
	ListenPort string
	// Requested は、設定が bind したいと書いたアドレス。ループバック以外なら
	// 束ねたことを notice にする。
	Requested string
	// To は転送先。dynamic では空。
	To string
}

// Bound は、要求された bind がループバックへ束ねられたかを報告する。
func (s ForwardSpec) Bound() bool {
	return s.Requested != "" && !isLoopback(s.Requested)
}

// Address は、実際に開く場所である。
func (s ForwardSpec) Address() string { return net.JoinHostPort(LoopbackHost, s.ListenPort) }

// ParseLocalForward は `LocalForward` の値を読む。
//
//	8080 10.0.0.5:80
//	127.0.0.1:8080 10.0.0.5:80
//	[::1]:8080 [fd00::1]:80
func ParseLocalForward(value string) (ForwardSpec, error) {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return ForwardSpec{}, ErrInvalidForward
	}
	host, port, err := splitListen(fields[0])
	if err != nil {
		return ForwardSpec{}, err
	}
	if _, _, err := net.SplitHostPort(fields[1]); err != nil {
		return ForwardSpec{}, ErrInvalidForward
	}
	return ForwardSpec{Kind: terminal.ForwardLocal, ListenPort: port, Requested: host, To: fields[1]}, nil
}

// ParseDynamicForward は `DynamicForward` の値を読む。listen 側だけを取る。
func ParseDynamicForward(value string) (ForwardSpec, error) {
	fields := strings.Fields(value)
	if len(fields) != 1 {
		return ForwardSpec{}, ErrInvalidForward
	}
	host, port, err := splitListen(fields[0])
	if err != nil {
		return ForwardSpec{}, err
	}
	return ForwardSpec{Kind: terminal.ForwardDynamic, ListenPort: port, Requested: host}, nil
}

// splitListen は `port`、`host:port`、`[v6]:port` を分ける。
func splitListen(field string) (host, port string, err error) {
	if !strings.Contains(field, ":") {
		if !validPort(field) {
			return "", "", ErrInvalidForward
		}
		return "", field, nil
	}
	host, port, splitErr := net.SplitHostPort(field)
	if splitErr != nil || !validPort(port) {
		return "", "", ErrInvalidForward
	}
	return host, port, nil
}

func validPort(value string) bool {
	number, err := strconv.Atoi(value)
	return err == nil && number > 0 && number <= 65535
}

// isLoopback は、その名前がこの機械だけを指すかを報告する。
func isLoopback(host string) bool {
	switch strings.ToLower(host) {
	case "", "localhost":
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

// forwards は、ひとつのセッションが開いた転送の全体である。
type forwards struct {
	mutex  sync.Mutex
	opened []managedForward
	next   uint64
	closed bool
}

type managedForward struct {
	view     terminal.Forward
	listener net.Listener
}

func (f *forwards) nextID() string {
	f.next++
	return "pf-" + strconv.FormatUint(f.next, 10)
}

func (f *forwards) note(entry terminal.Forward) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if entry.ID == "" {
		entry.ID = f.nextID()
	}
	f.opened = append(f.opened, managedForward{view: entry})
}

func (f *forwards) list() []terminal.Forward {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	result := make([]terminal.Forward, 0, len(f.opened))
	for _, entry := range f.opened {
		result = append(result, entry.view)
	}
	return result
}

// close は、開いた listener をすべて閉じる。
//
// 閉じ忘れたポートは、そのプロセスが終了するまで埋まる。
func (f *forwards) close() {
	f.mutex.Lock()
	f.closed = true
	listeners := make([]net.Listener, 0, len(f.opened))
	for _, entry := range f.opened {
		if entry.listener != nil {
			listeners = append(listeners, entry.listener)
		}
	}
	f.mutex.Unlock()
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

// open は、この接続の上に設定が求めた転送を開く。
//
// 開けなかった転送があっても接続は続ける。ポートが埋まっているのは普通の
// 出来事であり、それを理由にセッションごと失う方が困る。理由はその転送の
// Problem に残り、端末にも 1 行出る。
func (f *forwards) open(client *ssh.Client, specs []ForwardSpec, report io.Writer) {
	for _, spec := range specs {
		_, _ = f.start(client, spec, false, report, true)
	}
}

// start opens one forwarding listener. Configuration-derived failures remain in
// the session list; an explicitly requested temporary failure is returned and
// leaves no misleading entry behind.
func (f *forwards) start(
	client *ssh.Client, spec ForwardSpec, temporary bool, report io.Writer, keepFailure bool,
) (terminal.Forward, error) {
	listener, err := net.Listen("tcp", spec.Address())
	entry := terminal.Forward{Kind: spec.Kind, Listen: spec.Address(), To: spec.To, Temporary: temporary}
	if err != nil {
		entry.Problem = err.Error()
		if keepFailure {
			f.note(entry)
		}
		if report != nil {
			_, _ = io.WriteString(report, "sshc: "+spec.Address()+" could not be opened: "+err.Error()+"\r\n")
		}
		return entry, err
	}

	f.mutex.Lock()
	if f.closed {
		f.mutex.Unlock()
		_ = listener.Close()
		return terminal.Forward{}, terminal.ErrNotConnected
	}
	entry.ID = f.nextID()
	entry.Listen = listener.Addr().String()
	f.opened = append(f.opened, managedForward{view: entry, listener: listener})
	f.mutex.Unlock()
	if report != nil {
		_, _ = io.WriteString(report, "sshc: forwarding "+describe(spec)+"\r\n")
	}
	go accept(listener, client, spec)
	return entry, nil
}

func (f *forwards) stop(id string) error {
	f.mutex.Lock()
	index := -1
	var listener net.Listener
	for at, entry := range f.opened {
		if entry.view.ID == id {
			index = at
			listener = entry.listener
			break
		}
	}
	if index < 0 {
		f.mutex.Unlock()
		return terminal.ErrForwardNotFound
	}
	if listener == nil {
		f.mutex.Unlock()
		return terminal.ErrForwardUnavailable
	}
	f.opened = append(f.opened[:index], f.opened[index+1:]...)
	f.mutex.Unlock()
	return listener.Close()
}

func describe(spec ForwardSpec) string {
	if spec.Kind == terminal.ForwardDynamic {
		return spec.Address() + " as a SOCKS5 proxy"
	}
	return spec.Address() + " to " + spec.To
}

// maxConcurrentForwardConnections は、ひとつの listener が同時に扱う接続数である。
//
// loopback は外の機械から隠す境界であり、同じ機械の別ユーザーを隔離する境界では
// ない。共有機械の別ユーザーが接続を握ったままにしても、SSH セッション全体の
// goroutine とファイル記述子を際限なく増やせないようにする。
const maxConcurrentForwardConnections = 64

// accept は、開いた listener に届く接続を、この SSH の上へ流し続ける。
func accept(listener net.Listener, client *ssh.Client, spec ForwardSpec) {
	acceptLimited(listener, maxConcurrentForwardConnections, func(local net.Conn) {
		serve(local, client, spec)
	})
}

// acceptLimited は、ひとつの listener から同時に生やす処理数を制限する。
// 上限を超えた接続は goroutine を作る前に閉じる。
func acceptLimited(listener net.Listener, limit int, handle func(net.Conn)) {
	active := make(chan struct{}, limit)
	for {
		local, err := listener.Accept()
		if err != nil {
			// listener が閉じた。セッションが終わったということである。
			return
		}
		select {
		case active <- struct{}{}:
			go func() {
				defer func() { <-active }()
				handle(local)
			}()
		default:
			_ = local.Close()
		}
	}
}

// lingeringClose は、最後に書いたものが相手に届く見込みを作ってから閉じる。
//
// 順番が要点である: 先に FIN を送って「もう書かない」と伝え、それから残りを
// 読み捨てる。未読を空にしてから閉じるので RST にならない。
//
// 際限なく読まない。相手が流し続けるなら、こちらが断った事実は既に
// 書き終えている。上限と締切の両方を置く。
func lingeringClose(conn net.Conn) {
	if half, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = half.CloseWrite()
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, _ = io.Copy(io.Discard, io.LimitReader(conn, 4096))
}

func serve(local net.Conn, client *ssh.Client, spec ForwardSpec) {
	defer func() { _ = local.Close() }()

	destination := spec.To
	if spec.Kind == terminal.ForwardDynamic {
		asked, err := readSOCKS5(local)
		if err != nil {
			// 断りを届けてから閉じる。readSOCKS5 は要求を最後まで
			// 読まずに断るので、受信バッファに未読が残る。そのまま閉じると
			// TCP は FIN ではなく RST を送り、RST を受けた側は受信済みの
			// データを捨てる 。直前に書いた断りが、相手に届かずに消える。
			//
			// Linux と macOS では、たいてい相手が先に読み終えているので
			// 表に出なかった。Windows で出た。プロトコルの側が正しく、
			// 運が良かっただけである。
			lingeringClose(local)
			return
		}
		destination = asked
	}

	remote, err := client.Dial("tcp", destination)
	if err != nil {
		// この 1 本だけを閉じる。listener は実行中。
		return
	}
	defer func() { _ = remote.Close() }()

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(remote, local); done <- struct{}{} }()
	go func() { _, _ = io.Copy(local, remote); done <- struct{}{} }()
	<-done
}

// forwardAgent は、こちらの agent をリモートへ貸す。
//
// 鍵そのものは渡らない。渡るのは鍵を使う権利である。リモートのプロセスが
// 署名を求めると、その要求はこのチャンネルを通ってこちらの agent へ届く。
func forwardAgent(client *ssh.Client, session *ssh.Session, socket string, report io.Writer) terminal.Forward {
	entry := terminal.Forward{Kind: terminal.ForwardAgent}
	if socket == "" {
		entry.Problem = "no agent is reachable from this process"
		_, _ = io.WriteString(report, "sshc: agent forwarding was asked for but no agent is reachable\r\n")
		return entry
	}
	conn, err := (&net.Dialer{}).DialContext(context.Background(), "unix", socket)
	if err != nil {
		entry.Problem = err.Error()
		_, _ = io.WriteString(report, "sshc: agent forwarding: "+err.Error()+"\r\n")
		return entry
	}
	if err := agent.ForwardToAgent(client, agent.NewClient(conn)); err != nil {
		entry.Problem = err.Error()
		_ = conn.Close()
		return entry
	}
	if err := agent.RequestAgentForwarding(session); err != nil {
		entry.Problem = err.Error()
		_ = conn.Close()
		return entry
	}
	_, _ = io.WriteString(report, "sshc: forwarding this agent to the remote\r\n")
	return entry
}
