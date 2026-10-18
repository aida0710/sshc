// Package enginelog owns the private, device-local background engine log.
package enginelog

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"sshc/internal/filelock"
	"sshc/internal/storage"
)

const DirectoryName = "logs"
const FileName = "engine.log"

// MaximumSize keeps the previous startup's diagnostics without unbounded growth across restarts.
const MaximumSize = 5 << 20

// TailLines is enough startup context for a CLI failure without flooding the terminal.
const TailLines = 20

// tailBytes also bounds diagnostics when a process writes a single very long line.
const tailBytes = 64 << 10

// rotationTimeout bounds contention between simultaneous, short log preparations.
const rotationTimeout = 5 * time.Second

func Path(stateDir string) string { return filepath.Join(stateDir, DirectoryName, FileName) }

// Open serializes rotation and writes a startup marker before the child inherits the file.
func Open(stateDir, version string) (*os.File, error) {
	path := Path(stateDir)
	release, err := filelock.AcquireWithin(path+".lock", rotationTimeout)
	if err != nil {
		return nil, err
	}
	defer release()
	file, err := openPrivateAppend(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if info.Size() > MaximumSize {
		if err := file.Close(); err != nil {
			return nil, err
		}
		if err := (storage.OSFileSystem{}).Rename(path, path+".1"); err != nil {
			return nil, err
		}
		file, err = openPrivateAppend(path)
		if err != nil {
			return nil, err
		}
	}
	if _, err := fmt.Fprintf(file, "=== sshc %s engine started in the background at %s ===\n", version, time.Now().Format(time.RFC3339)); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func Tail(path string) (string, error) {
	file, err := openPrivateRead(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	start := max(int64(0), info.Size()-tailBytes)
	contents, err := io.ReadAll(io.NewSectionReader(file, start, tailBytes))
	if err != nil {
		return "", err
	}
	lines := bytes.Split(bytes.TrimSuffix(contents, []byte("\n")), []byte("\n"))
	return string(bytes.Join(lines[max(0, len(lines)-TailLines):], []byte("\n"))), nil
}
