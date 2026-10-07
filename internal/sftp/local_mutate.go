package sftp

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
)

type LocalMkdirRequest struct {
	Directory string
	Name      string
}
type LocalRenameRequest struct {
	Path             string
	Name             string
	ExpectedRevision string
}

// New local folders are private, matching files created by engine-side get.
const localCreatedDirectoryMode fs.FileMode = 0o700

func validLocalMutationName(name string) bool {
	if !ValidLocalChildName(name) || isLocalTemporaryName(name) {
		return false
	}
	if runtime.GOOS != "windows" {
		return true
	}
	// Windows 11 accepts some device names with extensions. Keep mutation names
	// portable to Windows versions that still interpret them as devices.
	base, _, _ := strings.Cut(name, ".")
	return strings.TrimRight(name, ". ") == name && (base == "" || filepath.IsLocal(strings.TrimRight(base, " ")))
}

func (m *TransferManager) MkdirLocal(ctx context.Context, request LocalMkdirRequest) (entry Entry, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	if m == nil {
		return Entry{}, ErrUnavailable
	}
	if request.Directory == "" || hasLocalTraversalSegment(request.Directory) || !validLocalMutationName(request.Name) {
		return Entry{}, ErrInvalidPath
	}
	directory, err := cleanLocalPath(request.Directory)
	if err != nil {
		return Entry{}, err
	}
	m.localMutationsMutex.Lock()
	defer m.localMutationsMutex.Unlock()
	if m.isClosed() {
		return Entry{}, ErrUnavailable
	}
	target, err := openLocalMutationTarget(joinLocalMutationPath(directory, request.Name))
	if err != nil {
		return Entry{}, err
	}
	defer target.parent.Close()
	if err := m.refuseLocalTransferOverlap([]string{target.absolute}); err != nil {
		return Entry{}, err
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	if err := target.parent.Mkdir(target.name, localCreatedDirectoryMode); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Entry{}, ErrAlreadyExists
		}
		return Entry{}, err
	}
	metadata, err := target.parent.Lstat(target.name)
	if err != nil {
		return Entry{}, err
	}
	return localMutationEntry(target.publicPath, metadata), nil
}

func (m *TransferManager) RenameLocal(ctx context.Context, request LocalRenameRequest) (entry Entry, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	if !validLocalMutationName(request.Name) {
		return Entry{}, ErrInvalidPath
	}
	// The same inspection also protects temporary files nested in folders.
	plan, err := m.PrepareLocalDelete(ctx, []LocalDeleteEntry{{Path: request.Path, ExpectedRevision: request.ExpectedRevision}})
	if err != nil {
		return Entry{}, err
	}
	defer plan.Close()
	source := plan.targets[len(plan.targets)-1]
	if request.Name == source.name {
		return Entry{}, ErrAlreadyExists
	}
	absoluteTarget := joinLocalMutationPath(source.parent.Name(), request.Name)
	if err := m.refuseLocalTransferOverlap([]string{absoluteTarget}); err != nil {
		return Entry{}, err
	}
	if _, err := source.parent.Lstat(request.Name); err == nil {
		return Entry{}, ErrAlreadyExists
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Entry{}, err
	}
	if err := plan.verify(ctx); err != nil {
		return Entry{}, err
	}
	if err := renameLocalMutationWithoutReplace(source.parent, source.name, request.Name); err != nil {
		return Entry{}, err
	}
	updated, err := source.parent.Lstat(request.Name)
	if err != nil {
		return Entry{}, err
	}
	cleaned, err := cleanLocalPath(request.Path)
	if err != nil {
		return Entry{}, err
	}
	return localMutationEntry(joinLocalMutationPath(filepath.Dir(filepath.FromSlash(cleaned)), request.Name), updated), nil
}
