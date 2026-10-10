package sftp

import (
	"io/fs"
	"os"
)

// Unix keeps replacement contents in a pinned private directory until their
// original access policy is complete. Windows uses a private single-file DACL.
type localTextStaging struct {
	file *os.File
	// Windows publishes through a duplicate of the creation handle so closing
	// the writer cannot make a later pathname lookup pick a replacement file.
	publicationFile *os.File
	directory       *os.Root
	directoryInfo   fs.FileInfo
	name            string
}

func (staging localTextStaging) cleanup(parent *os.Root) {
	if staging.file != nil {
		staging.file.Close()
	}
	if staging.publicationFile != nil {
		staging.publicationFile.Close()
	}
	removeLocalTextStagingFile(parent, staging)
	if staging.directory != nil {
		staging.directory.Close()
	}
}
