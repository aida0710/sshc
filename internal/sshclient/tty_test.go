package sshclient_test

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// fakeProcess は、terminal.Process を満たす最小の相手である。
type fakeProcess struct {
	mutex   sync.Mutex
	written []byte
	sizes   []terminal.Size
	output  *io.PipeReader
	sink    *io.PipeWriter
	exit    terminal.ExitInfo
	done    chan struct{}
	closed  bool
}

func newFakeProcess(exit terminal.ExitInfo) *fakeProcess {
	reader, writer := io.Pipe()
	return &fakeProcess{output: reader, sink: writer, exit: exit, done: make(chan struct{})}
}

func (p *fakeProcess) Read(b []byte) (int, error) { return p.output.Read(b) }

func (p *fakeProcess) Write(b []byte) (int, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.written = append(p.written, b...)
	return len(b), nil
}

func (p *fakeProcess) Resize(size terminal.Size) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.sizes = append(p.sizes, size)
	return nil
}

func (p *fakeProcess) Hangup() error { return p.Close() }

func (p *fakeProcess) Wait() terminal.ExitInfo {
	<-p.done
	return p.exit
}

func (p *fakeProcess) Close() error {
	p.mutex.Lock()
	if p.closed {
		p.mutex.Unlock()
		return nil
	}
	p.closed = true
	p.mutex.Unlock()
	_ = p.sink.Close()
	close(p.done)
	return nil
}

// finish は、リモートが終わったことにする。
func (p *fakeProcess) finish() {
	_ = p.sink.Close()
	p.mutex.Lock()
	if !p.closed {
		p.closed = true
		p.mutex.Unlock()
		close(p.done)
		return
	}
	p.mutex.Unlock()
}

func (p *fakeProcess) recorded() ([]byte, []terminal.Size) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return append([]byte(nil), p.written...), append([]terminal.Size(nil), p.sizes...)
}

// **テレタイプでない入力では raw にしない。大きさも問い合わせない。**
// パイプの中で走っているときがそれで、その場合でも読み書きは通る。
func TestAttachDoesNotChangeAPipe(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()

	process := newFakeProcess(terminal.ExitInfo{Code: 7})
	var output strings.Builder
	done := make(chan int, 1)
	go func() {
		code, err := sshclient.Attach(context.Background(), process, reader, &output)
		if err != nil {
			t.Errorf("Attach = %v", err)
		}
		done <- code
	}()

	if _, err := writer.WriteString("typed\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := process.sink.Write([]byte("from the remote\n")); err != nil {
		t.Fatal(err)
	}
	// 打たれたものが届くのを待ってから終わらせる。終わらせてから見ると、
	// この検査は「まだ写している最中だった」を失敗として報告する。
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if written, _ := process.recorded(); strings.Contains(string(written), "typed") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = writer.Close()
	process.finish()

	select {
	case code := <-done:
		// 終了コードはそのまま伝わる。
		if code != 7 {
			t.Fatalf("exit = %d, want the remote's 7", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Attach never returned")
	}

	if !strings.Contains(output.String(), "from the remote") {
		t.Errorf("output = %q", output.String())
	}
	written, sizes := process.recorded()
	if !strings.Contains(string(written), "typed") {
		t.Errorf("what was typed did not reach the remote: %q", written)
	}
	// 大きさは既定のまま。問い合わせられない相手に問い合わせない。
	if len(sizes) != 1 || sizes[0] != sshclient.DefaultLocalSize {
		t.Errorf("sizes = %#v, want only the default", sizes)
	}
}

// セッションが終われば Attach も終わる。**待ち続けない。**
func TestAttachReturnsWhenTheSessionEnds(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()

	process := newFakeProcess(terminal.ExitInfo{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := sshclient.Attach(context.Background(), process, reader, io.Discard); err != nil {
			t.Errorf("Attach = %v", err)
		}
	}()

	process.finish()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Attach kept waiting after the session ended")
	}
}
