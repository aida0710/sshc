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
	published       bool
}

func (staging *localTextStaging) publish(parent *os.Root, target string) error {
	if err := publishLocalTextReplacement(parent, *staging, target); err != nil {
		return err
	}
	staging.published = true
	return nil
}

func (staging *localTextStaging) cleanup(parent *os.Root) {
	if staging.file != nil {
		staging.file.Close()
		staging.file = nil
	}
	// Windows needs its pinned creation handle until its own unpublished file
	// has been marked for deletion. Names may already refer to someone else's file.
	removeLocalTextStagingFile(parent, *staging)
	if staging.publicationFile != nil {
		staging.publicationFile.Close()
		staging.publicationFile = nil
	}
	if staging.directory != nil {
		staging.directory.Close()
	}
}
