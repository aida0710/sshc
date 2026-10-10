//go:build unix

package sftp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// A regression to a blocking FIFO open must fail without hanging the suite.
const localFIFOOpenTestTimeout = time.Second

func TestLocalContentOpenRejectsAFIFOReplacementWithoutWaitingForAWriter(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "file")
	writeLocalMutationFile(t, filename, "regular")
	expected, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// Place the replacement after the observation used by the handle verifier.
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(filename, 0o600); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		opened, err := openLocalContentFile(root, "file")
		if err == nil {
			err = verifyLocalOpenedFile(opened, expected)
			opened.Close()
		}
		finished <- err
	}()
	timer := time.NewTimer(localFIFOOpenTestTimeout)
	defer timer.Stop()
	select {
	case err := <-finished:
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("FIFO replacement = %v; want conflict", err)
		}
	case <-timer.C:
		// Release a regressed blocking reader before failing, so no goroutine or
		// filesystem handle is left behind by the fixture itself.
		writer, err := unix.Open(filename, unix.O_RDWR|unix.O_NONBLOCK, 0)
		if err == nil {
			<-finished
			unix.Close(writer)
		}
		t.Fatal("opening the replacement FIFO waited for a writer")
	}
}
