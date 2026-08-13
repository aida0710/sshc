package sshclient

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"sshc/internal/terminal"
)

// LoopbackHost は、転送が bind する唯一のアドレスである。
//
// OpenSSH は `LocalForward 0.0.0.0:8080` や `GatewayPorts yes` で他の機械へ
// 開けるが、**このアプリケーションは開かない。** 常駐するプロセスが同じ機械の
// 他の面——HTTP サーバーも vault も——をループバックに閉じているのと同じ判断で
// ある。
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
	mutex     sync.Mutex
	opened    []terminal.Forward
	listeners []net.Listener
}

func (f *forwards) note(entry terminal.Forward) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.opened = append(f.opened, entry)
}

func (f *forwards) keep(listener net.Listener) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.listeners = append(f.listeners, listener)
}

func (f *forwards) list() []terminal.Forward {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return append([]terminal.Forward(nil), f.opened...)
}

// close は、開いた listener をすべて閉じる。
//
// **閉じ忘れたポートは、そのプロセスが死ぬまで埋まる。**
func (f *forwards) close() {
	f.mutex.Lock()
	listeners := f.listeners
	f.listeners = nil
	f.mutex.Unlock()
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

// open は、この接続の上に設定が求めた転送を開く。
//
// **開けなかった転送があっても接続は続ける。** ポートが埋まっているのは普通の
// 出来事であり、それを理由にセッションごと失う方が困る。理由はその転送の
// Problem に残り、端末にも 1 行出る。
func (f *forwards) open(client *ssh.Client, specs []ForwardSpec, report io.Writer) {
	for _, spec := range specs {
		listener, err := net.Listen("tcp", spec.Address())
		entry := terminal.Forward{Kind: spec.Kind, Listen: spec.Address(), To: spec.To}
		if err != nil {
			entry.Problem = err.Error()
			f.note(entry)
			_, _ = io.WriteString(report, "sshc: "+spec.Address()+" could not be opened: "+err.Error()+"\r\n")
			continue
		}
		f.keep(listener)
		f.note(entry)
		_, _ = io.WriteString(report, "sshc: forwarding "+describe(spec)+"\r\n")

		go accept(listener, client, spec)
	}
}

func describe(spec ForwardSpec) string {
	if spec.Kind == terminal.ForwardDynamic {
		return spec.Address() + " as a SOCKS5 proxy"
	}
	return spec.Address() + " to " + spec.To
}

// accept は、開いた listener に届く接続を、この SSH の上へ流し続ける。
func accept(listener net.Listener, client *ssh.Client, spec ForwardSpec) {
	for {
		local, err := listener.Accept()
		if err != nil {
			// listener が閉じた。セッションが終わったということである。
			return
		}
		go serve(local, client, spec)
	}
}

func serve(local net.Conn, client *ssh.Client, spec ForwardSpec) {
	defer func() { _ = local.Close() }()

	destination := spec.To
	if spec.Kind == terminal.ForwardDynamic {
		asked, err := readSOCKS5(local)
		if err != nil {
			return
		}
		destination = asked
	}

	remote, err := client.Dial("tcp", destination)
	if err != nil {
		// **この 1 本だけを閉じる。** listener は生きている。
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
// **鍵そのものは渡らない。渡るのは鍵を使う権利である。** リモートのプロセスが
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
