package sftp

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOwnershipChangesUseNoFollowExtensionAndRefuseUnprotectedConnections(t *testing.T) {
	testNoFollowAttributeChange(t, func(client *Client) error { return client.Chown("/shared/file", 0, ^uint32(0)) },
		[]uint32{sftpOwnerAttributes, 0, ^uint32(0)})
}

func TestUploadsPreserveFileBytesThatLookLikeOwnershipRequests(t *testing.T) {
	destination := new(ownershipBuffer)
	writer := &noFollowAttributesWriter{destination: destination}
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
