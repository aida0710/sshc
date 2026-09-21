//go:build !windows

package nofollow

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"sshc/internal/platform/nativepath"
)

// OpenRegular は通常ファイルを、途中も最後も symlink をたどらずに読み取り用に開く。
func OpenRegular(path string) (*os.File, error) {
	return open(path, false)
}

// OpenDirectory はディレクトリを、途中も最後も symlink をたどらずに開く。
func OpenDirectory(path string) (*os.File, error) {
	return open(path, true)
}

// OpenOrCreateDirectory は path までのディレクトリを symlink をたどらずに開き、
// 無い階層は permission で作って、最後のディレクトリの descriptor を返す。
// 呼び手はこの descriptor を基準に openat で中のファイルを扱うことで、確認後に
// パスが差し替えられても別の場所へ誘導されない。
func OpenOrCreateDirectory(path string, permission fs.FileMode) (*os.File, error) {
	components, err := splitAbsolute(path)
	if err != nil {
		return nil, err
	}
	if len(components) == 0 {
		return nil, os.ErrInvalid
	}
	current, err := openWalkRoot()
	if err != nil {
		return nil, err
	}
	for index, component := range components {
		final := index == len(components)-1
		next, openErr := openWalkDirectoryAt(current, component, final)
		if errors.Is(openErr, unix.ENOENT) {
			if err := unix.Mkdirat(int(current.Fd()), component, uint32(permission.Perm())); err != nil && !errors.Is(err, unix.EEXIST) {
				_ = current.Close()
				return nil, err
			}
			next, openErr = openWalkDirectoryAt(current, component, final)
		}
		_ = current.Close()
		if openErr != nil {
			return nil, openErr
		}
		current = next
	}
	if err := current.Chmod(permission); err != nil {
		_ = current.Close()
		return nil, err
	}
	return current, nil
}

func open(path string, directory bool) (*os.File, error) {
	components, err := splitAbsolute(path)
	if err != nil {
		return nil, err
	}
	current, err := openWalkRoot()
	if err != nil {
		return nil, err
	}
	for index, component := range components {
		final := index == len(components)-1
		var next *os.File
		var openErr error
		if !final || directory {
			next, openErr = openWalkDirectoryAt(current, component, final)
		} else {
			next, openErr = openRegularAt(current, component)
		}
		_ = current.Close()
		if openErr != nil {
			return nil, openErr
		}
		current = next
	}
	return current, nil
}

// splitAbsolute は絶対パスを root からの要素に分ける。"" や "." や ".." は
// 歩けないので拒む。root そのものは空の列になる。
func splitAbsolute(path string) ([]string, error) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return nil, os.ErrInvalid
	}
	cleaned, err := nativepath.ResolveRootAlias(cleaned)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrSymlinkPath, path, err)
	}
	relative := strings.TrimPrefix(cleaned, string(filepath.Separator))
	if relative == "" {
		return nil, nil
	}
	components := strings.Split(relative, string(filepath.Separator))
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return nil, os.ErrInvalid
		}
	}
	return components, nil
}

func openWalkDirectoryAt(parent *os.File, component string, readable bool) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), component, walkDirectoryFlags(readable), 0)
	if err != nil {
		return nil, classifyOpenError(parent, component, err)
	}
	return fileFromDescriptor(fd, component)
}

func openRegularAt(parent *os.File, component string) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), component, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, classifyOpenError(parent, component, err)
	}
	return fileFromDescriptor(fd, component)
}

func fileFromDescriptor(fd int, name string) (*os.File, error) {
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, os.ErrInvalid
	}
	return file, nil
}

func classifyOpenError(parent *os.File, component string, openErr error) error {
	if errors.Is(openErr, unix.ELOOP) {
		return ErrSymlinkPath
	}
	// Linux の O_PATH|O_NOFOLLOW|O_DIRECTORY は symlink を ENOTDIR として返す。
	// たどらずに現在の entry 型だけを確認し、公開 error の分類を保つ。
	if errors.Is(openErr, unix.ENOTDIR) {
		var stat unix.Stat_t
		if err := unix.Fstatat(int(parent.Fd()), component, &stat, unix.AT_SYMLINK_NOFOLLOW); err == nil && stat.Mode&unix.S_IFMT == unix.S_IFLNK {
			return ErrSymlinkPath
		}
	}
	return openErr
}
