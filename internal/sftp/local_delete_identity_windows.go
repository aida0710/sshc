package sftp

import (
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func localDeletionIdentity(parent *os.Root, name string, _ fs.FileInfo) (string, error) {
	handle, err := openLocalWindowsEntry(parent, name, windows.FILE_READ_ATTRIBUTES)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	var identity windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &identity); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d:%d", identity.VolumeSerialNumber, identity.FileIndexHigh, identity.FileIndexLow), nil
}
