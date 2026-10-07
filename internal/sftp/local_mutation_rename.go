package sftp

import (
	"errors"
	"io/fs"
	"os"
)

func renameLocalMutationWithoutReplace(parent *os.Root, from, to string) error {
	directory, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	err = renameLocalMutationInDirectory(directory, from, to)
	if errors.Is(err, fs.ErrExist) {
		return ErrAlreadyExists
	}
	return err
}
