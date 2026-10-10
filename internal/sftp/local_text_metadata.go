package sftp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"slices"
)

// Metadata is bounded independently of the editable contents. Resource forks
// and extended attributes must never turn a small text read into an unlimited one.
const maxLocalTextMetadataBytes = 16 << 20

type localTextMetadata struct {
	uid                int
	gid                int
	mode               fs.FileMode
	extendedAttributes map[string][]byte
	platformMetadata   []byte
	revision           string
}

func preserveLocalTextMetadata(target localMutationTarget, staged *os.File, snapshot localTextMetadata) error {
	observed, err := inspectLocalReadPath(target.parent, target.name)
	if err != nil {
		return err
	}
	file, err := openStableLocalFile(localContentRead{root: target.parent, relative: target.name, metadata: observed})
	if err != nil {
		return err
	}
	defer file.reader.Close()
	source := file.reader.(*os.File)
	current, err := captureLocalTextMetadata(source)
	if err != nil {
		return err
	}
	if current.revision != snapshot.revision {
		return ErrConflict
	}
	if err := applyLocalTextMetadata(source, staged, snapshot); err != nil {
		return err
	}
	if err := file.verify(); err != nil {
		return err
	}
	current, err = captureLocalTextMetadata(source)
	if err != nil {
		return err
	}
	retained, err := captureLocalTextMetadata(staged)
	if err != nil {
		return err
	}
	if current.revision != snapshot.revision || retained.revision != snapshot.revision {
		return ErrConflict
	}
	return nil
}

func localTextMetadataRevision(metadata localTextMetadata) string {
	digest := sha256.New()
	_, _ = fmt.Fprintf(digest, "%d:%d:%d\x00", metadata.uid, metadata.gid, metadata.mode)
	names := make([]string, 0, len(metadata.extendedAttributes))
	for name := range metadata.extendedAttributes {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		value := metadata.extendedAttributes[name]
		_, _ = fmt.Fprintf(digest, "%d:%s:%d:", len(name), name, len(value))
		_, _ = digest.Write(value)
	}
	_, _ = digest.Write(metadata.platformMetadata)
	return hex.EncodeToString(digest.Sum(nil))
}
