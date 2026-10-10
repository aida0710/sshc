//go:build !windows

package sftp

import "os"

// The private parent also protects contents while mode and ACL are restored.
const (
	localTextStagingPermission          = 0o600
	localTextStagingDirectoryPermission = 0o700
	localTextStagingContentsName        = "contents"
)

func openLocalTextStagingFile(parent *os.Root, name string) (staging localTextStaging, err error) {
	if err := parent.Mkdir(name, localTextStagingDirectoryPermission); err != nil {
		return localTextStaging{}, err
	}
	staging.name = name
	defer func() {
		if err != nil {
			staging.cleanup(parent)
		}
	}()
	staging.directoryInfo, err = parent.Lstat(name)
	if err != nil {
		return staging, err
	}
	staging.directory, err = parent.OpenRoot(name)
	if err != nil {
		return staging, err
	}
	// A default ACL can remove execute permission from the newly created
	// directory, so open its name before traversing it through the pinned root.
	directoryFile, err := parent.Open(name)
	if err != nil {
		return staging, err
	}
	defer directoryFile.Close()
	openedDirectoryInfo, err := directoryFile.Stat()
	if err != nil {
		return staging, err
	}
	if !openedDirectoryInfo.IsDir() || !os.SameFile(staging.directoryInfo, openedDirectoryInfo) {
		return staging, ErrConflict
	}
	if err := verifyLocalTextStagingIdentity(parent, staging); err != nil {
		return staging, err
	}
	if err := secureLocalTextStagingDirectory(directoryFile); err != nil {
		return staging, err
	}
	if err := verifyLocalTextStagingParent(parent, staging); err != nil {
		return staging, err
	}
	staging.file, err = staging.directory.OpenFile(localTextStagingContentsName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, localTextStagingPermission)
	return staging, err
}

func publishLocalTextReplacement(parent *os.Root, staging localTextStaging, target string) error {
	if err := verifyLocalTextStagingParent(parent, staging); err != nil {
		return err
	}
	return renameLocalTextStagingContents(parent, staging, target)
}

func verifyLocalTextStagingParent(parent *os.Root, staging localTextStaging) error {
	if err := verifyLocalTextStagingIdentity(parent, staging); err != nil {
		return err
	}
	directoryFile, err := staging.directory.Open(".")
	if err != nil {
		return err
	}
	defer directoryFile.Close()
	openedDirectoryInfo, err := directoryFile.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(staging.directoryInfo, openedDirectoryInfo) {
		return ErrConflict
	}
	return verifyLocalTextStagingDirectory(directoryFile)
}

func verifyLocalTextStagingIdentity(parent *os.Root, staging localTextStaging) error {
	current, err := parent.Lstat(staging.name)
	if err != nil {
		return err
	}
	if staging.directoryInfo == nil || !current.IsDir() || !os.SameFile(staging.directoryInfo, current) {
		return ErrConflict
	}
	return nil
}

func removeLocalTextStagingFile(parent *os.Root, staging localTextStaging) {
	if staging.directory != nil {
		staging.directory.Remove(localTextStagingContentsName)
	}
	if verifyLocalTextStagingIdentity(parent, staging) == nil {
		parent.Remove(staging.name)
	}
}
