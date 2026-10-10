//go:build linux

package sftp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func supplementaryLocalTextGroup(t *testing.T) int {
	t.Helper()
	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal(err)
	}
	for _, gid := range groups {
		if gid != os.Getegid() {
			return gid
		}
	}
	t.Skip("fixture requires a supplementary group")
	return 0
}

type localTextFixtureAttribute struct {
	name  string
	value []byte
}

func setLocalTextFixtureAttribute(t *testing.T, filename string, attribute localTextFixtureAttribute) {
	t.Helper()
	if err := unix.Setxattr(filename, attribute.name, attribute.value, 0); err != nil {
		if errors.Is(err, unix.ENOTSUP) {
			t.Skip("fixture filesystem does not support extended attributes")
		}
		t.Fatal(err)
	}
}

func readLocalTextFixtureAttribute(t *testing.T, filename, name string) []byte {
	t.Helper()
	size, err := unix.Getxattr(filename, name, nil)
	if err != nil {
		t.Fatal(err)
	}
	value := make([]byte, size)
	read, err := unix.Getxattr(filename, name, value)
	if err != nil || read != size {
		t.Fatalf("saved attribute %s has %d bytes, %v; want %d", name, read, err, size)
	}
	return value
}

func localTextFixtureACL(uid uint32) []byte {
	// Linux POSIX ACL xattrs use a version followed by tag/permissions/ID
	// entries. A named user makes this an extended ACL, beyond mode bits.
	const (
		version       = 2
		undefinedID   = ^uint32(0)
		userObjectTag = 1
		userTag       = 2
		groupTag      = 4
		maskTag       = 16
		otherTag      = 32
		read          = 4
		write         = 2
	)
	entries := []struct {
		tag         uint16
		permissions uint16
		id          uint32
	}{{userObjectTag, read | write, undefinedID}, {userTag, read, uid}, {groupTag, read, undefinedID}, {maskTag, read, undefinedID}, {otherTag, 0, undefinedID}}
	var encoded bytes.Buffer
	_ = binary.Write(&encoded, binary.LittleEndian, uint32(version))
	for _, entry := range entries {
		_ = binary.Write(&encoded, binary.LittleEndian, entry)
	}
	return encoded.Bytes()
}

func TestLocalTextSaveKeepsTheOriginalSupplementaryGroup(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	gid := supplementaryLocalTextGroup(t)
	if err := os.Chown(filename, -1, gid); err != nil {
		t.Fatal(err)
	}
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "after", ExpectedRevision: opened.Revision}); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	stat := metadata.Sys().(*syscall.Stat_t)
	if int(stat.Uid) != os.Getuid() || int(stat.Gid) != gid {
		t.Fatalf("saved owner = %d:%d; want %d:%d", stat.Uid, stat.Gid, os.Getuid(), gid)
	}
	assertLocalMutationFile(t, filename, "after")
}

func TestLocalTextSaveKeepsNamedUserACLAndExtendedAttributes(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	attribute := localTextFixtureAttribute{name: "user.sshc.fixture", value: []byte("retained\x00value")}
	acl := localTextFixtureAttribute{name: "system.posix_acl_access", value: localTextFixtureACL(uint32(os.Getuid() + 1))}
	setLocalTextFixtureAttribute(t, filename, attribute)
	setLocalTextFixtureAttribute(t, filename, acl)
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := captureLocalTextMetadata(before)
	before.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "after", ExpectedRevision: opened.Revision}); err != nil {
		t.Fatal(err)
	}
	after, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Close()
	retained, err := captureLocalTextMetadata(after)
	if err != nil || retained.revision != expected.revision {
		t.Fatalf("saved metadata = %+v, %v; want %+v", retained, err, expected)
	}
	for _, attribute := range []localTextFixtureAttribute{attribute, acl} {
		if value := readLocalTextFixtureAttribute(t, filename, attribute.name); !bytes.Equal(value, attribute.value) {
			t.Fatalf("saved attribute %s = %x; want %x", attribute.name, value, attribute.value)
		}
	}
	assertLocalMutationFile(t, filename, "after")
}

func TestLocalTextSaveRefusesAttributesOrOwnershipChangedSinceOpening(t *testing.T) {
	for _, change := range []string{"attribute", "group"} {
		t.Run(change, func(t *testing.T) {
			manager, directory := localMutationFixture(t)
			filename := filepath.Join(directory, "notes.txt")
			writeLocalMutationFile(t, filename, "before")
			opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
			if err != nil {
				t.Fatal(err)
			}
			if change == "attribute" {
				setLocalTextFixtureAttribute(t, filename, localTextFixtureAttribute{name: "user.sshc.fixture", value: []byte("external")})
			} else if err := os.Chown(filename, -1, supplementaryLocalTextGroup(t)); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "mine", ExpectedRevision: opened.Revision}); !errors.Is(err, ErrConflict) {
				t.Fatalf("changed metadata save = %v; want conflict", err)
			}
			assertLocalMutationFile(t, filename, "before")
		})
	}
}

func TestLocalTextSaveRemovesAnInheritedACLAbsentFromTheOriginal(t *testing.T) {
	manager, directory := localMutationFixture(t)
	setLocalTextFixtureAttribute(t, directory, localTextFixtureAttribute{name: "system.posix_acl_default", value: localTextFixtureACL(uint32(os.Getuid() + 1))})
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	if err := unix.Removexattr(filename, "system.posix_acl_access"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filename, 0o640); err != nil {
		t.Fatal(err)
	}
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "after", ExpectedRevision: opened.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Getxattr(filename, "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
		t.Fatalf("replacement inherited access ACL: %v", err)
	}
}

func TestLocalTextSaveLeavesPrivilegedFilesUnchangedAndCleansStaging(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	if err := os.Chmod(filename, 0o755|fs.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "mine", ExpectedRevision: opened.Revision}); !errors.Is(err, ErrUnsupportedEntry) {
		t.Fatalf("privileged save = %v; want unsupported", err)
	}
	assertLocalMutationFile(t, filename, "before")
	children, err := os.ReadDir(directory)
	if err != nil || len(children) != 1 {
		t.Fatalf("staging left after refused save = %v, %v", children, err)
	}
}
