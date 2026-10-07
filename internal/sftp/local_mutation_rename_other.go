//go:build (!linux && !darwin && !windows) || android

package sftp

import "os"

func renameLocalMutationInDirectory(directory *os.File, from, to string) error {
	// Refuse rather than risk replacing a destination created concurrently.
	return ErrUnsupportedEntry
}
