//go:build windows

package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

const fileDeleteChild = 0x00000040

// nativeReportsExecutableBit: Go の Windows 実装は通常ファイルの権限を読み取り専用属性
// から 0666 か 0444 として作るので、owner の実行ビットは読み返せない。
const nativeReportsExecutableBit = false

const (
	tempRandomByteCount = 16
	tempCollisionLimit  = 128
)

func makePrivateDirectories(path string, permission fs.FileMode) error {
	absolute, err := cleanAbsoluteDOSPath(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	root := volume + string(os.PathSeparator)
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == "." || filepath.IsAbs(relative) {
		return os.ErrInvalid
	}
	current, err := windowsacl.OpenVolumeRoot(root, windows.FILE_TRAVERSE)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(current) }()
	components := strings.Split(relative, string(os.PathSeparator))
	for index, component := range components {
		if component == "" || component == "." || component == ".." {
			return os.ErrInvalid
		}
		final := index == len(components)-1
		access := uint32(windows.FILE_TRAVERSE)
		if final {
			access |= windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL | windows.WRITE_DAC
		}
		existing := windowsacl.RelativeOpen{Access: access, Disposition: windows.FILE_OPEN, CreateOptions: windows.FILE_DIRECTORY_FILE}
		next, openErr := windowsacl.OpenRelative(current, component, existing)
		created := false
		if errors.Is(openErr, fs.ErrNotExist) {
			next, openErr = windowsacl.OpenRelative(current, component, privateDirectory)
			created = openErr == nil
			if errors.Is(openErr, windows.ERROR_FILE_EXISTS) || errors.Is(openErr, windows.ERROR_ALREADY_EXISTS) {
				next, openErr = windowsacl.OpenRelative(current, component, existing)
				created = false
			}
		}
		if openErr != nil {
			return openErr
		}
		if created || final {
			if err := windowsacl.RestrictDirectoryNativeHandle(next); err != nil {
				_ = windows.CloseHandle(next)
				return err
			}
		}
		_ = windows.CloseHandle(current)
		current = next
	}
	return nil
}

// privateDirectory は、makePrivateDirectories が足りない要素を作るときの開き方である。
// 作るときから非公開にする。
var privateDirectory = windowsacl.RelativeOpen{
	Access:        windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL | windows.WRITE_DAC,
	Disposition:   windows.FILE_CREATE,
	CreateOptions: windows.FILE_DIRECTORY_FILE,
	Private:       true,
}

// privateTemporaryFile は、一時ファイルを作るときの開き方である。作るときから非公開にし、
// 名前が既にあれば作らずに失敗する。
var privateTemporaryFile = windowsacl.RelativeOpen{
	Access:        windows.FILE_WRITE_DATA | windows.FILE_WRITE_ATTRIBUTES | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL | windows.WRITE_DAC | windows.DELETE | windows.SYNCHRONIZE,
	Disposition:   windows.FILE_CREATE,
	CreateOptions: windows.FILE_NON_DIRECTORY_FILE | windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_WRITE_THROUGH,
	Private:       true,
}

func createPrivateTemp(directory, prefix string) (*os.File, error) {
	absolute, err := cleanAbsoluteDOSPath(directory)
	if err != nil {
		return nil, err
	}
	parent, err := windowsacl.OpenDirectoryNoReparse(absolute, windows.FILE_TRAVERSE|windows.FILE_WRITE_DATA|fileDeleteChild)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(parent)
	return createPrivateTempRelative(parent, absolute, prefix)
}

func createPrivateTempRelative(parent windows.Handle, directory, prefix string) (*os.File, error) {
	if prefix != filepath.Base(prefix) || strings.ContainsRune(prefix, os.PathSeparator) {
		return nil, os.ErrInvalid
	}
	randomBytes := make([]byte, tempRandomByteCount)
	defer clear(randomBytes)
	for attempt := 0; attempt < tempCollisionLimit; attempt++ {
		if _, err := rand.Read(randomBytes); err != nil {
			return nil, err
		}
		name := prefix + hex.EncodeToString(randomBytes)
		handle, err := windowsacl.OpenRelative(parent, name, privateTemporaryFile)
		if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			continue
		}
		if err != nil {
			return nil, err
		}
		file := os.NewFile(uintptr(handle), filepath.Join(directory, name))
		if file == nil {
			_ = discardFileHandle(handle)
			_ = windows.CloseHandle(handle)
			return nil, os.ErrInvalid
		}
		if err := windowsacl.RestrictFileHandle(file); err != nil {
			_ = discardFileHandle(handle)
			_ = file.Close()
			return nil, err
		}
		return file, nil
	}
	return nil, fmt.Errorf("create private Windows temp: collision limit exceeded")
}

func openRegularNoFollow(path string) (*os.File, error) {
	absolutePath, err := cleanAbsoluteDOSPath(path)
	if err != nil {
		return nil, err
	}
	parent, err := windowsacl.OpenDirectoryNoReparse(filepath.Dir(absolutePath), windows.FILE_TRAVERSE)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(parent)

	fileName := filepath.Base(absolutePath)
	if fileName == "." || fileName == string(os.PathSeparator) {
		return nil, os.ErrInvalid
	}
	// 読むだけでも FILE_READ_ATTRIBUTES を要る。readBoundedRegularFile は
	// 通常ファイルであることを handle から確かめてから中身を読むためだ。
	// FILE_READ_DATA だけで開くと、その確認が Access is denied で落ち、
	// ユーザーの ~/.ssh/config がひとつも読めない。
	handle, err := windowsacl.OpenRelative(parent, fileName, windowsacl.RelativeOpen{
		Access:        windows.FILE_READ_DATA | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE,
		Disposition:   windows.FILE_OPEN,
		CreateOptions: windows.FILE_SYNCHRONOUS_IO_NONALERT,
	})
	if err != nil {
		return nil, err
	}
	closeHandle := true
	defer func() {
		if closeHandle {
			_ = windows.CloseHandle(handle)
		}
	}()

	file := os.NewFile(uintptr(handle), absolutePath)
	if file == nil {
		return nil, os.ErrInvalid
	}
	closeHandle = false
	return file, nil
}

// ReadPrivateFile は非公開状態を読み出す最終ハンドル自体を検証する。ユーザー管理の
// SSH ファイルは引き続き ReadFile で読む。
func (OSFileSystem) ReadPrivateFile(path string) ([]byte, error) {
	return (OSFileSystem{}).ReadPrivateFileLimited(path, MaxFileSize)
}

func (OSFileSystem) ReadPrivateFileLimited(path string, maximum int64) ([]byte, error) {
	if maximum < 0 {
		return nil, ErrFileTooLarge
	}
	file, err := windowsacl.OpenAuthenticatedFileForRead(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readBoundedRegularFile(file, maximum)
}

func (OSFileSystem) DigestPrivateFileLimited(path string, maximum int64) (string, error) {
	if maximum < 0 {
		return "", ErrFileTooLarge
	}
	file, err := windowsacl.OpenAuthenticatedFileForRead(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return digestBoundedRegularFile(file, maximum)
}

func (OSFileSystem) ReadPrivateFilePrefix(path string, maximum int) ([]byte, error) {
	file, err := windowsacl.OpenAuthenticatedFileForRead(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readRegularFilePrefix(file, maximum)
}

func cleanAbsoluteDOSPath(path string) (string, error) {
	if err := windowsacl.ValidatePrivatePath(path); err != nil {
		return "", err
	}
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 260)
	for {
		n, err := windows.GetFullPathName(pathUTF16, uint32(len(buffer)), &buffer[0], nil)
		if err != nil {
			return "", err
		}
		if n < uint32(len(buffer)) {
			return normalizeDOSPath(windows.UTF16ToString(buffer))
		}
		buffer = make([]uint16, n+1)
	}
}

func normalizeDOSPath(path string) (string, error) {
	if err := windowsacl.ValidatePrivatePath(path); err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) || filepath.VolumeName(path) == "" {
		return "", os.ErrInvalid
	}
	return path, nil
}

func replaceFile(oldPath, newPath string) error {
	oldAbsolute, err := cleanAbsoluteDOSPath(oldPath)
	if err != nil {
		return err
	}
	newAbsolute, err := cleanAbsoluteDOSPath(newPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.VolumeName(oldAbsolute), filepath.VolumeName(newAbsolute)) {
		return windows.ERROR_NOT_SAME_DEVICE
	}
	sourceParent, err := windowsacl.OpenDirectoryNoReparse(filepath.Dir(oldAbsolute), windows.FILE_TRAVERSE)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(sourceParent)
	source, err := windowsacl.OpenRelative(sourceParent, filepath.Base(oldAbsolute), windowsacl.RelativeOpen{
		Access:        windows.DELETE | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE,
		Disposition:   windows.FILE_OPEN,
		CreateOptions: windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_WRITE_THROUGH,
	})
	if err != nil {
		return err
	}
	defer windows.CloseHandle(source)

	destinationParent, err := windowsacl.OpenDirectoryNoReparse(
		filepath.Dir(newAbsolute),
		windows.FILE_TRAVERSE|windows.FILE_WRITE_DATA|fileDeleteChild,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(destinationParent)
	return renamePrivateFileHandle(source, destinationParent, filepath.Base(newAbsolute))
}

// fileRenameInformation は FILE_RENAME_INFORMATION と FILE_RENAME_INFORMATION_EX の両方の形である。
// 先頭は ReplaceIfExists（BOOLEAN）と Flags（ULONG）の共用体で、FileRenameInformation は
// 最初の1バイトだけを読む。
type fileRenameInformation struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [1]uint16
}

// fileRenameInformationEx は FILE_INFORMATION_CLASS の FileRenameInformationEx である。
// golang.org/x/sys/windows に定数が無い。
const fileRenameInformationEx = 65

func movePrivateFile(oldPath, newPath string) error {
	return movePrivateFileWith(oldPath, newPath, nil)
}

func movePrivateFileWith(oldPath, newPath string, afterAuthenticate func(*os.File) error) error {
	oldAbsolute, err := cleanAbsoluteDOSPath(oldPath)
	if err != nil {
		return err
	}
	newAbsolute, err := cleanAbsoluteDOSPath(newPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.VolumeName(oldAbsolute), filepath.VolumeName(newAbsolute)) {
		return windows.ERROR_NOT_SAME_DEVICE
	}

	sourceParent, err := windowsacl.OpenDirectoryNoReparse(filepath.Dir(oldAbsolute), windows.FILE_TRAVERSE)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(sourceParent)
	sourceHandle, err := windowsacl.OpenRelative(sourceParent, filepath.Base(oldAbsolute), windowsacl.RelativeOpen{
		Access:      windows.DELETE | windows.READ_CONTROL | windows.WRITE_DAC | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE,
		Disposition: windows.FILE_OPEN,
		// パス指定の移動へ戻さず、MoveFileEx と同じ write-through の永続性を維持する。
		CreateOptions: windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_WRITE_THROUGH,
	})
	if err != nil {
		return err
	}
	source := os.NewFile(uintptr(sourceHandle), oldAbsolute)
	if source == nil {
		_ = windows.CloseHandle(sourceHandle)
		return os.ErrInvalid
	}
	defer source.Close()
	if err := windowsacl.RestrictFileHandle(source); err != nil {
		return err
	}
	if afterAuthenticate != nil {
		if err := afterAuthenticate(source); err != nil {
			return err
		}
	}

	destinationParent, err := windowsacl.OpenDirectoryNoReparse(
		filepath.Dir(newAbsolute),
		windows.FILE_TRAVERSE|windows.FILE_WRITE_DATA|fileDeleteChild,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(destinationParent)
	return renamePrivateFileHandle(sourceHandle, destinationParent, filepath.Base(newAbsolute))
}

// renamePrivateFileHandle は、source を destinationParent の下の destinationName へ移し、
// 同じ名前のファイルがあれば置き換える。
//
// 従来の FileRenameInformation は、置き換える先を誰かが開いていると、相手が削除の共有を
// 許していても Access is denied で断る。保存のあいだにも、画面の読み直しや Windows Defender の
// 検査が metadata.json などを開くので、それだけでは設定の保存がその時々で断られる。そこで
// Access is denied のときは、POSIX の置き換え（FILE_RENAME_POSIX_SEMANTICS）でやり直す。開いている
// 人は前の中身を読み続け、名前は新しいファイルを指す。
//
// POSIX の置き換えを先に使わないのは、古い Windows やファイルシステム（FAT、exFAT、SMB の共有など）
// が持たず、そのとき何で断るかが決まっていないためである。Rust の std::fs::rename と同じく、
// やり直しも通らなければ最初の理由を返す。
func renamePrivateFileHandle(source, destinationParent windows.Handle, destinationName string) error {
	nameUTF16, err := windows.UTF16FromString(destinationName)
	if err != nil {
		return err
	}
	nameLength := (len(nameUTF16) - 1) * 2
	var layout fileRenameInformation
	bufferSize := int(unsafe.Offsetof(layout.FileName)) + nameLength
	buffer := make([]byte, bufferSize)
	information := (*fileRenameInformation)(unsafe.Pointer(&buffer[0]))
	information.Flags = 1 // ReplaceIfExists
	information.RootDirectory = destinationParent
	information.FileNameLength = uint32(nameLength)
	copy(unsafe.Slice(&information.FileName[0], nameLength/2), nameUTF16[:len(nameUTF16)-1])
	status := windows.IO_STATUS_BLOCK{}
	renameError := windowsacl.MapNoReparseError(windows.NtSetInformationFile(
		source,
		&status,
		&buffer[0],
		uint32(bufferSize),
		windows.FileRenameInformation,
	))
	if !errors.Is(renameError, windows.ERROR_ACCESS_DENIED) {
		return renameError
	}
	information.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	if windows.NtSetInformationFile(source, &status, &buffer[0], uint32(bufferSize), fileRenameInformationEx) != nil {
		return renameError
	}
	return nil
}

type fileDispositionInformation struct {
	DeleteFile byte
}

func discardFileHandle(handle windows.Handle) error {
	information := fileDispositionInformation{DeleteFile: 1}
	status := windows.IO_STATUS_BLOCK{}
	return windowsacl.MapNoReparseError(windows.NtSetInformationFile(
		handle,
		&status,
		(*byte)(unsafe.Pointer(&information)),
		uint32(unsafe.Sizeof(information)),
		windows.FileDispositionInformation,
	))
}

func removeNoFollow(path string) error {
	absolute, err := cleanAbsoluteDOSPath(path)
	if err != nil {
		return err
	}
	parent, err := windowsacl.OpenDirectoryNoReparse(filepath.Dir(absolute), windows.FILE_TRAVERSE|fileDeleteChild)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	handle, err := windowsacl.OpenRelative(parent, filepath.Base(absolute), windowsacl.RelativeOpen{
		Access:        windows.DELETE | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE,
		Disposition:   windows.FILE_OPEN,
		CreateOptions: windows.FILE_SYNCHRONOUS_IO_NONALERT,
	})
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return discardFileHandle(handle)
}

func syncDirectory(path string) error {
	// Windows には Unix の directory fsync に相当する API がない。このため
	// WriteTemp の file.Sync、handle-relative rename の write-through、および
	// MovePrivate の write-through source handle を永続化境界にし、ここでは
	// それ以上の永続性を装わない。ただし path の再解析点を追わずに開き、呼出し側が
	// 検証した directory と別の場所を同期したことにしない。
	directory, err := windowsacl.OpenDirectoryNoReparse(path, windows.FILE_TRAVERSE)
	if err != nil {
		return err
	}
	return windows.CloseHandle(directory)
}

// cleanAtomicTargetは、WriteAtomicFileの書き込み先を、ボリュームを持つ絶対パスに均す。
// デバイスのパス（`\\?\`など）や、代替データストリームの名前（`:`を含むもの）は断る
// （cleanAbsoluteDOSPath）。
func cleanAtomicTarget(path string) (string, error) {
	return cleanAbsoluteDOSPath(path)
}
