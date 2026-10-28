//go:build windows

package windowsacl

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"sshc/internal/platform/nativepath"
)

// この木で Windows のパスを、リンク（再解析点）をたどらずに歩く手順はここだけに置く。
// Unix で同じ役を持つのは platform/nofollow である。
//
// 歩き方: ドライブか共有の根を開き、そのハンドルを RootDirectory にして、各要素を
// OBJ_DONT_REPARSE 付きの NtCreateFile で1つずつ開く。開いたハンドルは検査した親の
// 連鎖に固定されるので、途中を junction に置き換えても別の場所へは誘導されない。

// RelativeOpen は、開いた親の下の名前をどう開くかである。
type RelativeOpen struct {
	Access uint32
	// Disposition は FILE_OPEN（あるものだけ）、FILE_CREATE（新しく作るだけ）、
	// FILE_OPEN_IF（無ければ作る）のどれかである。
	Disposition   uint32
	CreateOptions uint32
	// Private は、作るときから非公開の記述子を付ける（NewPrivateSecurityDescriptor）。
	// ディレクトリかどうかは CreateOptions の FILE_DIRECTORY_FILE で決まる。
	Private bool
}

// OpenVolumeRoot は、歩き始めるドライブか共有の根を開く。RootDirectory 相対の
// NtCreateFile を始めるには Win32 で開いたディレクトリが要る。SMB サーバーには
// OPEN_REPARSE_POINT の実装義務がないので、根には使わない。根より下の各要素には
// OBJ_DONT_REPARSE を使う。
func OpenVolumeRoot(root string, access uint32) (windows.Handle, error) {
	rootUTF16, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(
		rootUTF16,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
}

// OpenRelative は、parent の下の name を、再解析点をたどらずに開くか作る。
// 失敗は MapNoReparseError で直して返す。
func OpenRelative(parent windows.Handle, name string, open RelativeOpen) (windows.Handle, error) {
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
	if open.Private {
		descriptor, err := NewPrivateSecurityDescriptor(open.CreateOptions&windows.FILE_DIRECTORY_FILE != 0)
		if err != nil {
			return 0, err
		}
		attributes.SecurityDescriptor = descriptor
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(
		&handle,
		open.Access,
		attributes,
		&status,
		nil,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		open.Disposition,
		open.CreateOptions,
		0,
		0,
	)
	runtime.KeepAlive(attributes)
	if err != nil {
		return 0, MapNoReparseError(err)
	}
	return handle, nil
}

// OpenDirectoryNoReparse は、directory 自体を根から1要素ずつ開き、最後のディレクトリを
// finalAccess で開いたハンドルを返す。directory が根そのものなら、根を finalAccess で開く。
// directory は絶対パスであること。
func OpenDirectoryNoReparse(directory string, finalAccess uint32) (windows.Handle, error) {
	directory = filepath.Clean(directory)
	volume := filepath.VolumeName(directory)
	if volume == "" {
		return 0, os.ErrInvalid
	}
	root := volume + string(os.PathSeparator)
	relative, ok := nativepath.Relative(root, directory)
	if !ok {
		return 0, os.ErrInvalid
	}
	if relative == "." {
		return OpenVolumeRoot(root, finalAccess)
	}
	current, err := OpenVolumeRoot(root, windows.FILE_TRAVERSE)
	if err != nil {
		return 0, err
	}
	components := strings.Split(relative, string(os.PathSeparator))
	for index, component := range components {
		if component == "" || component == "." || component == ".." {
			_ = windows.CloseHandle(current)
			return 0, os.ErrInvalid
		}
		access := uint32(windows.FILE_TRAVERSE)
		if index == len(components)-1 {
			access = finalAccess
		}
		next, err := OpenRelative(current, component, RelativeOpen{Access: access, Disposition: windows.FILE_OPEN, CreateOptions: windows.FILE_DIRECTORY_FILE})
		_ = windows.CloseHandle(current)
		if err != nil {
			return 0, err
		}
		current = next
	}
	return current, nil
}

// MapNoReparseError は、Nt* が返す NTSTATUS を、この木の他の層が知っている形に直す。
// 再解析点に当たったものは ErrReparsePoint に、それ以外は Win32 の errno にする。
//
// NTSTATUS は fs.ErrNotExist に一致しない。「まだ無い」と「読めなかった」を errors.Is で
// 分ける呼び出し側が上に何十とあり、生の NTSTATUS を返せば、まだ置かれていないだけの
// ファイルが読み取りの失敗として扱われる。Unix 側は os.OpenFile の errno を返す。
func MapNoReparseError(err error) error {
	if errors.Is(err, windows.STATUS_STOPPED_ON_SYMLINK) ||
		errors.Is(err, windows.STATUS_REPARSE_POINT_ENCOUNTERED) ||
		errors.Is(err, windows.STATUS_IO_REPARSE_TAG_NOT_HANDLED) ||
		errors.Is(err, windows.ERROR_STOPPED_ON_SYMLINK) {
		return ErrReparsePoint
	}
	var status windows.NTStatus
	if errors.As(err, &status) {
		return status.Errno()
	}
	return err
}
