package sshclient_test

import (
	"context"
	"flag"
	"io"
	"net"
	"os"
	"runtime"
	"testing"

	"golang.org/x/crypto/ssh"

	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// relayTo は、この検査バイナリを ProxyCommand として起こしたときの行き先である。
//
// **中継のために別のプログラムを持ち込まない。** `nc` がある機械と無い機械が
// あり、Windows にはそもそも無い。os/exec 自身の検査が使っているのと同じ手で、
// この検査バイナリを自分で起こす。
var relayTo = flag.String("sshc.relay", "", "ProxyCommand として起きたとき、ここへ中継する")

// TestProxyCommandRelay は、ProxyCommand として起こされたときだけ働く。
//
// **標準出力に何も混ぜてはならない。** ここを流れているのは SSH の
// バイト列であり、testing が最後に書く "PASS" もその中に混ざる。だから
// 中継が終わったらそのまま抜ける。
func TestProxyCommandRelay(t *testing.T) {
	if *relayTo == "" {
		t.Skip("ProxyCommand として起こされたときだけ働く")
	}
	conn, err := net.Dial("tcp", *relayTo)
	if err != nil {
		os.Exit(1)
	}
	go func() { _, _ = io.Copy(conn, os.Stdin) }()
	_, _ = io.Copy(os.Stdout, conn)
	_ = conn.Close()
	os.Exit(0)
}

// relayCommand は、この検査バイナリを中継として起こす綴りを組み立てる。
func relayCommand(t *testing.T, address string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return `"` + self + `" -test.run=^TestProxyCommandRelay$ -sshc.relay=` + address
}

// closedPort は、誰も待ち受けていないポートを返す。
//
// **ここが要である。** target の宛先をここに向けておけば、TCP で繋ぎに行った
// 瞬間に失敗する——繋がったなら、通ったのは ProxyCommand である。
func closedPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()
	return port
}

// **ProxyCommand を起こして、その標準入出力の上で SSH を話す。**
//
// かつてこの設定は断られていた。`cloudflared access ssh`、`aws ssm`、会社の
// bastion helper——それらを書いている接続先は、一覧に並んでいるのに押すと
// 断られた。
func TestAConnectionGoesThroughItsProxyCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		// cmd.exe の引用規則が違うので、あちらは別に確かめる。
		t.Skip("cmd.exe の綴りは proxycommand_windows_test.go が見る")
	}
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			_, _ = io.WriteString(channel, "through the command\r\n")
		},
	})

	// **宛先は、誰も居ないポートである。** TCP で行けば必ず失敗する。
	port := closedPort(t)
	target := sshclient.Target{
		Alias: "bastion", HostName: "127.0.0.1", Port: port, User: "ops",
		Identities:   []string{path},
		Methods:      sshclient.DefaultMethods(),
		ProxyCommand: relayCommand(t, server.Address()),
	}

	known := knownHostsLine("[127.0.0.1]:"+port, server.HostKey.PublicKey())
	dialer := sshclient.Dialer{
		Auth:     sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return []byte(known), nil }},
	}

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "through the command")
}

// **黙って起こさない。** 既定が無言であることの、ただ一つの例外である。
//
// 何が走るかを知らないまま走ることが無いようにするためであり、繋がるまで
// 数秒かかる理由もそこにある。
func TestTheProxyCommandIsAnnouncedEvenWhenQuiet(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cmd.exe の綴りは proxycommand_windows_test.go が見る")
	}
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})

	port := closedPort(t)
	command := relayCommand(t, server.Address())
	target := sshclient.Target{
		Alias: "bastion", HostName: "127.0.0.1", Port: port, User: "ops",
		Identities: []string{path}, Methods: sshclient.DefaultMethods(), ProxyCommand: command,
	}
	known := knownHostsLine("[127.0.0.1]:"+port, server.HostKey.PublicKey())
	dialer := sshclient.Dialer{
		Auth:     sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return []byte(known), nil }},
		// **無言を選んでいる。** それでもこの一行は出る。
		Verbosity: func() sshclient.Verbosity { return sshclient.Quiet },
	}

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "ProxyCommand を起こします")
}

// **繋がらなかった理由は、たいていプログラムの標準エラーにしかない。**
//
// `ssh -W` は "Connection refused" をそこへ書く。握手の失敗としてだけ見せると、
// 読む人には何が起きたのか分からない。
func TestAFailingProxyCommandSaysWhatItComplainedAbout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cmd.exe の綴りは proxycommand_windows_test.go が見る")
	}
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})

	port := closedPort(t)
	target := sshclient.Target{
		Alias: "bastion", HostName: "127.0.0.1", Port: port, User: "ops",
		Identities: []string{path}, Methods: sshclient.DefaultMethods(),
		ProxyCommand: "echo zzz-could-not-reach-it >&2; exit 1",
	}
	known := knownHostsLine("[127.0.0.1]:"+port, server.HostKey.PublicKey())
	dialer := sshclient.Dialer{
		Auth:     sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return []byte(known), nil }},
	}

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "zzz-could-not-reach-it")
}

// **踏み台の向こうのホップは ProxyCommand を使えない。**
//
// そのプログラムはこの機械で走る。手前のホップの中ではない。走らせても、
// 設定が言っている場所には届かない。
func TestAHopReachedThroughAnotherRefusesItsProxyCommand(t *testing.T) {
	path, contents, public := keyPair(t)
	gateway := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	gateway.allow("203.0.113.9:22")

	inner := sshclient.Target{
		Alias: "inner", HostName: "203.0.113.9", Port: "22", User: "ops",
		Identities: []string{path}, Methods: sshclient.DefaultMethods(),
		ProxyCommand: "/bin/true",
	}
	first := sshclient.Target{
		Alias: "gateway", HostName: gateway.Host(), Port: gateway.Port(), User: "ops",
		Identities: []string{path}, Methods: sshclient.DefaultMethods(),
	}
	// gateway を先に通り、その上で inner へ行く形。
	inner.Jump = []sshclient.Target{first}

	known := knownHostsLine(net.JoinHostPort(gateway.Host(), gateway.Port()), gateway.HostKey.PublicKey())
	if gateway.Port() != "22" {
		known = knownHostsLine("["+gateway.Host()+"]:"+gateway.Port(), gateway.HostKey.PublicKey())
	}
	dialer := sshclient.Dialer{
		Auth: sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{
			Read: func() ([]byte, error) { return []byte(known), nil },
			Add:  func(knownhosts.Candidate) error { return nil },
		},
	}

	process, err := dialer.Open(context.Background(), inner, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()

	readUntil(t, process, "cannot use ProxyCommand")
}
