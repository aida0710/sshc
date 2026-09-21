package app

import (
	"context"
	"crypto/rand"
	"testing"
)

func TestEngineReadsProxyEnvironmentOnlyWhenConnecting(t *testing.T) {
	reads := 0
	services, err := newEngineServices(Dependencies{
		Home: t.TempDir(), Random: rand.Reader,
		Environ: func() []string {
			reads++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reads != 0 {
		t.Fatal("read shell environment while starting the engine")
	}
	if services.ssh.dialer.ProxyEnvironment == nil {
		t.Fatal("engine connections do not load the shell PATH")
	}
	if _, err := services.ssh.dialer.ProxyEnvironment(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("environment reads = %d, want 1", reads)
	}
}

func TestCLIConnectionKeepsItsCallingShellEnvironment(t *testing.T) {
	connection, err := NewCLIConnection(t.TempDir(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if connection.parts.dialer.ProxyEnvironment != nil {
		t.Fatal("CLI connection reloads an environment already supplied by its shell")
	}
}
