package sshclient

import (
	"bytes"
	"strings"
	"testing"
)

func TestAShellOutputWatchGetsWhatTheShellWroteBeforeItAndThenEachPieceInOrder(t *testing.T) {
	var output shellOutput
	var terminal bytes.Buffer
	shell := output.tee(&terminal)
	if _, err := shell.Write([]byte("Welcome\r\n$ ")); err != nil {
		t.Fatal(err)
	}

	var seen []string
	stop := output.watch(func(p []byte) { seen = append(seen, string(p)) })
	if _, err := shell.Write([]byte("ls\r\n")); err != nil {
		t.Fatal(err)
	}
	stop()
	if _, err := shell.Write([]byte("after stop")); err != nil {
		t.Fatal(err)
	}

	if want := []string{"Welcome\r\n$ ", "ls\r\n"}; strings.Join(seen, "|") != strings.Join(want, "|") {
		t.Fatalf("seen = %q, want %q", seen, want)
	}
	if got := terminal.String(); got != "Welcome\r\n$ ls\r\nafter stop" {
		t.Fatalf("terminal got %q, want everything the shell wrote", got)
	}
}

func TestAShellOutputWatchGetsOnlyTheEndOfALongBacklog(t *testing.T) {
	var output shellOutput
	shell := output.tee(&bytes.Buffer{})
	if _, err := shell.Write(bytes.Repeat([]byte("x"), shellOutputBacklog)); err != nil {
		t.Fatal(err)
	}
	if _, err := shell.Write([]byte("$ ")); err != nil {
		t.Fatal(err)
	}
	var seen []byte
	output.watch(func(p []byte) { seen = append(seen, p...) })
	if len(seen) != shellOutputBacklog || !bytes.HasSuffix(seen, []byte("$ ")) {
		t.Fatalf("backlog of %d bytes ending %q, want the last %d bytes", len(seen), seen[max(0, len(seen)-4):], shellOutputBacklog)
	}
}
