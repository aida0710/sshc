package sftp

import (
	"bytes"
	"encoding/binary"
	"os"
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
	client := openOpenSSHTestClient(t)
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
