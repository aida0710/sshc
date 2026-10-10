package sftp

import "os"

func secureLocalTextStagingDirectory(directory *os.File) error {
	if _, err := localTextStagingDirectoryInfo(directory); err != nil {
		return err
	}
	if err := setDarwinTextSecurity(directory, nil); err != nil {
		return err
	}
	if err := directory.Chmod(localTextStagingDirectoryPermission); err != nil {
		return err
	}
	return verifyLocalTextStagingDirectory(directory)
}

func verifyLocalTextStagingDirectory(directory *os.File) error {
	info, err := localTextStagingDirectoryInfo(directory)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != localTextStagingDirectoryPermission {
		return ErrConflict
	}
	security, err := readDarwinTextSecurity(directory)
	if err != nil {
		return err
	}
	if len(security) != 0 {
		return ErrConflict
	}
	return nil
}
