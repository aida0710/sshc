package sshclient_test

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"sshc/internal/effective"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
	"sshc/internal/validate"
)

// holdShell は、シェルを接続が閉じられるまで開いたままにする。OnShell の無いテストサーバーはシェルを
// すぐ終わらせ、セッションが終わるとパスワードは手放されるので、問いが出ないことを確かめられない。
func holdShell(channel ssh.Channel) { _, _ = io.Copy(io.Discard, channel) }

// openToAsk は、打ったパスワードの保存を尋ねてよい接続を開く。
func openToAsk(t *testing.T, dialer sshclient.Dialer, target sshclient.Target) terminal.Process {
	t.Helper()
	dialer.OfferPasswordSave = func(sshclient.Target) bool { return true }
	process, err := dialer.Open(context.Background(), target, terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close() })
	return process
}

func typedPassword(t *testing.T, process terminal.Process) (terminal.TypedPassword, bool) {
	t.Helper()
	asking, ok := process.(terminal.PasswordSaveAsking)
	if !ok {
		t.Fatal("an SSH session does not ask about saving a password")
	}
	return asking.TypedPassword()
}

func typeLine(t *testing.T, process terminal.Process, line string) {
	t.Helper()
	if _, err := process.Write([]byte(line + "\r")); err != nil {
		t.Fatal(err)
	}
}

func TestAPasswordTypedAtThePromptWaitsOnTheAnswerAfterItAuthenticates(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2", OnShell: holdShell})
	target := targetWith(server)
	target.Alias = "web-01"
	process := openToAsk(t, dialerFor(t, server, sshclient.Auth{}), target)

	readUntil(t, process, "Password for ")
	if _, waiting := typedPassword(t, process); waiting {
		t.Fatal("asked to save a password nobody typed yet")
	}
	typeLine(t, process, "hunter2")
	drain(process)
	if err := awaitReady(process); err != nil {
		t.Fatal(err)
	}

	typed, waiting := typedPassword(t, process)
	if !waiting || typed.Password != "hunter2" || typed.Alias != "web-01" || typed.Binding != target.AuthenticationBinding() {
		t.Fatalf("typed = %+v, %v", typed, waiting)
	}
}

func TestOnlyTheLastPasswordTypedIsTheOneThatAuthenticated(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2", OnShell: holdShell})
	target := targetWith(server)
	process := openToAsk(t, dialerFor(t, server, sshclient.Auth{}), target)

	readUntil(t, process, "Password for ")
	typeLine(t, process, "mistyped")
	readUntil(t, process, "Password for ")
	typeLine(t, process, "hunter2")
	drain(process)
	if err := awaitReady(process); err != nil {
		t.Fatal(err)
	}
	if typed, waiting := typedPassword(t, process); !waiting || typed.Password != "hunter2" {
		t.Fatalf("typed = %+v, %v", typed, waiting)
	}
}

func TestAPasswordThatFailsIsNeverOffered(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2", OnShell: holdShell})
	process := openToAsk(t, dialerFor(t, server, sshclient.Auth{}), targetWith(server))
	readUntil(t, process, "Password for ")
	for attempt := 0; attempt < 3; attempt++ {
		typeLine(t, process, "mistyped")
		if attempt < 2 {
			readUntil(t, process, "Password for ")
		}
	}
	drain(process)
	if err := awaitReady(process); err == nil {
		t.Fatal("three wrong passwords connected")
	}
	if _, waiting := typedPassword(t, process); waiting {
		t.Fatal("offered a password that did not authenticate")
	}
}

func TestAStoredPasswordIsNotOfferedBackForSaving(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2", OnShell: holdShell})
	auth := sshclient.Auth{Password: func(sshclient.Target) (string, bool) { return "hunter2", true }}
	process := openToAsk(t, dialerFor(t, server, auth), targetWith(server))
	drain(process)
	if err := awaitReady(process); err != nil {
		t.Fatal(err)
	}
	if _, waiting := typedPassword(t, process); waiting {
		t.Fatal("offered to save a password the vault already released")
	}
}

func TestAPasswordTypedAfterTheStoredOneWasRefusedIsOffered(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "rotated", OnShell: holdShell})
	auth := sshclient.Auth{Password: func(sshclient.Target) (string, bool) { return "before-rotation", true }}
	process := openToAsk(t, dialerFor(t, server, auth), targetWith(server))
	readUntil(t, process, "Saved password was rejected.")
	typeLine(t, process, "rotated")
	drain(process)
	if err := awaitReady(process); err != nil {
		t.Fatal(err)
	}
	if typed, waiting := typedPassword(t, process); !waiting || typed.Password != "rotated" {
		t.Fatalf("typed = %+v, %v", typed, waiting)
	}
}

func TestAKeyboardInteractiveAnswerCountsOnlyWhenTheQuestionNamesThePassword(t *testing.T) {
	cases := []struct {
		name, question string
		wanted         bool
	}{
		{"named password", "Password: ", true},
		{"named in Japanese", "パスワード: ", true},
		{"a verification code", "Verification code: ", false},
		{"an unnamed answer may be one time", "Enter PIN: ", false},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			server := newTestServer(t, serverOptions{Keyboard: map[string]string{item.question: "answered"}, OnShell: holdShell})
			target := targetWith(server)
			target.Methods = sshclient.Methods{Keyboard: true}
			process := openToAsk(t, dialerFor(t, server, sshclient.Auth{}), target)
			readUntil(t, process, strings.TrimSpace(item.question))
			typeLine(t, process, "answered")
			drain(process)
			if err := awaitReady(process); err != nil {
				t.Fatal(err)
			}
			if _, waiting := typedPassword(t, process); waiting != item.wanted {
				t.Fatalf("waiting = %v, want %v", waiting, item.wanted)
			}
		})
	}
}

func TestAWrongPasswordFollowedByAKeyIsNotOffered(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{Password: "hunter2", AcceptKeys: []ssh.PublicKey{public}, OnShell: holdShell})
	target := targetWith(server, path)
	target.Methods = sshclient.Methods{Preferred: []string{"password", "publickey"}, Password: true, PublicKey: true}
	auth := sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }}
	process := openToAsk(t, dialerFor(t, server, auth), target)

	for attempt := 0; attempt < 3; attempt++ {
		readUntil(t, process, "Password for ")
		typeLine(t, process, "mistyped")
	}
	drain(process)
	if err := awaitReady(process); err != nil {
		t.Fatal(err)
	}
	if _, waiting := typedPassword(t, process); waiting {
		t.Fatal("offered a password the server refused before the key got in")
	}
}

func TestAPasswordTypedForAJumpHostIsNotThePasswordOfTheDestination(t *testing.T) {
	path, contents, public := keyPair(t)
	inner := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}, OnShell: holdShell})
	edge := newTestServer(t, serverOptions{
		Password: "edge-password", AllowDirectTCPIP: true,
		Reached: map[string]func() net.Conn{inner.Address(): func() net.Conn { return inner.Dial() }},
	})
	known := knownHostsLine("["+edge.Host()+"]:"+edge.Port(), edge.HostKey.PublicKey()) +
		knownHostsLine("["+inner.Host()+"]:"+inner.Port(), inner.HostKey.PublicKey())
	dialer := sshclient.Dialer{
		Auth:     sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{Read: func(string) ([]byte, error) { return []byte(known), nil }},
	}
	target := targetWith(inner, path)
	target.Alias = "destination"
	jump := targetWith(edge)
	jump.Alias = "bastion"
	target.Jump = []sshclient.Target{jump}

	process := openToAsk(t, dialer, target)
	readUntil(t, process, "Password for ops@"+edge.Address()+" (bastion): ")
	typeLine(t, process, "edge-password")
	drain(process)
	if err := awaitReady(process); err != nil {
		t.Fatal(err)
	}
	if _, waiting := typedPassword(t, process); waiting {
		t.Fatal("offered the password of the jump host for the destination")
	}
}

func TestNothingIsKeptWhenTheConnectionIsNotAskedAbout(t *testing.T) {
	cases := map[string]func(sshclient.Target) bool{
		"no offer wired": nil,
		"refused":        func(sshclient.Target) bool { return false },
	}
	for name, offer := range cases {
		t.Run(name, func(t *testing.T) {
			server := newTestServer(t, serverOptions{Password: "hunter2", OnShell: holdShell})
			dialer := dialerFor(t, server, sshclient.Auth{})
			dialer.OfferPasswordSave = offer
			process, err := dialer.Open(context.Background(), targetWith(server), terminal.Size{Cols: 80, Rows: 24})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = process.Close() }()
			readUntil(t, process, "Password for ")
			typeLine(t, process, "hunter2")
			drain(process)
			if err := awaitReady(process); err != nil {
				t.Fatal(err)
			}
			if _, waiting := typedPassword(t, process); waiting {
				t.Fatal("kept a typed password")
			}
		})
	}
}

func TestClosingTheSessionLetsGoOfThePassword(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2", OnShell: holdShell})
	process := openToAsk(t, dialerFor(t, server, sshclient.Auth{}), targetWith(server))
	readUntil(t, process, "Password for ")
	typeLine(t, process, "hunter2")
	drain(process)
	if err := awaitReady(process); err != nil {
		t.Fatal(err)
	}
	if _, waiting := typedPassword(t, process); !waiting {
		t.Fatal("the password was let go of before the session ended")
	}
	if err := process.Close(); err != nil {
		t.Fatal(err)
	}
	if _, waiting := typedPassword(t, process); waiting {
		t.Fatal("a closed session still holds the password")
	}
}

func TestAShellThatEndsLetsGoOfThePassword(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	process := openToAsk(t, dialerFor(t, server, sshclient.Auth{}), targetWith(server))
	readUntil(t, process, "Password for ")
	typeLine(t, process, "hunter2")
	drain(process)
	process.Wait()
	if _, waiting := typedPassword(t, process); waiting {
		t.Fatal("a session whose shell ended still holds the password")
	}
}

func TestADirectDestinationReadsTheConfigAndBeatsItWithWhatWasTyped(t *testing.T) {
	resolve := func(alias string) (effective.Values, error) {
		return effective.Values{Entries: map[string][]string{
			"hostname": {alias}, "user": {"from-config"}, "port": {"2200"}, "serveraliveinterval": {"30"},
		}}, nil
	}
	cases := []struct {
		destination validate.DirectDestination
		port        int
	}{
		{validate.DirectDestination{User: "deploy", Host: "10.0.0.5", Port: 2222}, 2222},
		{validate.DirectDestination{User: "deploy", Host: "10.0.0.5"}, 2200},
		{validate.DirectDestination{User: "deploy", Host: "2001:db8::1", Port: 22}, 22},
	}
	for _, item := range cases {
		target, err := sshclient.NewDirectTarget(item.destination, resolve, testFacts)
		if err != nil {
			t.Fatal(err)
		}
		if !target.Direct || target.Alias != item.destination.Host || target.HostName != item.destination.Host ||
			target.User != "deploy" || target.Port != strconv.Itoa(item.port) || target.KeepAlive == 0 {
			t.Errorf("NewDirectTarget(%+v) = %+v", item.destination, target)
		}
	}
}

func TestADirectDestinationCarriesAResolverFailure(t *testing.T) {
	refused := errors.New("refused")
	resolve := func(string) (effective.Values, error) { return effective.Values{}, refused }
	_, err := sshclient.NewDirectTarget(validate.DirectDestination{User: "deploy", Host: "web"}, resolve, testFacts)
	if !errors.Is(err, refused) {
		t.Fatalf("err = %v", err)
	}
}
