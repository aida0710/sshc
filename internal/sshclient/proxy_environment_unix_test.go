//go:build !windows

package sshclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestProxyCommandFindsBothAwsAndItsPluginInTheSuppliedPath(t *testing.T) {
	directory := t.TempDir()
	for name, script := range map[string]string{
		"aws":                    "exec session-manager-plugin\n",
		"session-manager-plugin": "printf '%s' \"$AWS_PROFILE\"\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("#!/bin/sh\n"+script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	dialer := Dialer{ProxyEnvironment: func(context.Context) ([]string, error) {
		return []string{"PATH=" + directory, "AWS_PROFILE=kept-profile"}, nil
	}}
	connection, err := dialer.open(context.Background(), Target{ProxyCommand: "aws"}, nil, newTracer(Quiet, io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	output, err := io.ReadAll(connection)
	if err != nil || string(output) != "kept-profile" {
		t.Fatalf("ProxyCommand = %q, %v", output, err)
	}
}

func TestProxyCommandUsesInheritedEnvironmentWhenPathLoadingFails(t *testing.T) {
	var log bytes.Buffer
	dialer := Dialer{ProxyEnvironment: func(context.Context) ([]string, error) {
		return []string{"PATH=/usr/bin:/bin", "SSHC_TEST_VALUE=inherited"}, errors.New("shell failed")
	}}
	connection, err := dialer.open(context.Background(), Target{ProxyCommand: `printf '%s' "$SSHC_TEST_VALUE"`}, nil, newTracer(Quiet, &log))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	output, err := io.ReadAll(connection)
	if err != nil || string(output) != "inherited" || !bytes.Contains(log.Bytes(), []byte("shell failed")) {
		t.Fatalf("fallback = %q, %v; log = %q", output, err, log.String())
	}
}

func TestCancellingPathLoadingDoesNotStartTheProxyCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dialer := Dialer{ProxyEnvironment: func(context.Context) ([]string, error) {
		cancel()
		return nil, ctx.Err()
	}}
	connection, err := dialer.open(ctx, Target{ProxyCommand: "sleep 60"}, nil, newTracer(Quiet, io.Discard))
	if connection != nil {
		_ = connection.Close()
		t.Fatal("started a ProxyCommand after cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("open = %v", err)
	}
}
