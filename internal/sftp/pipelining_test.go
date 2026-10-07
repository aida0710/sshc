package sftp_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pkgsftp "github.com/pkg/sftp"

	"sshc/internal/sftp"
)

// SFTP packet types this test follows (draft-ietf-secsh-filexfer-02).
const (
	sftpPacketRead   = 5
	sftpPacketWrite  = 6
	sftpPacketStatus = 101
	sftpPacketData   = 103
)

// responseLatency holds back every answer from the server, so a client that
// waits for each answer before the next request never has two outstanding.
const responseLatency = 100 * time.Microsecond

// outstandingRequests records the most READ and WRITE requests that were
// waiting for an answer at the same moment.
type outstandingRequests struct {
	mutex   sync.Mutex
	pending map[uint32]bool
	most    int
}

func (o *outstandingRequests) sent(kind byte, id uint32) {
	if kind != sftpPacketRead && kind != sftpPacketWrite {
		return
	}
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.pending[id] = true
	o.most = max(o.most, len(o.pending))
}

func (o *outstandingRequests) answered(kind byte, id uint32) {
	if kind != sftpPacketStatus && kind != sftpPacketData {
		return
	}
	time.Sleep(responseLatency)
	o.mutex.Lock()
	defer o.mutex.Unlock()
	delete(o.pending, id)
}

func (o *outstandingRequests) mostAtOnce() int {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	return o.most
}

// relayPackets forwards SFTP packets and reports each one's type and request
// id before forwarding it.
func relayPackets(destination io.WriteCloser, source io.Reader, observe func(kind byte, id uint32)) {
	defer destination.Close()
	for {
		var length [4]byte
		if _, err := io.ReadFull(source, length[:]); err != nil {
			return
		}
		packet := make([]byte, binary.BigEndian.Uint32(length[:]))
		if _, err := io.ReadFull(source, packet); err != nil {
			return
		}
		if len(packet) >= 5 {
			observe(packet[0], binary.BigEndian.Uint32(packet[1:5]))
		}
		if _, err := destination.Write(append(length[:], packet...)); err != nil {
			return
		}
	}
}

type serverPipe struct {
	io.Reader
	io.WriteCloser
}

// observedRemote connects a pkg/sftp client, configured as the engine does,
// to a pkg/sftp server on this machine's filesystem through the relay.
func observedRemote(t *testing.T, requests *outstandingRequests) sftp.Remote {
	t.Helper()
	fromClient, toRelay := io.Pipe()
	fromRelay, toServer := io.Pipe()
	fromServer, toRelayBack := io.Pipe()
	fromRelayBack, toClient := io.Pipe()
	server, err := pkgsftp.NewServer(serverPipe{Reader: fromRelay, WriteCloser: toRelayBack})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		// Serve returns when the client hangs up, but leaves the pipe back
		// to the client open until Close.
		_ = server.Serve()
		_ = server.Close()
	}()
	go relayPackets(toServer, fromClient, requests.sent)
	go relayPackets(toClient, fromServer, requests.answered)
	client, err := sftp.NewClientPipeForTest(fromRelayBack, toRelay, pkgsftp.UseConcurrentWrites(true))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// localPath names a file of the engine's file system in a transfer request.
func localPath(directory, name string) string {
	return filepath.ToSlash(filepath.Join(directory, name))
}

// servedPath names the same file as the remote path under which the pkg/sftp
// server of observedRemote serves it. A remote path is always an absolute
// POSIX path, so a Windows drive follows a leading slash, as in /C:/Users; the
// server maps that back to C:\Users.
func servedPath(directory, name string) string {
	slashed := localPath(directory, name)
	if strings.HasPrefix(slashed, "/") {
		return slashed
	}
	return "/" + slashed
}

func TestTransfersKeepSeveralSFTPRequestsInFlight(t *testing.T) {
	contents := bytes.Repeat([]byte("pipelined bytes\n"), (4<<20)/16)
	for _, test := range []struct {
		name string
		// target is the file beside local.bin that the copy writes.
		target  string
		request func(directory, target string) sftp.RemoteTransferRequest
		copy    func(sftp.Service, context.Context, sftp.RemoteTransferRequest) error
	}{
		{
			name:   "put",
			target: "uploaded.bin",
			request: func(directory, target string) sftp.RemoteTransferRequest {
				return sftp.RemoteTransferRequest{
					SourcePath: localPath(directory, "local.bin"), TargetAlias: "edge",
					TargetPath: servedPath(directory, target), Operation: sftp.RemotePut,
				}
			},
			copy: copyLocal,
		},
		{
			name:   "get",
			target: "downloaded.bin",
			request: func(directory, target string) sftp.RemoteTransferRequest {
				return sftp.RemoteTransferRequest{
					SourceAlias: "edge", SourcePath: servedPath(directory, "local.bin"),
					TargetPath: localPath(directory, target), Operation: sftp.RemoteGet,
				}
			},
			copy: copyLocal,
		},
		{
			name:   "copy within a host",
			target: "copied.bin",
			request: func(directory, target string) sftp.RemoteTransferRequest {
				return sftp.RemoteTransferRequest{
					SourceAlias: "edge", SourcePath: servedPath(directory, "local.bin"),
					TargetAlias: "edge", TargetPath: servedPath(directory, target),
					Operation: sftp.RemoteCopy,
				}
			},
			copy: copyRemote,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			t.Setenv("HOME", directory)
			t.Setenv("USERPROFILE", directory)
			if err := os.WriteFile(filepath.Join(directory, "local.bin"), contents, 0o600); err != nil {
				t.Fatal(err)
			}
			requests := &outstandingRequests{pending: make(map[uint32]bool)}
			service := sftp.Service{Open: func(context.Context, string) (sftp.Remote, error) {
				return observedRemote(t, requests), nil
			}}

			if err := test.copy(service, t.Context(), test.request(directory, test.target)); err != nil {
				t.Fatal(err)
			}
			copied, err := os.ReadFile(filepath.Join(directory, test.target))
			if err != nil || !bytes.Equal(copied, contents) {
				t.Fatalf("copied %d bytes, %v; want %d identical bytes", len(copied), err, len(contents))
			}
			if most := requests.mostAtOnce(); most < 2 {
				t.Fatalf("at most %d READ or WRITE request in flight, want the transfer pipelined", most)
			}
		})
	}
}

func copyLocal(service sftp.Service, ctx context.Context, request sftp.RemoteTransferRequest) error {
	return service.CopyLocal(ctx, request, func(int64) error { return nil })
}

func copyRemote(service sftp.Service, ctx context.Context, request sftp.RemoteTransferRequest) error {
	return service.CopyRemote(ctx, request, func(int64) error { return nil })
}
