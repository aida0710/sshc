package sshclient

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
)

func TestDirectConnectionsDoNotReadTheProxyShellEnvironment(t *testing.T) {
	dialed := errors.New("direct dial")
	dialer := Dialer{
		Dial: func(context.Context, string, string) (net.Conn, error) { return nil, dialed },
		ProxyEnvironment: func(context.Context) ([]string, error) {
			t.Fatal("started a shell for a direct connection")
			return nil, nil
		},
	}
	_, err := dialer.open(context.Background(), Target{HostName: "unused", Port: "22"}, nil, newTracer(Quiet, io.Discard))
	if !errors.Is(err, dialed) {
		t.Fatalf("direct connection = %v", err)
	}
}

func TestConnectionFailureMessageTranslatesCommonContextErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "timeout", err: context.DeadlineExceeded, want: "SSHハンドシェイクがタイムアウトしました。"},
		{name: "cancelled", err: context.Canceled, want: "SSHハンドシェイクをキャンセルしました。"},
		{name: "detail", err: errors.New("connection reset"), want: "SSHハンドシェイクに失敗しました。"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := connectionFailureMessage("SSHハンドシェイク", test.err); got != test.want {
				t.Fatalf("connectionFailureMessage() = %q, want %q", got, test.want)
			}
		})
	}
}
