package sftp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	pkgsftp "github.com/pkg/sftp"
)

func TestOwnershipChangesUseNoFollowExtensionAndRefuseUnprotectedConnections(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		version   string
		protected bool
		allowed   bool
	}{
		{name: "supported", version: "1", protected: true, allowed: true},
		{name: "missing extension", protected: true},
		{name: "unknown extension version", version: "2", protected: true},
		{name: "unprotected client", version: "1"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			serverConnection, clientConnection := net.Pipe()
			requests := make(chan []byte, 1)
			serverErrors := make(chan error, 1)
			go func() {
				defer serverConnection.Close()
				serverErrors <- serveOwnershipFixture(serverConnection, scenario.version, requests)
			}()
			var client *Client
			var err error
			if scenario.protected {
				client, err = newClientPipe(clientConnection, clientConnection)
			} else {
				var raw *pkgsftp.Client
				raw, err = pkgsftp.NewClientPipe(clientConnection, clientConnection)
				client = NewClient(raw)
			}
			if err != nil {
				t.Fatal(err)
			}
			err = client.Chown("/shared/file", 0, ^uint32(0))
			if scenario.allowed && err != nil || !scenario.allowed && !errors.Is(err, ErrUnsupportedOperation) {
				t.Fatalf("ownership change = %v", err)
			}
			_ = client.Close()
			if err := <-serverErrors; err != nil {
				t.Fatal(err)
			}
			if !scenario.allowed {
				select {
				case packet := <-requests:
					t.Fatalf("refused operation sent a mutation: %x", packet)
				default:
				}
				return
			}
			packet := <-requests
			if len(packet) == 0 || packet[0] != sftpExtendedPacket {
				t.Fatalf("ownership used a link-following request: %x", packet)
			}
			request := bytes.NewReader(packet[5:]) // type and request ID
			for _, expected := range []string{noFollowSetstatExtension, "/shared/file"} {
				var length uint32
				if err := binary.Read(request, binary.BigEndian, &length); err != nil {
					t.Fatal(err)
				}
				value := make([]byte, length)
				if _, err := io.ReadFull(request, value); err != nil || string(value) != expected {
					t.Fatalf("request value = %q, %v", value, err)
				}
			}
			var attributes [3]uint32
			if err := binary.Read(request, binary.BigEndian, &attributes); err != nil || attributes != [3]uint32{sftpOwnerAttributes, 0, ^uint32(0)} || request.Len() != 0 {
				t.Fatalf("owner attributes = %v, %v", attributes, err)
			}
		})
	}
}

func serveOwnershipFixture(connection net.Conn, version string, requests chan<- []byte) error {
	if _, err := readOwnershipFixtureFrame(connection); err != nil {
		return err
	}
	hello := new(bytes.Buffer)
	hello.WriteByte(2) // VERSION
	_ = binary.Write(hello, binary.BigEndian, uint32(3))
	if version != "" {
		writeAttributeString(hello, noFollowSetstatExtension)
		writeAttributeString(hello, version)
	}
	if err := writeOwnershipFixtureFrame(connection, hello.Bytes()); err != nil {
		return err
	}
	packet, err := readOwnershipFixtureFrame(connection)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	requests <- packet
	status := make([]byte, 17) // STATUS, echoed request ID, OK, empty message/language
	status[0] = 101
	copy(status[1:5], packet[1:5])
	if err := writeOwnershipFixtureFrame(connection, status); err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, connection)
	return err
}

func readOwnershipFixtureFrame(reader io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	packet := make([]byte, length)
	_, err := io.ReadFull(reader, packet)
	return packet, err
}

func writeOwnershipFixtureFrame(writer io.Writer, packet []byte) error {
	frame := new(bytes.Buffer)
	_ = binary.Write(frame, binary.BigEndian, uint32(len(packet)))
	frame.Write(packet)
	_, err := writer.Write(frame.Bytes())
	return err
}

type ownershipBuffer struct{ bytes.Buffer }

func (*ownershipBuffer) Close() error { return nil }

func TestUploadsPreserveFileBytesThatLookLikeOwnershipRequests(t *testing.T) {
	destination := new(ownershipBuffer)
	writer := &noFollowOwnershipWriter{destination: destination}
	payload := []byte{0, 0, 0, 1, sftpSetstatPacket, 'f', 'i', 'l', 'e'}
	header := make([]byte, 9)
	header[4] = 6 // WRITE, followed by a separate file payload
	binary.BigEndian.PutUint32(header[:4], uint32(len(header)+len(payload)-4))
	for _, segment := range [][]byte{header, payload[:5], payload[5:]} {
		if count, err := writer.Write(segment); err != nil || count != len(segment) {
			t.Fatalf("upload write = %d, %v", count, err)
		}
	}
	if !bytes.Equal(destination.Bytes(), append(header, payload...)) {
		t.Fatal("file payload was interpreted as a metadata request")
	}
}

func TestOpenSSHOwnershipChangesNeverFollowAReplacedLink(t *testing.T) {
	serverPath := ""
	for _, candidate := range []string{"/usr/lib/openssh/sftp-server", "/usr/libexec/sftp-server"} {
		if _, err := os.Stat(candidate); err == nil {
			serverPath = candidate
			break
		}
	}
	if serverPath == "" {
		t.Skip("OpenSSH sftp-server is not installed")
	}
	server := exec.CommandContext(t.Context(), serverPath)
	reader, err := server.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	writer, err := server.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	client, err := newClientPipe(reader, writer)
	if err != nil {
		_ = writer.Close()
		_ = server.Wait()
		t.Fatal(err)
	}
	defer func() {
		_ = client.Close()
		if err := server.Wait(); err != nil {
			t.Errorf("OpenSSH fixture exited: %v", err)
		}
	}()
	directory := t.TempDir()
	filePath := filepath.Join(directory, "file")
	if err := os.WriteFile(filePath, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{filePath, directory} {
		info, err := client.Lstat(candidate)
		if err != nil {
			t.Fatal(err)
		}
		owner, available := ownershipFrom(info)
		if !available {
			t.Fatal("OpenSSH did not report owner IDs")
		}
		if err := client.Chown(candidate, owner.UID, owner.GID); err != nil {
			t.Fatal(err)
		}
	}
	targetBefore, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(directory, "replaced")
	if err := os.Symlink(filePath, linkPath); err != nil {
		t.Fatal(err)
	}
	linkInfo, err := client.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	owner, _ := ownershipFrom(linkInfo)
	if err := client.Chown(linkPath, owner.UID, owner.GID); err != nil {
		t.Fatal(err)
	}
	targetAfter, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	// Even chown to the existing owner changes ctime. Comparing native stat
	// proves that the real server did not apply the request to the link target.
	if !reflect.DeepEqual(targetBefore.Sys(), targetAfter.Sys()) {
		t.Fatal("OpenSSH changed the link target's metadata")
	}
}
