package sftp

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
)

type LocalTextReadOptions struct {
	Path             string
	ExpectedRevision string
}

type LocalTextSaveRequest struct {
	Path             string
	Contents         string
	ExpectedRevision string
}

// Pin the resolved parent as local mutations do, then refuse the final link.
// The handle and path must still name the same regular file after the read.
func readLocalBytes(ctx context.Context, target localMutationTarget, limit int64) ([]byte, fs.FileInfo, error) {
	metadata, err := inspectLocalReadPath(target.parent, target.name)
	if err != nil {
		return nil, nil, err
	}
	info := metadata[target.name]
	if !info.Mode().IsRegular() {
		return nil, nil, ErrNotRegularFile
	}
	if info.Size() < 0 || info.Size() > limit {
		return nil, nil, ErrTextTooLarge
	}
	file, err := openStableLocalFile(localContentRead{root: target.parent, relative: target.name, metadata: metadata})
	if err != nil {
		return nil, nil, err
	}
	defer file.reader.Close()
	var contents bytes.Buffer
	budget := contentReadBudget{maxBytes: limit}
	if err := budget.stream(ctx, contentStream{file: file, destination: &contents}); err != nil {
		return nil, nil, err
	}
	return contents.Bytes(), info, nil
}

func localTextFile(target localMutationTarget, contents []byte, metadata fs.FileInfo) (TextFile, error) {
	if !validText(contents) {
		return TextFile{}, ErrNotUTF8
	}
	identity, err := localDeletionIdentity(target.parent, target.name, metadata)
	if err != nil {
		return TextFile{}, err
	}
	// Content alone cannot detect replacement by another file with equal bytes.
	revision := contentRevision([]byte(fmt.Sprintf("%s\x00%s\x00%s", identity, metadataRevision(metadata), contentRevision(contents))))
	return TextFile{Entry: localMutationEntry(target.publicPath, metadata), Contents: string(contents), Revision: revision}, nil
}

func ReadLocalText(ctx context.Context, options LocalTextReadOptions) (file TextFile, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	target, err := openLocalMutationTarget(options.Path)
	if err != nil {
		return TextFile{}, err
	}
	defer target.parent.Close()
	contents, metadata, err := readLocalBytes(ctx, target, MaxEditableFileBytes)
	if err != nil {
		return TextFile{}, err
	}
	if options.ExpectedRevision != "" && metadataRevision(metadata) != options.ExpectedRevision {
		return TextFile{}, ErrConflict
	}
	return localTextFile(target, contents, metadata)
}

func (m *TransferManager) SaveLocalText(ctx context.Context, request LocalTextSaveRequest) (file TextFile, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	if m == nil {
		return TextFile{}, ErrUnavailable
	}
	if request.ExpectedRevision == "" {
		return TextFile{}, ErrRevisionRequired
	}
	if len(request.Contents) > MaxEditableFileBytes {
		return TextFile{}, ErrTextTooLarge
	}
	if !validText([]byte(request.Contents)) {
		return TextFile{}, ErrNotUTF8
	}
	m.localMutationsMutex.Lock()
	defer m.localMutationsMutex.Unlock()
	if m.isClosed() {
		return TextFile{}, ErrUnavailable
	}
	target, err := openLocalMutationTarget(request.Path)
	if err != nil {
		return TextFile{}, err
	}
	defer target.parent.Close()
	if err := m.refuseLocalTransferOverlap([]string{target.absolute}); err != nil {
		return TextFile{}, err
	}
	contents, metadata, err := readLocalBytes(ctx, target, MaxEditableFileBytes)
	if err != nil {
		return TextFile{}, err
	}
	current, err := localTextFile(target, contents, metadata)
	if err != nil {
		return TextFile{}, err
	}
	if current.Revision != request.ExpectedRevision {
		return TextFile{}, ErrConflict
	}
	if err := publishLocalText(ctx, localTextPublication{target: target, request: request, mode: metadata.Mode().Perm()}); err != nil {
		return TextFile{}, err
	}
	contents, metadata, err = readLocalBytes(ctx, target, MaxEditableFileBytes)
	if err != nil {
		return TextFile{}, err
	}
	return localTextFile(target, contents, metadata)
}

type localTextPublication struct {
	target  localMutationTarget
	request LocalTextSaveRequest
	mode    fs.FileMode
}

func publishLocalText(ctx context.Context, publication localTextPublication) error {
	target, request, mode := publication.target, publication.request, publication.mode
	suffix, err := newLocalSuffix()
	if err != nil {
		return err
	}
	// Keep the sibling name short enough to edit files with maximum-length names.
	temporary := ".sshc-editor.sshc-" + suffix + ".tmp"
	staged, err := target.parent.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer target.parent.Remove(temporary)
	if _, err := staged.WriteString(request.Contents); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Chmod(mode); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Sync(); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	contents, metadata, err := readLocalBytes(ctx, target, MaxEditableFileBytes)
	if err != nil {
		return err
	}
	current, err := localTextFile(target, contents, metadata)
	if err != nil {
		return err
	}
	if current.Revision != request.ExpectedRevision {
		return ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Rename publishes a complete sibling and never follows a destination link.
	return target.parent.Rename(temporary, target.name)
}
