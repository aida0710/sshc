package storage

import (
	"os"
	"testing"
)

const mutationLockTestDirectoryEnvironment = "SSHC_STORAGE_TEST_LOCK_DIRECTORY"

func TestMain(main *testing.M) {
	directory := os.Getenv(mutationLockTestDirectoryEnvironment)
	owned := directory == ""
	if owned {
		var err error
		directory, err = os.MkdirTemp("", "sshc-storage-locks-")
		if err != nil {
			panic(err)
		}
		if err := os.Setenv(mutationLockTestDirectoryEnvironment, directory); err != nil {
			panic(err)
		}
	}
	workspaceMutationLockDirectory = func(*Workspace) (string, error) { return directory, nil }
	code := main.Run()
	if owned {
		_ = os.RemoveAll(directory)
	}
	os.Exit(code)
}
