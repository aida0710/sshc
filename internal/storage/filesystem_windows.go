//go:build windows

package storage

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/windowsacl"
)

const fileDeleteChild = 0x00000040

func makePrivateDirectories(path string, permission fs.FileMode) error {
	return windowsacl.EnsureDirectory(path)
}

func createPrivateTemp(directory, prefix string) (*os.File, error) {
	return windowsacl.CreateTemp(directory, prefix)
}

func openRegularNoFollow(path string) (*os.File, error) {
	absolutePath, err := cleanAbsoluteDOSPath(path)
	if err != nil {
		return nil, err
	}
	parent, err := openNoReparseDirectory(filepath.Dir(absolutePath))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(parent)

	fileName := filepath.Base(absolutePath)
	if fileName == "." || fileName == string(os.PathSeparator) {
		return nil, os.ErrInvalid
	}
	// 読むだけでも FILE_READ_ATTRIBUTES を要る。readBoundedRegularFile は
	// **通常ファイルであることを handle から確かめてから**中身を読むためだ。
	// FILE_READ_DATA だけで開くと、その確認が Access is denied で落ち、
	// ユーザーの ~/.ssh/config がひとつも読めない。
	handle, err := openRelativeNoReparse(parent, fileName, windows.FILE_READ_DATA|windows.FILE_READ_ATTRIBUTES, false, true)
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

// ReadPrivateFile authenticates the same final handle that supplies private
// state bytes. User-managed SSH files continue through ReadFile instead.
func (OSFileSystem) ReadPrivateFile(path string) ([]byte, error) {
	file, err := windowsacl.OpenAuthenticatedFileForRead(path)
	if err != nil {
		return nil, mapPrivateOpenError(err)
	}
	defer file.Close()
	return readBoundedRegularFile(file)
}

func mapPrivateOpenError(err error) error {
	if errors.Is(err, windowsacl.ErrReparsePoint) {
		return ErrSymlinkPath
	}
	return err
}

// openNoReparseDirectory は渡された directory 自体を handle 相対でたどる。
// OBJ_DONT_REPARSE を各 component の name resolution に渡すため、検査後の置換で
// 別 tree をたどれない。呼び出し側が file path を渡す関数ではない。
func openNoReparseDirectory(directory string) (windows.Handle, error) {
	return openNoReparseDirectoryWithAccess(directory, windows.FILE_TRAVERSE)
}

func openNoReparseDirectoryWithAccess(directory string, finalAccess uint32) (windows.Handle, error) {
	volume := filepath.VolumeName(directory)
	if volume == "" {
		return 0, os.ErrInvalid
	}
	root := volume + string(os.PathSeparator)
	relativeParent, err := filepath.Rel(root, directory)
	if err != nil || filepath.IsAbs(relativeParent) || relativeParent == ".." || strings.HasPrefix(relativeParent, ".."+string(os.PathSeparator)) {
		return 0, os.ErrInvalid
	}

	rootAccess := uint32(windows.FILE_TRAVERSE)
	if relativeParent == "." {
		rootAccess = finalAccess
	}
	current, err := openAbsoluteNoReparseDirectory(root, rootAccess)
	if err != nil {
		return 0, err
	}
	if relativeParent == "." {
		return current, nil
	}
	components := strings.Split(relativeParent, string(os.PathSeparator))
	for index, component := range components {
		if component == "" || component == "." {
			continue
		}
		access := uint32(windows.FILE_TRAVERSE)
		if index == len(components)-1 {
			access = finalAccess
		}
		next, err := openRelativeNoReparse(current, component, access, true, false)
		if err != nil {
			_ = windows.CloseHandle(current)
			return 0, err
		}
		_ = windows.CloseHandle(current)
		current = next
	}
	return current, nil
}

// openAbsoluteNoReparseDirectory は volume/share root を開く。RootDirectory 相対
// NtCreateFile を始めるため Win32 の directory open が必要だが、root 自体は入力の
// lexical parent ではないため FILE_TRAVERSE だけを要求する。
func openAbsoluteNoReparseDirectory(path string, access uint32) (windows.Handle, error) {
	pathUTF16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	handle, err := windows.CreateFile(
		pathUTF16,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return 0, err
	}
	return handle, nil
}

// openRelativeNoReparse は既に検査済みの parent handle を RootDirectory に渡す。
// これにより parent path の名前空間が後から junction 等へ置換されても、開く
// object は保持中の親 directory に固定される。
func openRelativeNoReparse(parent windows.Handle, name string, access uint32, directory, synchronous bool) (windows.Handle, error) {
	return openRelativeNoReparseWithOptions(parent, name, access, directory, synchronous, 0)
}

func openRelativeNoReparseWithOptions(parent windows.Handle, name string, access uint32, directory, synchronous bool, createOptions uint32) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	options := createOptions
	if directory {
		options |= windows.FILE_DIRECTORY_FILE
	}
	if synchronous {
		access |= windows.SYNCHRONIZE
		options |= windows.FILE_SYNCHRONOUS_IO_NONALERT
	}
	err = windows.NtCreateFile(
		&handle,
		access,
		attributes,
		&status,
		nil,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		options,
		0,
		0,
	)
	if err != nil {
		return 0, mapNtError(err)
	}
	return handle, nil
}

// mapNtError は、Nt* が返す NTSTATUS を、この木の他の層が知っている形に直す。
//
// **NTSTATUS は fs.ErrNotExist に一致しない。** 「まだ無い」と「読めなかった」を
// errors.Is で分ける呼び出し側が上に何十とあり、生の NTSTATUS を返すと、存在
// しないだけの ~/.ssh/config が読み取り失敗として扱われる。Unix 側は
// os.OpenFile の errno を返しているので、こちらも Win32 の errno へ揃える。
func mapNtError(err error) error {
	if errors.Is(err, windows.STATUS_STOPPED_ON_SYMLINK) ||
		errors.Is(err, windows.STATUS_REPARSE_POINT_ENCOUNTERED) ||
		errors.Is(err, windows.ERROR_STOPPED_ON_SYMLINK) {
		return ErrSymlinkPath
	}
	var status windows.NTStatus
	if errors.As(err, &status) {
		return status.Errno()
	}
	return err
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
	oldUTF16, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	newUTF16, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(oldUTF16, newUTF16, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

type fileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

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

	sourceParent, err := openNoReparseDirectory(filepath.Dir(oldAbsolute))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(sourceParent)
	sourceHandle, err := openRelativeNoReparseWithOptions(
		sourceParent,
		filepath.Base(oldAbsolute),
		windows.DELETE|windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_ATTRIBUTES,
		false,
		true,
		// The rename must retain the write-through durability intent of the
		// former MoveFileEx adapter without returning to a path-based source.
		windows.FILE_WRITE_THROUGH,
	)
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

	destinationParent, err := openNoReparseDirectoryWithAccess(
		filepath.Dir(newAbsolute),
		windows.FILE_TRAVERSE|windows.FILE_WRITE_DATA|fileDeleteChild,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(destinationParent)
	return renamePrivateFileHandle(sourceHandle, destinationParent, filepath.Base(newAbsolute))
}

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
	information.ReplaceIfExists = 1
	information.RootDirectory = destinationParent
	information.FileNameLength = uint32(nameLength)
	copy(unsafe.Slice(&information.FileName[0], nameLength/2), nameUTF16[:len(nameUTF16)-1])
	status := windows.IO_STATUS_BLOCK{}
	return mapNtError(windows.NtSetInformationFile(
		source,
		&status,
		&buffer[0],
		uint32(bufferSize),
		windows.FileRenameInformation,
	))
}

func syncDirectory(path string) error {
	// Windows には Unix の directory fsync に相当する API がない。このため
	// WriteTemp の file.Sync、replaceFile の MOVEFILE_WRITE_THROUGH、および
	// MovePrivate の write-through source handle を永続化境界にし、ここでは
	// それ以上の永続性を装わない。
	return nil
}
