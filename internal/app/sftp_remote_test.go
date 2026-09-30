package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pkgsftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	sshcSFTP "sshc/internal/sftp"
)

// sftpSubsystemServer は、loopbackでSFTP subsystemを提供するSSHサーバーである。
// SSH transportを残したまま、開いたSFTP subsystemだけを終えられる。
type sftpSubsystemServer struct {
	address string

	mutex    sync.Mutex
	channels []ssh.Channel
	opened   chan struct{}
}

func startSFTPSubsystemServer(t *testing.T) *sftpSubsystemServer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := &sftpSubsystemServer{address: listener.Addr().String(), opened: make(chan struct{}, 8)}
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = connection.Close() })
			go server.serve(connection, config)
		}
	}()
	return server
}

func (server *sftpSubsystemServer) serve(connection net.Conn, config *ssh.ServerConfig) {
	_, channels, requests, err := ssh.NewServerConn(connection, config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)
	for newChannel := range channels {
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go func() {
			for request := range channelRequests {
				if request.Type != "subsystem" {
					_ = request.Reply(false, nil)
					continue
				}
				_ = request.Reply(true, nil)
				sftpServer, err := pkgsftp.NewServer(channel)
				if err != nil {
					_ = channel.Close()
					continue
				}
				go func() { _ = sftpServer.Serve() }()
				server.mutex.Lock()
				server.channels = append(server.channels, channel)
				server.mutex.Unlock()
				server.opened <- struct{}{}
			}
		}()
	}
}

// endSubsystem は、sftp-serverが終了したときと同じくexit-statusを送ってから
// SFTPのchannelだけを閉じる。SSH transportは開いたままである。
func (server *sftpSubsystemServer) endSubsystem(t *testing.T, index int) {
	t.Helper()
	server.mutex.Lock()
	channel := server.channels[index]
	server.mutex.Unlock()
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{1}))
	if err := channel.Close(); err != nil {
		t.Fatal(err)
	}
}

func (server *sftpSubsystemServer) open(t *testing.T) *sftpRemote {
	t.Helper()
	transport, err := ssh.Dial("tcp4", server.address, &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := pkgsftp.NewClient(transport)
	if err != nil {
		_ = transport.Close()
		t.Fatal(err)
	}
	select {
	case <-server.opened:
	case <-time.After(5 * time.Second):
		t.Fatal("the server did not register the SFTP subsystem")
	}
	return newSFTPRemote(session, transport)
}

// remotePathOf は、このテストのSFTPサーバー（pkg/sftpのServer）でローカルの local を指す
// リモートのパスを返す。sshcはリモートのパスを絶対POSIX表記でしか受け付けない。
// pkg/sftpのServerはWindowsで "/C:/Users/..." を "C:\Users\..." として開く。
func remotePathOf(local string) string {
	slashed := filepath.ToSlash(local)
	if strings.HasPrefix(slashed, "/") {
		return slashed
	}
	return "/" + slashed
}

// SSH transportを残したままSFTP subsystemだけが終わった接続は、poolが次の操作へ
// 貸さず、新しい接続を開く。貸し続けると、利用者が再試行するたびにconnection lostになる。
func TestSFTPPoolOpensANewConnectionWhenOnlyTheSubsystemEnded(t *testing.T) {
	server := startSFTPSubsystemServer(t)
	var (
		mutex   sync.Mutex
		remotes []*sftpRemote
	)
	pool := sshcSFTP.NewRemotePool(func(context.Context, string) (sshcSFTP.RemoteTarget, error) {
		return sshcSFTP.RemoteTarget{Identity: "fixed", Open: func(context.Context) (sshcSFTP.Remote, error) {
			remote := server.open(t)
			mutex.Lock()
			remotes = append(remotes, remote)
			mutex.Unlock()
			return remote, nil
		}}, nil
	})
	t.Cleanup(func() { _ = pool.Close() })
	service := sshcSFTP.Service{Open: pool.Open}
	ctx := context.Background()
	directory := remotePathOf(t.TempDir())

	if _, err := service.Stat(ctx, "host", directory); err != nil {
		t.Fatalf("first Stat = %v", err)
	}
	server.endSubsystem(t, 0)
	mutex.Lock()
	first := remotes[0]
	mutex.Unlock()
	select {
	case <-first.dead:
	case <-time.After(5 * time.Second):
		t.Fatal("the remote still reports itself alive after its SFTP subsystem ended")
	}

	if _, err := service.Stat(ctx, "host", directory); err != nil {
		t.Fatalf("Stat after the subsystem ended = %v, want it to succeed on a new connection", err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(remotes) != 2 {
		t.Fatalf("connections opened = %d, want 2", len(remotes))
	}
}
