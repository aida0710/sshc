//go:build windows

package windowspipe

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/sys/windows"
)

// pipeServer は、テストのために名前つきパイプを一本だけ提供する。
//
// **本物のエージェントを当てにしない。** CI の Windows に OpenSSH エージェント
// が走っている保証は無く、走っていたとしても、そこへ鍵を登録するテストを書く
// わけにはいかない。確かめたいのは運ぶ管の方である。
type pipeServer struct {
	name     string
	listener windows.Handle
	once     sync.Once
}

func newPipeServer(t *testing.T) *pipeServer {
	t.Helper()
	// 名前はテストごとに変える。同じ名前を二つのテストが同時に使うと、
	// どちらの client がどちらへ繋がったか分からなくなる。
	name := fmt.Sprintf(`\\.\pipe\sshc-test-%d-%s`, os.Getpid(), strings.ReplaceAll(t.Name(), "/", "-"))
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := windows.CreateNamedPipe(
		wide,
		windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1, maxPipeMessage, maxPipeMessage, 0, nil,
	)
	if err != nil {
		t.Fatalf("create named pipe: %v", err)
	}
	server := &pipeServer{name: name, listener: listener}
	t.Cleanup(server.close)
	return server
}

// accept は、client が繋いでくるのを待って、その側の net.Conn を返す。
//
// **t.Fatal を使わない。** これは別の goroutine から呼ばれるもので、そこでの
// Fatal は呼んだ goroutine を終わらせるだけであり、テストは止まらない。
// 理由は値として返し、テストの goroutine が受け取ってから落とす。
func (server *pipeServer) accept() (net.Conn, error) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(event) }()
	overlapped := &windows.Overlapped{HEvent: event}

	err = windows.ConnectNamedPipe(server.listener, overlapped)
	switch {
	case errors.Is(err, windows.ERROR_PIPE_CONNECTED):
		// 待つ前に繋がっていた。
	case errors.Is(err, windows.ERROR_IO_PENDING):
		waited, waitErr := windows.WaitForSingleObject(event, 10_000)
		if waitErr != nil {
			return nil, waitErr
		}
		if waited != windows.WAIT_OBJECT_0 {
			return nil, fmt.Errorf("wait for connection ended in state %d", waited)
		}
	case err != nil:
		return nil, err
	}

	// server 側も同じ conn 型で包む。**両側で同じものを使う**ので、テストが
	// 通ったことは、この型が読み書きの両方向で動いたということになる。
	side, err := newConn(server.listener, server.name)
	if err != nil {
		return nil, err
	}
	server.listener = 0
	return side, nil
}

// accepting は、accept を別の goroutine で走らせ、その結果を待てるようにする。
func (server *pipeServer) accepting() <-chan acceptResult {
	results := make(chan acceptResult, 1)
	go func() {
		side, err := server.accept()
		results <- acceptResult{conn: side, err: err}
	}()
	return results
}

type acceptResult struct {
	conn net.Conn
	err  error
}

// serverSide は、繋がってきた側を受け取る。**テストの goroutine で落とす。**
func serverSide(t *testing.T, results <-chan acceptResult) net.Conn {
	t.Helper()
	select {
	case result := <-results:
		if result.err != nil {
			t.Fatalf("accept: %v", result.err)
		}
		t.Cleanup(func() { _ = result.conn.Close() })
		return result.conn
	case <-time.After(10 * time.Second):
		t.Fatal("no client ever connected")
		return nil
	}
}

func (server *pipeServer) close() {
	server.once.Do(func() {
		if server.listener != 0 {
			_ = windows.CloseHandle(server.listener)
			server.listener = 0
		}
	})
}

func dial(t *testing.T, server *pipeServer) net.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := DialContext(ctx, server.name)
	if err != nil {
		t.Fatalf("dial %s: %v", server.name, err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// **運ぶのは、渡したものそのままである。** 途中で切れても混ざってもいけない。
func TestThePipeCarriesBytesInBothDirections(t *testing.T) {
	server := newPipeServer(t)
	accepting := server.accepting()
	client := dial(t, server)
	side := serverSide(t, accepting)

	message := []byte("the agent protocol is just bytes")
	if _, err := client.Write(message); err != nil {
		t.Fatalf("write: %v", err)
	}
	received := make([]byte, len(message))
	if _, err := io.ReadFull(side, received); err != nil {
		t.Fatalf("read on the server side: %v", err)
	}
	if string(received) != string(message) {
		t.Errorf("server read %q, want %q", received, message)
	}

	reply := []byte("and it answers on the same handle")
	if _, err := side.Write(reply); err != nil {
		t.Fatalf("write on the server side: %v", err)
	}
	answered := make([]byte, len(reply))
	if _, err := io.ReadFull(client, answered); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(answered) != string(reply) {
		t.Errorf("client read %q, want %q", answered, reply)
	}
}

// **答えない相手は、締切で手を離す。** 解除する手段が無ければ、エージェントが
// 固まった日にこのアプリケーション全体がそこで止まる。
func TestAReadPastItsDeadlineIsReleased(t *testing.T) {
	server := newPipeServer(t)
	accepting := server.accepting()
	client := dial(t, server)
	side := serverSide(t, accepting)

	if err := client.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err := client.Read(make([]byte, 16))

	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("read error = %v, want a deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("the read took %s to give up", elapsed)
	}
	// 取り下げたあとも handle は使える。締切は接続を壊すものではない。
	if err := client.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := side.Write([]byte("still here")); err != nil {
		t.Fatalf("write on the server side: %v", err)
	}
	buffer := make([]byte, len("still here"))
	if _, err := io.ReadFull(client, buffer); err != nil {
		t.Fatalf("read after the deadline: %v", err)
	}
}

// **閉じることは、待っている読みを解くことでもある。** 解かずに handle を
// 閉じれば、kernel が触っている最中のものを無効にすることになる。
func TestClosingReleasesAPendingRead(t *testing.T) {
	server := newPipeServer(t)
	accepting := server.accepting()
	client := dial(t, server)
	// 繋がってきた側は、書き込まないまま持っておく。**そのおかげで client の
	// 読みは待ち続ける**——確かめたいのは、その待ちを閉鎖が解くことである。
	serverSide(t, accepting)

	failed := make(chan error, 1)
	go func() {
		_, err := client.Read(make([]byte, 16))
		failed <- err
	}()
	// 読みが実際に待ち始めるまでの間を置く。閉じるのが先だと、確かめたい
	// 「待っている最中の閉鎖」ではなく「閉じたあとの読み」になる。
	time.Sleep(200 * time.Millisecond)
	if err := client.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case err := <-failed:
		if err == nil {
			t.Fatal("the pending read returned without an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("closing did not release the pending read")
	}
}

// **繋げない相手は、繋げないと言う。** 存在しない名前をいつまでも待つと、
// エージェントが居ない機械で鍵の一覧が固まる。
func TestDialingAPipeThatIsNotThereFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := DialContext(ctx, fmt.Sprintf(`\\.\pipe\sshc-absent-%d`, os.Getpid()))

	if err == nil {
		_ = conn.Close()
		t.Fatal("dialling a pipe that does not exist succeeded")
	}
}

// 宛先の表示に資格情報は現れない。**エラーもログもこれを含む。**
func TestTheAddressIsTheFixedPipeNameAndNothingElse(t *testing.T) {
	server := newPipeServer(t)
	accepting := server.accepting()
	client := dial(t, server)
	serverSide(t, accepting)

	for _, address := range []net.Addr{client.LocalAddr(), client.RemoteAddr()} {
		if address.Network() != "pipe" {
			t.Errorf("Network() = %q, want pipe", address.Network())
		}
		if address.String() != server.name {
			t.Errorf("String() = %q, want %q", address.String(), server.name)
		}
	}
}

// **運べればよい、では足りない。** この管の上を通るのは agent のプロトコルで
// あり、その要求と応答の枠が保たれることまで確かめて初めて、鍵の一覧が
// Windows で読めると言える。
func TestTheAgentProtocolSurvivesThisTransport(t *testing.T) {
	server := newPipeServer(t)
	accepting := server.accepting()
	client := dial(t, server)
	side := serverSide(t, accepting)

	keyring := agent.NewKeyring()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const comment = "carried-over-the-pipe"
	if err := keyring.Add(agent.AddedKey{PrivateKey: &private, Comment: comment}); err != nil {
		t.Fatalf("add to the keyring: %v", err)
	}
	go func() { _ = agent.ServeAgent(keyring, side) }()

	identities, err := agent.NewClient(client).List()

	if err != nil {
		t.Fatalf("list over the pipe: %v", err)
	}
	if len(identities) != 1 {
		t.Fatalf("identities = %d, want 1", len(identities))
	}
	if identities[0].Comment != comment {
		t.Errorf("comment = %q, want %q", identities[0].Comment, comment)
	}
	if _, err := ssh.ParsePublicKey(identities[0].Blob); err != nil {
		t.Errorf("the identity blob did not survive the transport: %v", err)
	}
}
