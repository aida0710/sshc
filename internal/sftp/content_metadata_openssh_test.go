package sftp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	pkgsftp "github.com/pkg/sftp"
)

type incompleteContentRemote struct {
	*Client
	missingAttribute uint32
	openedFiles      int
}

func (remote *incompleteContentRemote) Lstat(candidate string) (fs.FileInfo, error) {
	info, err := remote.Client.Lstat(candidate)
	if err != nil || !info.Mode().IsRegular() {
		return info, err
	}
	flags, _ := metadataFlagsFrom(info)
	attributes := *info.Sys().(*pkgsftp.FileStat)
	attributes.Extended = []pkgsftp.StatExtended{{ExtType: metadataFlagsAttribute, ExtData: strconv.FormatUint(uint64(flags&^remote.missingAttribute), 10)}}
	return incompleteContentInfo{FileInfo: info, attributes: &attributes, missingAttribute: remote.missingAttribute}, nil
}

func (remote *incompleteContentRemote) Open(candidate string) (io.ReadCloser, error) {
	remote.openedFiles++
	return remote.Client.Open(candidate)
}

type incompleteContentInfo struct {
	fs.FileInfo
	attributes       *pkgsftp.FileStat
	missingAttribute uint32
}

func (info incompleteContentInfo) Sys() any { return info.attributes }

func (info incompleteContentInfo) Size() int64 {
	if info.missingAttribute == sftpSizeAttributes {
		return 0
	}
	return info.FileInfo.Size()
}

func TestContentSearchDoesNotReadFilesWithMissingSizeOrModificationTime(t *testing.T) {
	for name, attribute := range map[string]uint32{"size": sftpSizeAttributes, "mtime": sftpTimeAttributes} {
		t.Run(name, func(t *testing.T) {
			client := openOpenSSHTestClient(t)
			remote := &incompleteContentRemote{Client: client, missingAttribute: attribute}
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "file.txt"), []byte("needle\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			service := Service{Open: func(context.Context, string) (Remote, error) { return remote, nil }}
			search, err := service.Search(t.Context(), SearchOptions{Alias: "fixture", Path: filepath.ToSlash(directory), Query: "needle", Mode: SearchContent})
			if err != nil || len(search.Matches) != 0 || remote.openedFiles != 0 || len(search.Omissions) != 1 || search.Omissions[0].Reason != omissionUnreadable {
				t.Fatalf("incomplete metadata search = %+v, opened = %d, error = %v", search, remote.openedFiles, err)
			}
		})
	}
}

func TestRecoveryRejectsMissingSizeBeforeOpeningTheSource(t *testing.T) {
	client := openOpenSSHTestClient(t)
	remote := &incompleteContentRemote{Client: client, missingAttribute: sftpSizeAttributes}
	filePath := filepath.Join(t.TempDir(), "nonempty.txt")
	if err := os.WriteFile(filePath, []byte("must not become an empty copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	end := serverFileEnd{remote: remote}
	if _, err := end.stat(filepath.ToSlash(filePath)); !errors.Is(err, ErrMetadataUnavailable) || remote.openedFiles != 0 {
		t.Fatalf("missing-size recovery source = %v, opened = %d", err, remote.openedFiles)
	}
}
