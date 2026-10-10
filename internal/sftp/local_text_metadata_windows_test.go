package sftp

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 || dacl == nil || dacl.AceCount != 2 {
		t.Fatalf("initial staging security = %s", descriptor.String())
	}
	// FILE_ALL_ACCESS from winnt.h includes the file-specific DELETE_CHILD bit.
	const fileAllAccess = 0x001f01ff
	seenUser, seenSystem := false, false
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			t.Fatal(err)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 || ace.Mask != fileAllAccess {
			t.Fatalf("unexpected staging ACE: %+v", ace)
		}
		switch {
		case sid.Equals(user.User.Sid):
			seenUser = true
		case sid.Equals(system):
			seenSystem = true
		default:
			t.Fatalf("staging grants access to %s", sid.String())
		}
	}
	if !seenUser || !seenSystem {
		t.Fatalf("initial staging security = %s", descriptor.String())
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
	baseRoot, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	defer baseRoot.Close()
	// Go's top-level OpenRoot handle omits delete sharing. A relative child
	// Root permits the ancestor rename while retaining the directory identity.
	parent, err := baseRoot.OpenRoot("original")
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	// NTFS refuses directory renames while a descendant file is open.
	// Change the parent name before creating the pinned publication handle.
	if err := os.Rename(directory, retired); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeLocalMutationFile(t, filepath.Join(directory, "notes.txt"), "replacement directory")
	staged, err := openLocalTextStagingFile(parent, "staged.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer staged.cleanup(parent)
	if _, err := staged.file.WriteString("after"); err != nil {
		t.Fatal(err)
	}
	if err := staged.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := staged.publish(parent, "notes.txt"); err != nil {
		t.Fatal(err)
	}
	staged.cleanup(parent)
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
	if err := staged.publish(parent, "notes.txt"); err != nil {
		t.Fatal(err)
	}
	staged.cleanup(parent)
	assertLocalMutationFile(t, filename, "after")
	assertLocalMutationFile(t, filepath.Join(directory, staged.name), "replacement staging file")
}

func TestWindowsLocalTextFailedPublicationCleansOnlyItsOwnMovedStagingFile(t *testing.T) {
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
	if err := parent.Rename(staged.name, "moved.txt"); err != nil {
		t.Fatal(err)
	}
	writeLocalMutationFile(t, filepath.Join(directory, staged.name), "replacement staging file")
	if err := parent.Mkdir("blocked", 0o700); err != nil {
		t.Fatal(err)
	}
	writeLocalMutationFile(t, filepath.Join(directory, "blocked", "keep.txt"), "keep")
	if err := staged.publish(parent, "blocked"); err == nil {
		t.Fatal("publication replaced a nonempty directory")
	}
	staged.cleanup(parent)
	assertLocalMutationFile(t, filename, "before")
	assertLocalMutationFile(t, filepath.Join(directory, staged.name), "replacement staging file")
	assertLocalMutationFile(t, filepath.Join(directory, "blocked", "keep.txt"), "keep")
	if _, err := parent.Lstat("moved.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original moved staging file remains after cleanup: %v", err)
	}
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

func TestWindowsLocalTextSaveDoesNotAddCurrentParentPermissions(t *testing.T) {
	for _, policy := range []struct{ name, flags string }{
		{name: "explicit", flags: ""},
		{name: "automatic inheritance", flags: "AI"},
	} {
		t.Run(policy.name, func(t *testing.T) {
			manager, directory := localMutationFixture(t)
			filename := filepath.Join(directory, "notes.txt")
			writeLocalMutationFile(t, filename, "before")
			windowsLocalTextFixtureSecurity(t, filename, "")
			windowsLocalTextFixtureSecurity(t, directory, "(A;OICI;FR;;;BU)")
			parent, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			handle, err := openLocalWindowsEntry(parent, "notes.txt", windows.WRITE_OWNER|windows.WRITE_DAC|windows.READ_CONTROL)
			if err != nil {
				t.Fatal(err)
			}
			source := os.NewFile(uintptr(handle), filename)
			defer source.Close()
			user, err := windows.GetCurrentProcessToken().GetTokenUser()
			if err != nil {
				t.Fatal(err)
			}
			// A moved file can be unprotected and narrower than its current parent.
			sddl := "O:" + user.User.Sid.String() + "G:BUD:" + policy.flags + "(A;;FA;;;" + user.User.Sid.String() + ")(A;;FA;;;SY)"
			if err := applyWindowsLocalTextSecurity(source, sddl); err != nil {
				t.Fatal(err)
			}
			source.Close()
			expected := windowsLocalTextFixtureMetadata(t, filename)
			descriptor, err := windows.SecurityDescriptorFromString(sddl)
			if err != nil {
				t.Fatal(err)
			}
			var platform windowsLocalTextMetadata
			if err := json.Unmarshal(expected.platformMetadata, &platform); err != nil {
				t.Fatal(err)
			}
			if platform.SecurityDescriptor != descriptor.String() {
				t.Fatalf("fixture ACL = %s; want %s", platform.SecurityDescriptor, descriptor.String())
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
				t.Fatalf("parent permissions added: %s; want %s", retained.platformMetadata, expected.platformMetadata)
			}
			assertLocalMutationFile(t, filename, "after")
		})
	}
}

func TestWindowsLocalTextSaveRefusesAnUnexpectedInheritedIntegrityLabel(t *testing.T) {
	for _, policy := range []struct{ name, sddl string }{
		{name: "source has no label", sddl: ""},
		{name: "source has a different label", sddl: "S:(ML;;NW;;;ME)"},
	} {
		t.Run(policy.name, func(t *testing.T) {
			manager, directory := localMutationFixture(t)
			filename := filepath.Join(directory, "notes.txt")
			writeLocalMutationFile(t, filename, "before")
			if policy.sddl != "" {
				descriptor, err := windows.SecurityDescriptorFromString(policy.sddl)
				if err != nil {
					t.Fatal(err)
				}
				sacl, _, err := descriptor.SACL()
				if err != nil {
					t.Fatal(err)
				}
				if err := windows.SetNamedSecurityInfo(filename, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, sacl); err != nil {
					t.Fatal(err)
				}
			}
			expected := windowsLocalTextFixtureMetadata(t, filename)
			opened, err := ReadLocalText(t.Context(), LocalTextReadOptions{Path: filename})
			if err != nil {
				t.Fatal(err)
			}
			name, err := windows.UTF16PtrFromString(directory)
			if err != nil {
				t.Fatal(err)
			}
			handle, err := windows.CreateFile(name, windows.WRITE_OWNER, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
				nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
			if err != nil {
				t.Fatal(err)
			}
			parentFile := os.NewFile(uintptr(handle), directory)
			defer parentFile.Close()
			label, err := windows.SecurityDescriptorFromString("S:(ML;OICI;NW;;;LW)")
			if err != nil {
				t.Fatal(err)
			}
			// The native setter updates the parent without propagating to the existing
			// source. A new sibling inherits the label, unlike the selected source.
			if err := setWindowsLocalTextSecurityDescriptor(parentFile, windows.LABEL_SECURITY_INFORMATION, label); err != nil {
				t.Fatal(err)
			}
			current := windowsLocalTextFixtureMetadata(t, filename)
			if current.revision != expected.revision {
				t.Fatalf("source fixture was changed: %s; want %s", current.platformMetadata, expected.platformMetadata)
			}
			parent, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			staged, err := openLocalTextStagingFile(parent, "verify-label.txt")
			if err != nil {
				t.Fatal(err)
			}
			stageMetadata, err := captureLocalTextMetadata(staged.file)
			if err != nil {
				t.Fatal(err)
			}
			staged.cleanup(parent)
			parent.Close()
			if !bytes.Contains(stageMetadata.platformMetadata, []byte("ML;ID;NW;;;LW")) {
				t.Fatalf("stage did not inherit low label: %s", stageMetadata.platformMetadata)
			}
			if _, err := manager.SaveLocalText(t.Context(), LocalTextSaveRequest{Path: filename, Contents: "after", ExpectedRevision: opened.Revision}); !errors.Is(err, ErrUnsupportedEntry) {
				t.Fatalf("unexpected inherited label save = %v; want safe refusal", err)
			}
			retained := windowsLocalTextFixtureMetadata(t, filename)
			if retained.revision != expected.revision {
				t.Fatalf("source metadata changed after refusal: %s; want %s", retained.platformMetadata, expected.platformMetadata)
			}
			assertLocalMutationFile(t, filename, "before")
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 {
				t.Fatalf("staging left after refusal = %v, %v", entries, err)
			}
		})
	}
}
