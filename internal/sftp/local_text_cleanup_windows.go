package sftp

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func removeLocalTextStagingFile(_ *os.Root, staging localTextStaging) {
	if staging.published || staging.publicationFile == nil {
		return
	}
	_ = markWindowsLocalTextStagingForDeletion(windows.Handle(staging.publicationFile.Fd()))
}

func markWindowsLocalTextStagingForDeletion(handle windows.Handle) error {
	// FileDispositionInfo acts on the original file object, even if its name
	// was moved and replaced. Closing the final duplicate completes deletion.
	deleteFile := byte(1)
	return windows.SetFileInformationByHandle(handle, windows.FileDispositionInfo,
		&deleteFile, uint32(unsafe.Sizeof(deleteFile)))
}
