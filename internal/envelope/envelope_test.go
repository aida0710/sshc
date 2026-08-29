package envelope_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/envelope"
)

// Seal は鍵だけで動き、開く操作はその鏡像である。すでに鍵を保持している
// 呼び出し側（鍵を保持し、パスフレーズは意図的に保持しない vault）は、自分の
// ファイルの隣にもうひとつ暗号化し、ユーザーに再度尋ねることなく読み戻すことが
// できる。
func TestAKeyOpensWhatItSealed(t *testing.T) {
	key, err := envelope.Derive("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := key.Seal([]byte("the object store settings"))
	if err != nil {
		t.Fatal(err)
	}

	plaintext, err := key.Open(sealed)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	if string(plaintext) != "the object store settings" {
		t.Errorf("plaintext = %q", plaintext)
	}
}

func TestAnotherKeyCannotOpenIt(t *testing.T) {
	mine, err := envelope.Derive("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := envelope.Derive("a different master password")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := mine.Seal([]byte("the object store settings"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := theirs.Open(sealed); !errors.Is(err, envelope.ErrWrongPassphrase) {
		t.Errorf("Open with another key = %v, want ErrWrongPassphrase", err)
	}
}

func TestDestroyClearsOnlyTheOwnedKeyCopy(t *testing.T) {
	key, err := envelope.Derive("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	copy := key.Clone()
	copy.Destroy()
	if _, err := copy.Seal([]byte("discarded")); !errors.Is(err, envelope.ErrNotAnEnvelope) {
		t.Fatalf("destroyed clone Seal = %v, want ErrNotAnEnvelope", err)
	}
	if _, err := key.Seal([]byte("still live")); err != nil {
		t.Fatalf("destroying a clone erased the live key: %v", err)
	}
	key.Destroy()
	if _, err := key.Seal([]byte("discarded")); !errors.Is(err, envelope.ErrNotAnEnvelope) {
		t.Fatalf("destroyed key Seal = %v, want ErrNotAnEnvelope", err)
	}
}

func TestAKeyRefusesTamperedBytes(t *testing.T) {
	key, err := envelope.Derive("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := key.Seal([]byte("the object store settings"))
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 0xff

	if _, err := key.Open(sealed); err == nil {
		t.Error("a flipped bit was accepted")
	}
}

func TestAKeyRejectsBothOlderAndNewerEnvelopeVersions(t *testing.T) {
	key, err := envelope.Derive("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := key.Seal([]byte("current format"))
	if err != nil {
		t.Fatal(err)
	}
	for name, version := range map[string]byte{"older": 0, "newer": 2} {
		candidate := append([]byte(nil), sealed...)
		candidate[16] = version
		if _, err := key.Open(candidate); !errors.Is(err, envelope.ErrUnsupportedVersion) {
			t.Fatalf("%s version error = %v, want ErrUnsupportedVersion", name, err)
		}
	}
}

// 鍵の導出は意図的に高価なので、同時に何個走れるかは呼び出し側ではなくこちらが
// 決める数字である。
//
// アンロックも push も pull も導出を行い、三つとも普通のリクエストである。放って
// おけば、タブがいくつか開いたページやスクリプトが、64 MiB ずつのものを何十個も
// 要求しうる。単一ユーザーが操作するローカルインターフェースでは、この制限による
// 待ち時間はごく短い。一方、回避できるメモリ確保はギガバイト単位になる。
func TestDerivationsDoNotAllRunAtOnce(t *testing.T) {
	const attempts = 8
	var running, peak int64
	var mutex sync.Mutex
	var group sync.WaitGroup

	envelope.OnDerive = func(step func()) {
		mutex.Lock()
		running++
		if running > peak {
			peak = running
		}
		mutex.Unlock()
		step()
		mutex.Lock()
		running--
		mutex.Unlock()
	}
	t.Cleanup(func() { envelope.OnDerive = nil })

	for range attempts {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := envelope.Derive("a passphrase long enough"); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if peak > envelope.MaxConcurrentDerivations {
		t.Errorf("%d derivations ran at once, want at most %d", peak, envelope.MaxConcurrentDerivations)
	}
}

// ネットワーク越しに届いた envelope には、このインストールが書いたものより厳しい
// 上限を課す。その中のパラメータを選んだのはそれを書いた誰かであり、コストを
// 払うのは開くときだからである。
func TestARemoteEnvelopeMayNotAskForWhatALocalOneMay(t *testing.T) {
	// このインストールがローカルで受け入れるパラメータで暗号化したもの。
	key, err := envelope.Derive("a passphrase long enough")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := key.Seal([]byte("a snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := envelope.OpenWithin(sealed, "a passphrase long enough", envelope.AcceptedFromRemote); err != nil {
		t.Fatalf("what Derive writes must open under the remote ceiling: %v", err)
	}

	// Derive が書く値を下回る上限は、コストを払わずに拒否する。
	tiny := envelope.Limits{Time: 1, MemoryKiB: 1024, Threads: 1}
	if _, _, err := envelope.OpenWithin(sealed, "a passphrase long enough", tiny); !errors.Is(err, envelope.ErrCostRefused) {
		t.Errorf("OpenWithin under a tiny ceiling = %v, want ErrCostRefused", err)
	}
}

func TestRemoteDerivationsAreSerializedWithoutBlockingALocalDerivation(t *testing.T) {
	key, err := envelope.Derive("a passphrase long enough")
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := key.Seal([]byte("a snapshot"))
	if err != nil {
		t.Fatal(err)
	}

	firstStarted := make(chan struct{})
	localStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int64
	envelope.OnDerive = func(step func()) {
		switch calls.Add(1) {
		case 1:
			close(firstStarted)
			<-releaseFirst
		case 2:
			close(localStarted)
		}
		step()
	}
	t.Cleanup(func() { envelope.OnDerive = nil })

	open := func(done chan<- error) {
		_, _, err := envelope.OpenWithin(sealed, "a passphrase long enough", envelope.AcceptedFromRemote)
		done <- err
	}
	remoteDone := make(chan error, 2)
	go open(remoteDone)
	<-firstStarted
	go open(remoteDone)

	localDone := make(chan error, 1)
	go func() {
		_, err := envelope.Derive("another passphrase long enough")
		localDone <- err
	}()
	select {
	case <-localStarted:
	case <-time.After(2 * time.Second):
		close(releaseFirst)
		t.Fatal("a remote derivation blocked an independent local derivation")
	}
	if got := calls.Load(); got != 2 {
		close(releaseFirst)
		t.Fatalf("%d derivations started while one remote derivation was blocked, want 2", got)
	}
	close(releaseFirst)
	for range 2 {
		if err := <-remoteDone; err != nil {
			t.Fatal(err)
		}
	}
	if err := <-localDone; err != nil {
		t.Fatal(err)
	}
}
