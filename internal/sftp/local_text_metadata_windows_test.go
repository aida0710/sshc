package sftp

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func windowsLocalTextFixtureMetadata(t *testing.T, filename string) localTextMetadata {
	t.Helper()
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	metadata, err := captureLocalTextMetadata(file)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func windowsLocalTextFixtureSecurity(t *testing.T, filename, suffix string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString("O:" + user.User.Sid.String() + "G:BUD:P(A;;FA;;;" + user.User.Sid.String() + ")(A;;FA;;;SY)" + suffix)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		t.Fatal(err)
	}
	group, _, err := descriptor.Group()
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(filename, windows.SE_FILE_OBJECT,
		windowsLocalTextSecurityInformation|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, group, dacl, nil); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsLocalTextSaveKeepsOwnerGroupDACLStreamsAndOrdinaryAttributes(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	windowsLocalTextFixtureSecurity(t, filename, "(A;;FR;;;BU)")
	for name, contents := range map[string]string{"Zone.Identifier": "[ZoneTransfer]\r\nZoneId=3\r\n", "日本語": "retained", "empty": ""} {
		if err := os.WriteFile(filename+":"+name, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	file, err := os.OpenFile(filename, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Keep a distinctive creation time, not the sibling's creation time.
	created := windows.NsecToFiletime(time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC).UnixNano())
	if err := windows.SetFileTime(windows.Handle(file.Fd()), &created, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := setWindowsLocalTextAttributes(windows.Handle(file.Fd()), windows.FILE_ATTRIBUTE_HIDDEN|windows.FILE_ATTRIBUTE_ARCHIVE|windows.FILE_ATTRIBUTE_NOT_CONTENT_INDEXED); err != nil {
		t.Fatal(err)
	}
	file.Close()
	expected := windowsLocalTextFixtureMetadata(t, filename)
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil || opened.Contents != "before" {
		t.Fatalf("open = %+v, %v", opened, err)
	}
	saved, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "after", ExpectedRevision: opened.Revision})
	if err != nil || saved.Contents != "after" {
		t.Fatalf("save = %+v, %v", saved, err)
	}
	retained := windowsLocalTextFixtureMetadata(t, filename)
	if retained.revision != expected.revision {
		t.Fatalf("saved metadata = %s; want %s", retained.platformMetadata, expected.platformMetadata)
	}
	for name, contents := range map[string]string{"Zone.Identifier": "[ZoneTransfer]\r\nZoneId=3\r\n", "日本語": "retained", "empty": ""} {
		observed, err := os.ReadFile(filename + ":" + name)
		if err != nil || string(observed) != contents {
			t.Fatalf("stream %s = %q, %v", name, observed, err)
		}
	}
}

func TestWindowsLocalTextSaveRefusesMetadataChangedSinceOpening(t *testing.T) {
	for _, change := range []string{"acl", "stream", "attributes"} {
		t.Run(change, func(t *testing.T) {
			manager, directory := localMutationFixture(t)
			filename := filepath.Join(directory, "notes.txt")
			writeLocalMutationFile(t, filename, "before")
			opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "acl":
				windowsLocalTextFixtureSecurity(t, filename, "(A;;FR;;;BU)")
			case "stream":
				if err := os.WriteFile(filename+":Zone.Identifier", []byte("changed"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "attributes":
				name, _ := windows.UTF16PtrFromString(filename)
				if err := windows.SetFileAttributes(name, windows.FILE_ATTRIBUTE_HIDDEN); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "mine", ExpectedRevision: opened.Revision}); !errors.Is(err, ErrConflict) {
				t.Fatalf("changed %s save = %v", change, err)
			}
			assertLocalMutationFile(t, filename, "before")
		})
	}
}

func TestWindowsLocalTextSaveLeavesReadOnlyFilesUnchangedAndCleansStaging(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	if err := os.Chmod(filename, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filename, 0o600) })
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "mine", ExpectedRevision: opened.Revision}); !errors.Is(err, ErrUnsupportedEntry) {
		t.Fatalf("read-only save = %v", err)
	}
	assertLocalMutationFile(t, filename, "before")
	children, err := os.ReadDir(directory)
	if err != nil || len(children) != 1 {
		t.Fatalf("staging left = %v, %v", children, err)
	}
}

func TestWindowsLocalTextRefusesUnboundedAlternateStreamMetadata(t *testing.T) {
	_, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	if err := os.WriteFile(filename+":large", bytes.Repeat([]byte{'x'}, maxLocalTextMetadataBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename}); !errors.Is(err, ErrUnsupportedEntry) {
		t.Fatalf("oversized metadata read = %v", err)
	}
	assertLocalMutationFile(t, filename, "before")
}

func TestWindowsLocalTextStagingIsPrivateBeforeTheFirstWrite(t *testing.T) {
	directory := t.TempDir()
	windowsLocalTextFixtureSecurity(t, directory, "(A;OICI;FR;;;BU)")
	parent, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staged, err := openLocalTextStagingFile(parent, "staged.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer staged.file.Close()
	defer staged.publicationFile.Close()
	descriptor, err := windows.GetSecurityInfo(windows.Handle(staged.file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sddl := descriptor.String()
	if !strings.Contains(sddl, "D:P") || !strings.Contains(sddl, user.User.Sid.String()) || !strings.Contains(sddl, "SY") || strings.Contains(sddl, "BU") || strings.Contains(sddl, "WD") {
		t.Fatalf("initial staging security = %s", sddl)
	}
}

func TestWindowsLocalTextPublicationUsesThePinnedParentAfterItsNameChanges(t *testing.T) {
	base := t.TempDir()
	directory := filepath.Join(base, "original")
	retired := filepath.Join(base, "retired")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeLocalMutationFile(t, filepath.Join(directory, "notes.txt"), "before")
	parent, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staged, err := openLocalTextStagingFile(parent, "staged.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer staged.publicationFile.Close()
	if _, err := staged.file.WriteString("after"); err != nil {
		t.Fatal(err)
	}
	if err := staged.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(directory, retired); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeLocalMutationFile(t, filepath.Join(directory, "notes.txt"), "replacement directory")
	if err := publishLocalTextReplacement(parent, staged, "notes.txt"); err != nil {
		t.Fatal(err)
	}
	assertLocalMutationFile(t, filepath.Join(retired, "notes.txt"), "after")
	assertLocalMutationFile(t, filepath.Join(directory, "notes.txt"), "replacement directory")
}

func TestWindowsLocalTextPublicationPinsTheStagedFileAndRefusesOtherDataWriters(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	parent, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staged, err := openLocalTextStagingFile(parent, "staged.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer staged.cleanup(parent)
	if _, err := staged.file.WriteString("after"); err != nil {
		t.Fatal(err)
	}
	snapshot := windowsLocalTextFixtureMetadata(t, filename)
	if err := applyLocalTextMetadata(nil, staged.file, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := staged.file.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(filepath.Join(directory, staged.name), os.O_WRONLY, 0)
	if writer != nil {
		writer.Close()
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("another writer after final DACL = %v", err)
	}
	if err := parent.Rename(staged.name, "moved.txt"); err != nil {
		t.Fatal(err)
	}
	writeLocalMutationFile(t, filepath.Join(directory, staged.name), "replacement staging file")
	if err := publishLocalTextReplacement(parent, staged, "notes.txt"); err != nil {
		t.Fatal(err)
	}
	assertLocalMutationFile(t, filename, "after")
	assertLocalMutationFile(t, filepath.Join(directory, staged.name), "replacement staging file")
}

func TestWindowsLocalTextSavePreservesExtendedAttributes(t *testing.T) {
	manager, directory := localMutationFixture(t)
	filename := filepath.Join(directory, "notes.txt")
	writeLocalMutationFile(t, filename, "before")
	file, err := os.OpenFile(filename, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	// FILE_FULL_EA_INFORMATION consists of offset/flags/name length/value
	// length, a terminated byte name, then the value.
	attribute := []byte{0, 0, 0, 0, 0, 4, 3, 0, 'N', 'O', 'T', 'E', 0, 'e', 'a', '!'}
	var status windows.IO_STATUS_BLOCK
	err = windows.NtSetEaFile(windows.Handle(file.Fd()), &status, &attribute[0], uint32(len(attribute)))
	file.Close()
	if err != nil {
		t.Fatal(localWindowsFileError(err))
	}
	expected := windowsLocalTextFixtureMetadata(t, filename)
	opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "after", ExpectedRevision: opened.Revision}); err != nil {
		t.Fatal(err)
	}
	retained := windowsLocalTextFixtureMetadata(t, filename)
	if retained.revision != expected.revision {
		t.Fatal("Windows extended attributes changed on save")
	}
	file, err = os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	buffer := make([]byte, len(attribute))
	if err := windows.NtQueryEaFile(windows.Handle(file.Fd()), &status, (*byte)(unsafe.Pointer(&buffer[0])), uint32(len(buffer)), false, nil, 0, nil, true); err != nil {
		t.Fatal(localWindowsFileError(err))
	}
	if !bytes.Equal(buffer, attribute) {
		t.Fatalf("saved EA = %x; want %x", buffer, attribute)
	}
}

func TestWindowsLocalTextSaveKeepsMandatoryLabelsAndResourceAttributes(t *testing.T) {
	for _, fixture := range []struct {
		name        string
		sddl        string
		marker      string
		information windows.SECURITY_INFORMATION
	}{
		{name: "integrity label", sddl: "S:(ML;;NW;;;LW)", marker: "ML;;NW;;;LW", information: windows.LABEL_SECURITY_INFORMATION},
		{name: "resource attribute", sddl: `S:(RA;;;;;WD;("Project",TS,0,"sshc"))`, marker: "Project", information: windows.ATTRIBUTE_SECURITY_INFORMATION},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			manager, directory := localMutationFixture(t)
			filename := filepath.Join(directory, "notes.txt")
			writeLocalMutationFile(t, filename, "before")
			descriptor, err := windows.SecurityDescriptorFromString(fixture.sddl)
			if err != nil {
				t.Fatal(err)
			}
			sacl, _, err := descriptor.SACL()
			if err != nil {
				t.Fatal(err)
			}
			if err := windows.SetNamedSecurityInfo(filename, windows.SE_FILE_OBJECT, fixture.information, nil, nil, nil, sacl); err != nil {
				t.Fatal(err)
			}
			expected := windowsLocalTextFixtureMetadata(t, filename)
			if !bytes.Contains(expected.platformMetadata, []byte(fixture.marker)) {
				t.Fatalf("fixture %s absent = %s", fixture.name, expected.platformMetadata)
			}
			opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "after", ExpectedRevision: opened.Revision}); err != nil {
				t.Fatal(err)
			}
			retained := windowsLocalTextFixtureMetadata(t, filename)
			if retained.revision != expected.revision {
				t.Fatalf("saved %s = %s; want %s", fixture.name, retained.platformMetadata, expected.platformMetadata)
			}
		})
	}
}
