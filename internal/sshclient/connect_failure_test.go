package sshclient_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

func TestConnectFailuresAreToldApartByWhatHappened(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want sshclient.ConnectFailure
	}{
		{"a closed port", fmt.Errorf("dial tcp: %w", syscall.ECONNREFUSED), sshclient.ConnectFailureRefused},
		{"the connect timeout", context.DeadlineExceeded, sshclient.ConnectFailureTimedOut},
		{"a TCP timeout", fmt.Errorf("dial tcp: %w", syscall.ETIMEDOUT), sshclient.ConnectFailureTimedOut},
		{"a name that does not exist", &net.DNSError{Err: "no such host", IsNotFound: true}, sshclient.ConnectFailureHostNotFound},
		{"a name server out of reach", &net.DNSError{Err: "server misbehaving", IsTemporary: true}, sshclient.ConnectFailureNoNetwork},
		{"no route out of this machine", fmt.Errorf("dial tcp: %w", syscall.ENETUNREACH), sshclient.ConnectFailureNoNetwork},
		{"a network that went down", fmt.Errorf("dial tcp: %w", syscall.ENETDOWN), sshclient.ConnectFailureNoNetwork},
		{"a host that does not answer on its network", fmt.Errorf("dial tcp: %w", syscall.EHOSTUNREACH), sshclient.ConnectFailureHostUnreachable},
		{"a server that went away", fmt.Errorf("ssh: handshake failed: %w", io.EOF), sshclient.ConnectFailureClosed},
		{"a connection reset", fmt.Errorf("read tcp: %w", syscall.ECONNRESET), sshclient.ConnectFailureClosed},
		{"a jump host refused", &ssh.OpenChannelError{Reason: ssh.ConnectionFailed, Message: "Connection refused"}, sshclient.ConnectFailureRefused},
		{"cancelled by the person", context.Canceled, sshclient.ConnectFailureUnknown},
		{"something else", errors.New("ssh: no common algorithm for key exchange"), sshclient.ConnectFailureUnknown},
	}
	for _, testCase := range cases {
		if got := sshclient.ClassifyConnectFailure(testCase.err); got != testCase.want {
			t.Errorf("%s: ClassifyConnectFailure(%v) = %v, want %v", testCase.name, testCase.err, got, testCase.want)
		}
	}
}

func TestAFailureWithoutASentenceStillSaysWhatTheLibrarySaid(t *testing.T) {
	t.Parallel()
	const original = "ssh: no common algorithm for key exchange"
	if got := sshclient.FailureSentence(errors.New(original)); !strings.Contains(got, original) {
		t.Errorf("FailureSentence = %q; the reason is lost", got)
	}
}

// PAM の無い OpenSSH は、keyboard-interactive を問いを出さずに断る。これは認証の失敗であり、
// 再接続しても通らない。
func TestAServerThatRefusesKeyboardInteractiveWithoutAskingIsAnAuthenticationFailure(t *testing.T) {
	path, contents, _ := keyPair(t)
	server := newTestServer(t, serverOptions{
		AcceptKeys:                   []ssh.PublicKey{newHostKey(t).PublicKey()},
		KeyboardRefusesWithoutAsking: true,
	})
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	target := targetWith(server, path)
	target.Methods = sshclient.DefaultMethods()

	process, err := dialerFor(t, server, auth).Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	seen := readUntil(t, process, "サーバーが受け付ける方式：")
	drain(process)
	if readyErr := awaitReady(process); !errors.Is(readyErr, sshclient.ErrAuthenticationRejected) {
		t.Fatalf("Ready = %v, want an authentication rejection", readyErr)
	}
	if strings.Contains(seen, "unexpected message type") {
		t.Errorf("the terminal shows the words of the SSH library: %q", seen)
	}
}

// 読むと EOF になる接続は、ホスト鍵の答えを待つ間にサーバーが切った接続である。
func cutOnRead(conn net.Conn) { _ = conn.(*net.TCPConn).CloseRead() }

// sshd の LoginGraceTime で切られたあとに受け入れても、失敗にせず、もう一度鍵を聞かずにつなぐ。
func TestAConnectionCutWhileTheHostKeyQuestionWaitsIsOpenedAgainWithoutAskingTwice(t *testing.T) {
	server := newTestServer(t, serverOptions{
		Password: "hunter2",
		OnShell:  func(channel ssh.Channel) { _, _ = io.WriteString(channel, "connected-again\r\n") },
	})
	dials := make(chan net.Conn, 2)
	var remembered []knownhosts.Candidate
	dialer := sshclient.Dialer{
		Auth: sshclient.Auth{Password: func(sshclient.Target) (string, bool) { return "hunter2", true }},
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
			if err == nil {
				dials <- conn
			}
			return conn, err
		},
		HostKeys: sshclient.HostKeys{
			Read: func(string) ([]byte, error) { return nil, nil },
			Add: func(_ string, candidate knownhosts.Candidate) error {
				remembered = append(remembered, candidate)
				return nil
			},
		},
	}
	target := targetWith(server)

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	readUntil(t, process, "Are you sure you want to continue connecting")
	cutOnRead(<-dials)
	if _, err := process.Write([]byte("yes\r")); err != nil {
		t.Fatal(err)
	}

	seen := readUntil(t, process, "connected-again")
	if !strings.Contains(seen, "つなぎ直します") {
		t.Errorf("the terminal does not say it connected again: %q", seen)
	}
	if strings.Contains(seen, "Are you sure") {
		t.Errorf("the same key was asked about twice: %q", seen)
	}
	if readyErr := awaitReady(process); readyErr != nil {
		t.Fatalf("Ready = %v", readyErr)
	}
	if len(remembered) != 1 {
		t.Errorf("known_hosts was written %d times, want once", len(remembered))
	}
}

// SFTP のタブの接続も同じである。ここでは受け入れた鍵を known_hosts へ書けない接続を確かめる。
func TestASubsystemConnectionCutWhileAnsweringIsOpenedAgainEvenWhenTheKeyCouldNotBeSaved(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	dialer, _ := unknownHostDialer(contents, "")
	dials := make(chan net.Conn, 2)
	dialer.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err == nil {
			dials <- conn
		}
		return conn, err
	}
	asked := 0
	ctx := sshclient.WithConnectionObserver(context.Background(), sshclient.ConnectionObserver{
		ConfirmHostKey: func(context.Context, string) (bool, error) {
			asked++
			cutOnRead(<-dials)
			return true, nil
		},
	})

	connection, err := dialer.Connect(ctx, targetWith(server, path))
	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	defer func() { _ = connection.Close() }()
	if asked != 1 {
		t.Errorf("asked %d times, want once", asked)
	}
}

// 受け入れていないのに切れた接続は、つなぎ直さない。
func TestAConnectionCutWithoutAQuestionIsNotOpenedAgain(t *testing.T) {
	dials := 0
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	dialer := sshclient.Dialer{
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dials++
			conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
			if err == nil {
				cutOnRead(conn)
			}
			return conn, err
		},
		HostKeys: sshclient.HostKeys{Read: func(string) ([]byte, error) { return nil, nil }},
	}
	target := targetWith(server)
	target.Timeout = 5 * time.Second

	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	drain(process)
	if readyErr := awaitReady(process); readyErr == nil {
		t.Fatal("a cut connection was reported as connected")
	}
	if dials != 1 {
		t.Errorf("dialed %d times, want once", dials)
	}
}
