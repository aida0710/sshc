package sshclient_test

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

func TestOSDetectionUsesASeparateAuthenticatedChannel(t *testing.T) {
	path, contents, public := keyPair(t)
	var calls atomic.Int32
	shellStarted := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	server := newTestServer(t, serverOptions{
		AcceptKeys: []ssh.PublicKey{public},
		OnShell: func(channel ssh.Channel) {
			calls.Add(1)
			close(shellStarted)
			<-release
		},
		OnExec: func(channel ssh.Channel) {
			calls.Add(1)
			_, _ = io.WriteString(channel, "Linux\nID=ubuntu\n")
		},
	})
	dialer := dialerFor(t, server, sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }})
	observed := make(chan string, 1)
	dialer.ObserveOS = func(sshclient.Target) func(string) { return func(os string) { observed <- os } }
	process, err := dialer.Open(context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	go func() { _, _ = io.Copy(io.Discard, process) }()
	select {
	case err := <-process.(terminal.Readier).Ready():
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interactive readiness blocked")
	}
	select {
	case os := <-observed:
		if os != "ubuntu" {
			t.Fatalf("OS=%q", os)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("missing OS result")
	}
	// The server acknowledges shell readiness before starting its callback.
	select {
	case <-shellStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("interactive shell callback did not start")
	}
	if calls.Load() != 2 {
		t.Fatalf("channels=%d", calls.Load())
	}
	if server.Command() != "uname -s; cat /etc/os-release 2>/dev/null" {
		t.Fatal("unexpected detection command")
	}
	if !server.ShellRan() {
		t.Fatal("interactive shell was replaced")
	}
}

func TestOSDetectionDoesNotDelayAConnectionWhenProbeStalls(t *testing.T) {
	path, contents, public := keyPair(t)
	release := make(chan struct{})
	defer close(release)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}, OnShell: func(ssh.Channel) { <-release }})
	dialer := dialerFor(t, server, sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }})
	observed := make(chan string, 1)
	dialer.ObserveOS = func(sshclient.Target) func(string) { return func(os string) { observed <- os } }
	process, err := dialer.Open(context.Background(), targetWith(server, path), terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	go func() { _, _ = io.Copy(io.Discard, process) }()
	select {
	case err := <-process.(terminal.Readier).Ready():
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("probe delayed interactive readiness")
	}
	select {
	case os := <-observed:
		t.Fatalf("unexpected result %q", os)
	case <-time.After(3200 * time.Millisecond):
	}
	if _, err := process.Write([]byte("still usable")); err != nil {
		t.Fatal(err)
	}
}
