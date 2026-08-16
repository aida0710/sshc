//go:build unix

package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func awaitOwnership(t *testing.T, events <-chan error) error {
	t.Helper()
	select {
	case cause := <-events:
		return cause
	case <-time.After(5 * time.Second):
		t.Fatal("the ownership monitor reported nothing")
		return nil
	}
}

// **持ち主が手を離したことは、正常な終わりである。**
//
// 実測: 書き手を閉じたパイプは POLLIN と POLLHUP を同時に返す。POLLIN だけを
// 見て中身だと決めると、通常の終了が毎回「規約違反」として報告される。Linux の
// Node が子へ渡す stdin は socket ではなく本物のパイプなので、そこでは毎回そうなる。
func TestOwnershipPipeEndsCleanlyWhenTheWriterCloses(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()

	monitor, err := newOwnershipMonitor(read)
	if err != nil {
		t.Fatalf("newOwnershipMonitor = %v", err)
	}
	events, err := monitor.Start(context.Background())
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	write.Close()

	if cause := awaitOwnership(t, events); !errors.Is(cause, errOwnershipEnded) {
		t.Fatalf("closing the writer reported %v, want errOwnershipEnded", cause)
	}
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Stop = %v", err)
	}
}

// 寿命だけを運ぶチャンネルに中身が来ることは、常に規約違反である。
func TestOwnershipPipeRefusesPayload(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()

	monitor, err := newOwnershipMonitor(read)
	if err != nil {
		t.Fatal(err)
	}
	events, err := monitor.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write.Write([]byte("command")); err != nil {
		t.Fatal(err)
	}
	if cause := awaitOwnership(t, events); !errors.Is(cause, errOwnershipProtocol) {
		t.Fatalf("payload on the ownership channel reported %v, want errOwnershipProtocol", cause)
	}
	_ = monitor.Stop()
}

// 開始より前に閉じていたチャンネルは、ロックを取る前に断られる。
func TestOwnershipRefusesAChannelThatWasAlreadyClosed(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	write.Close()

	monitor, err := newOwnershipMonitor(read)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := monitor.Start(context.Background()); !errors.Is(err, errOwnershipEnded) {
		t.Fatalf("Start on a closed channel = %v, want errOwnershipEnded", err)
	}
}

// Electron が実際に渡すのは、macOS では接続済みの AF_UNIX ストリームである。
// パイプと同じ規則で扱われなければならない。
func TestOwnershipAcceptsAConnectedUnixStreamAndSeesItsEOF(t *testing.T) {
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	local := os.NewFile(uintptr(pair[0]), "ownership")
	peer := os.NewFile(uintptr(pair[1]), "peer")
	defer local.Close()

	monitor, err := newOwnershipMonitor(local)
	if err != nil {
		t.Fatalf("newOwnershipMonitor = %v", err)
	}
	events, err := monitor.Start(context.Background())
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	peer.Close()

	if cause := awaitOwnership(t, events); !errors.Is(cause, errOwnershipEnded) {
		t.Fatalf("closing the peer reported %v, want errOwnershipEnded", cause)
	}
	_ = monitor.Stop()
}

// 端末も通常ファイルも寿命を伝えない。**ロックを取る前に断る。**
func TestOwnershipRefusesDescriptorsThatCarryNoLifetime(t *testing.T) {
	regular, err := os.CreateTemp(t.TempDir(), "ownership")
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()

	for name, reader := range map[string]*os.File{"regular file": regular, "null device": null} {
		t.Run(name, func(t *testing.T) {
			if _, err := newOwnershipMonitor(reader); !errors.Is(err, errOwnershipProtocol) {
				t.Fatalf("newOwnershipMonitor(%s) = %v, want errOwnershipProtocol", name, err)
			}
		})
	}
}
