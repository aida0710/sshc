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

type localByteReadRequest struct {
	target           localMutationTarget
	limit            int64
	withTextMetadata bool
}

type localByteRead struct {
	contents     []byte
	metadata     fs.FileInfo
	textMetadata localTextMetadata
}

// Pin the resolved parent as local mutations do, then refuse the final link.
// The handle and path must still name the same regular file after the read.
func readLocalBytes(ctx context.Context, request localByteReadRequest) (localByteRead, error) {
	target, limit := request.target, request.limit
	metadata, err := inspectLocalReadPath(target.parent, target.name)
	if err != nil {
		return localByteRead{}, err
	}
	info := metadata[target.name]
	if !info.Mode().IsRegular() {
		return localByteRead{}, ErrNotRegularFile
	}
	if info.Size() < 0 || info.Size() > limit {
		return localByteRead{}, ErrTextTooLarge
	}
	file, err := openStableLocalFile(localContentRead{root: target.parent, relative: target.name, metadata: metadata})
	if err != nil {
		return localByteRead{}, err
	}
	defer file.reader.Close()
	textMetadata := localTextMetadata{}
	if request.withTextMetadata {
		textMetadata, err = captureLocalTextMetadata(file.reader.(*os.File))
		if err != nil {
			return localByteRead{}, err
		}
	}
	var contents bytes.Buffer
	budget := contentReadBudget{maxBytes: limit}
	if err := budget.stream(ctx, contentStream{file: file, destination: &contents}); err != nil {
		return localByteRead{}, err
	}
	if request.withTextMetadata {
		latest, err := captureLocalTextMetadata(file.reader.(*os.File))
		if err != nil {
			return localByteRead{}, err
		}
		if latest.revision != textMetadata.revision {
			return localByteRead{}, ErrConflict
		}
	}
	return localByteRead{contents: contents.Bytes(), metadata: info, textMetadata: textMetadata}, nil
}

func localTextFile(target localMutationTarget, read localByteRead) (TextFile, error) {
	contents, metadata := read.contents, read.metadata
	if !validText(contents) {
		return TextFile{}, ErrNotUTF8
	}
	identity, err := localDeletionIdentity(target.parent, target.name, metadata)
	if err != nil {
		return TextFile{}, err
	}
	// Content alone cannot detect replacement by another file with equal bytes.
	revision := contentRevision([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s", identity, metadataRevision(metadata), contentRevision(contents), read.textMetadata.revision)))
	return TextFile{Entry: localMutationEntry(target.publicPath, metadata), Contents: string(contents), Revision: revision}, nil
}

func ReadLocalText(ctx context.Context, options LocalTextReadOptions) (file TextFile, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	target, err := openLocalMutationTarget(options.Path)
	if err != nil {
		return TextFile{}, err
	}
	defer target.parent.Close()
	read, err := readLocalBytes(ctx, localByteReadRequest{target: target, limit: MaxEditableFileBytes, withTextMetadata: true})
	if err != nil {
		return TextFile{}, err
	}
	if options.ExpectedRevision != "" && metadataRevision(read.metadata) != options.ExpectedRevision {
		return TextFile{}, ErrConflict
	}
	return localTextFile(target, read)
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
	read, err := readLocalBytes(ctx, localByteReadRequest{target: target, limit: MaxEditableFileBytes, withTextMetadata: true})
	if err != nil {
		return TextFile{}, err
	}
	current, err := localTextFile(target, read)
	if err != nil {
		return TextFile{}, err
	}
	if current.Revision != request.ExpectedRevision {
		return TextFile{}, ErrConflict
	}
	if err := publishLocalText(ctx, localTextPublication{target: target, request: request, metadata: read.textMetadata}); err != nil {
		return TextFile{}, err
	}
	read, err = readLocalBytes(ctx, localByteReadRequest{target: target, limit: MaxEditableFileBytes, withTextMetadata: true})
	if err != nil {
		return TextFile{}, err
	}
	return localTextFile(target, read)
}

type localTextPublication struct {
	target   localMutationTarget
	request  LocalTextSaveRequest
	metadata localTextMetadata
}

func publishLocalText(ctx context.Context, publication localTextPublication) error {
	target, request := publication.target, publication.request
	suffix, err := newLocalSuffix()
	if err != nil {
		return err
	}
	// Keep the sibling name short enough to edit files with maximum-length names.
	temporary := ".sshc-editor.sshc-" + suffix + ".tmp"
	staged, err := openLocalTextStagingFile(target.parent, temporary)
	if err != nil {
		return err
	}
	defer target.parent.Remove(temporary)
	if _, err := staged.WriteString(request.Contents); err != nil {
		staged.Close()
		return err
	}
	if err := preserveLocalTextMetadata(target, staged, publication.metadata); err != nil {
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
	read, err := readLocalBytes(ctx, localByteReadRequest{target: target, limit: MaxEditableFileBytes, withTextMetadata: true})
	if err != nil {
		return err
	}
	current, err := localTextFile(target, read)
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
	return publishLocalTextReplacement(target.parent, temporary, target.name)
}
