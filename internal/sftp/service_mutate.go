package sftp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
)

// Changes to the remote tree: create, rename, delete, and the
// temporary-file replace that keeps a half-written file from being seen.

func (s Service) Mkdir(ctx context.Context, alias, remotePath string) (Entry, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return Entry{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	if mkdirErr := remote.Mkdir(cleaned); mkdirErr != nil {
		info, statErr := remote.Lstat(cleaned)
		if statErr != nil {
			return Entry{}, mkdirErr
		}
		if !info.IsDir() {
			return Entry{}, ErrAlreadyExists
		}
		return entryFrom(path.Dir(cleaned), namedInfo{FileInfo: info, name: path.Base(cleaned)}), nil
	}
	info, err := remote.Lstat(cleaned)
	if err != nil {
		return Entry{}, err
	}
	return entryFrom(path.Dir(cleaned), namedInfo{FileInfo: info, name: path.Base(cleaned)}), nil
}

// CreateEmptyFile creates a new zero-byte regular file without replacing an
// existing remote path. O_EXCL closes the race between an existence check and
// creation on servers that support the standard SFTP open flags.
func (s Service) CreateEmptyFile(ctx context.Context, alias, remotePath string) (Entry, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return Entry{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	file, err := remote.OpenFile(cleaned, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Entry{}, ErrAlreadyExists
		}
		return Entry{}, err
	}
	if err := file.Close(); err != nil {
		return Entry{}, err
	}
	info, err := remote.Lstat(cleaned)
	if err != nil {
		return Entry{}, err
	}
	if !info.Mode().IsRegular() {
		return Entry{}, ErrNotRegularFile
	}
	return entryFrom(path.Dir(cleaned), namedInfo{FileInfo: info, name: path.Base(cleaned)}), nil
}

// Rename は既存の移動先を上書きしない。置換は Upload と SaveText だけが明示的に扱う。
func (s Service) Rename(ctx context.Context, alias, from, to string) (Entry, error) {
	source, err := cleanPublicPath(from, false)
	if err != nil {
		return Entry{}, err
	}
	target, err := cleanPublicPath(to, false)
	if err != nil {
		return Entry{}, err
	}
	if source == target {
		return Entry{}, ErrAlreadyExists
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	if _, statErr := remote.Lstat(target); statErr == nil {
		return Entry{}, ErrAlreadyExists
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return Entry{}, statErr
	}
	if err := remote.Rename(source, target); err != nil {
		return Entry{}, err
	}
	info, err := remote.Lstat(target)
	if err != nil {
		return Entry{}, err
	}
	return entryFrom(path.Dir(target), namedInfo{FileInfo: info, name: path.Base(target)}), nil
}

func (s Service) replace(
	ctx context.Context,
	remote Remote,
	target string,
	source io.Reader,
	mode fs.FileMode,
	maxBytes int64,
	verify func() error,
) (written int64, resultErr error) {
	temporary, err := s.temporaryPath(target)
	if err != nil {
		return 0, err
	}
	if path.Dir(temporary) != path.Dir(target) || temporary == target {
		return 0, fmt.Errorf("%w: temporary path must be beside the target", ErrInvalidPath)
	}
	file, err := remote.Create(temporary)
	if err != nil {
		return 0, err
	}
	cleanup := true
	defer func() {
		if closeErr := file.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
		if cleanup {
			_ = remote.Remove(temporary)
		}
	}()

	written, err = copyContext(ctx, file, source, maxBytes)
	if err != nil {
		return written, err
	}
	closeErr := file.Close()
	file = closedWriter{}
	if closeErr != nil {
		return written, closeErr
	}
	if err := remote.Chmod(temporary, mode.Perm()); err != nil {
		return written, err
	}
	if verify != nil {
		if err := verify(); err != nil {
			return written, err
		}
	}
	if err := remote.Replace(temporary, target); err != nil {
		return written, err
	}
	cleanup = false
	return written, nil
}

func (s Service) temporaryPath(target string) (string, error) {
	if s.TemporaryPath != nil {
		return s.TemporaryPath(target)
	}
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return path.Join(path.Dir(target), "."+path.Base(target)+".sshc-"+hex.EncodeToString(suffix[:])+".tmp"), nil
}
