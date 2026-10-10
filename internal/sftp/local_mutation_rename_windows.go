package sftp

import (
	"errors"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// FILE_RENAME_INFORMATION counts UTF-16 names in bytes.
const windowsUTF16CodeUnitBytes = 2

// windowsLocalRenameInformation is the native FILE_RENAME_INFORMATION layout.
// ReplaceIfExists stays zero, so a racing destination is never overwritten.
type windowsLocalRenameInformation struct {
	ReplaceIfExists byte
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func renameLocalMutationInDirectory(directory *os.File, from, to string) error {
	sourceHandle, err := openLocalWindowsEntryInDirectory(directory, from, windows.DELETE)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(sourceHandle)
	parentHandle := windows.Handle(directory.Fd())
	name, err := windows.UTF16FromString(to)
	if err != nil {
		return err
	}
	var header windowsLocalRenameInformation
	nameBytes := (len(name) - 1) * windowsUTF16CodeUnitBytes
	buffer := make([]byte, int(unsafe.Offsetof(header.FileName))+nameBytes)
	rename := (*windowsLocalRenameInformation)(unsafe.Pointer(&buffer[0]))
	rename.RootDirectory = parentHandle
	rename.FileNameLength = uint32(nameBytes)
	copy(unsafe.Slice(&rename.FileName[0], len(name)-1), name[:len(name)-1])
	var status windows.IO_STATUS_BLOCK
	return localWindowsFileError(windows.NtSetInformationFile(sourceHandle, &status, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation))
}

// Open relative to the pinned directory, including the reparse point itself.
func openLocalWindowsEntry(parent *os.Root, name string, access uint32) (windows.Handle, error) {
	directory, err := parent.Open(".")
	if err != nil {
		return 0, err
	}
	defer directory.Close()
	return openLocalWindowsEntryInDirectory(directory, name, access)
}

func openLocalWindowsEntryInDirectory(directory *os.File, name string, access uint32) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(directory.Fd()), ObjectName: objectName, Attributes: windows.OBJ_CASE_INSENSITIVE}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var sourceHandle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&sourceHandle, access|windows.SYNCHRONIZE, &attributes, &status, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return 0, localWindowsFileError(err)
	}
	return sourceHandle, nil
}

func localWindowsFileError(err error) error {
	if status, ok := err.(windows.NTStatus); ok {
		err = status.Errno()
	}
	if errors.Is(err, fs.ErrExist) {
		return ErrAlreadyExists
	}
	return err
}
