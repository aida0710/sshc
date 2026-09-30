//go:build !windows

package sftp

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestAFullDiskWhileSpoolingIsReportedAsSpoolFull(t *testing.T) {
	written := &os.PathError{Op: "write", Path: "download.part", Err: unix.ENOSPC}
	if err := spoolWriteError(written); !errors.Is(err, ErrSpoolFull) || !errors.Is(err, unix.ENOSPC) {
		t.Fatalf("spool write error = %v, want ErrSpoolFull wrapping ENOSPC", err)
	}
	remote := errors.New("remote read failed")
	if err := spoolWriteError(remote); err != remote {
		t.Fatalf("other error = %v, want it unchanged", err)
	}
}
