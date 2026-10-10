package sftp

import (
	"os"

	"golang.org/x/sys/windows"
)

func publishLocalTextReplacement(parent *os.Root, staging localTextStaging, target string) error {
	directory, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	if staging.publicationFile == nil {
		return ErrConflict
	}
	source := windows.Handle(staging.publicationFile.Fd())
	var metadata windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(source, &metadata); err != nil {
		return err
	}
	if metadata.FileAttributes & ^uint32(windowsLocalTextOrdinaryAttributes) != 0 {
		return ErrUnsupportedEntry
	}
	// The source file and destination parent stay pinned. ReplaceFileW would
	// reopen absolute paths, letting a renamed ancestor redirect publication.
	return renameLocalWindowsHandleInDirectory(directory, windowsLocalRenameRequest{
		source: source, destinationName: target, replaceExisting: true,
	})
}
