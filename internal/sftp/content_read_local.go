package sftp

import (
	"io/fs"
	"os"
	"path"
)

func inspectLocalReadPath(root *os.Root, relative string) (map[string]fs.FileInfo, error) {
	metadata := make(map[string]fs.FileInfo)
	for current := relative; ; current = path.Dir(current) {
		info, err := root.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, ErrConflict
		}
		if current != relative && !info.IsDir() {
			return nil, ErrConflict
		}
		metadata[current] = info
		if current == "." {
			return metadata, nil
		}
	}
}

func verifyLocalReadPath(root *os.Root, metadata map[string]fs.FileInfo) error {
	for relative, expected := range metadata {
		current, err := root.Lstat(relative)
		if err != nil || !os.SameFile(current, expected) || metadataRevision(current) != metadataRevision(expected) {
			return ErrConflict
		}
	}
	return nil
}

type localContentRead struct {
	root     *os.Root
	relative string
	metadata map[string]fs.FileInfo
}

func openStableLocalFile(request localContentRead) (stableContentFile, error) {
	relative := request.relative
	metadata, err := inspectLocalReadPath(request.root, relative)
	if err != nil {
		return stableContentFile{}, labelLocalAccessRefusal(existingContentReadError(err))
	}
	expected := request.metadata[relative]
	current := metadata[relative]
	if expected == nil || !current.Mode().IsRegular() || !os.SameFile(current, expected) || metadataRevision(current) != metadataRevision(expected) {
		return stableContentFile{}, ErrConflict
	}
	// Verify listed ancestors too; an in-root replacement must be a conflict.
	for parent, observed := range metadata {
		if listed, exists := request.metadata[parent]; exists && !os.SameFile(observed, listed) {
			return stableContentFile{}, ErrConflict
		}
	}
	opened, err := openLocalContentFile(request.root, relative)
	if err != nil {
		return stableContentFile{}, labelLocalAccessRefusal(existingContentReadError(err))
	}
	verify := func() error {
		if err := verifyLocalOpenedFile(opened, expected); err != nil {
			return err
		}
		return verifyLocalReadPath(request.root, metadata)
	}
	if err := verify(); err != nil {
		opened.Close()
		return stableContentFile{}, err
	}
	return stableContentFile{reader: opened, size: expected.Size(), verify: verify}, nil
}

func verifyLocalOpenedFile(opened *os.File, expected fs.FileInfo) error {
	info, err := opened.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(info, expected) || metadataRevision(info) != metadataRevision(expected) {
		return ErrConflict
	}
	return nil
}
