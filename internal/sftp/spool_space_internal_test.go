package sftp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSpoolSpaceCheckRefusesADownloadThatWouldFillTheDisk(t *testing.T) {
	const size = int64(1 << 30)
	for _, test := range []struct {
		name      string
		available uint64
		want      error
	}{
		{name: "room to spare", available: uint64(size) + spoolSpaceHeadroom, want: nil},
		{name: "fits without headroom", available: uint64(size) + spoolSpaceHeadroom - 1, want: ErrSpoolFull},
		{name: "smaller than the file", available: uint64(size) / 2, want: ErrSpoolFull},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := checkSpoolSpace(test.available, size); !errors.Is(err, test.want) {
				t.Fatalf("check = %v, want %v", err, test.want)
			}
		})
	}
}

func TestAnEmptyReservationFileCountsOnlyTheSpooledFiles(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "sshc-sftp-spool-current")
	if err := os.Mkdir(current, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := prepareDownloadSpoolDirectory(current); err != nil {
		t.Fatal(err)
	}
	// The file is opened as writeDownloadSpoolReservation opens it, so that on
	// Windows it carries the private ACL the reader requires; only the first
	// write is missing.
	reservation, err := openOrCreateSpoolFile(filepath.Join(current, downloadSpoolReservationName))
	if err != nil {
		t.Fatal(err)
	}
	if err := reservation.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "download.part"), []byte("spooled"), 0o600); err != nil {
		t.Fatal(err)
	}

	total, err := activeDownloadSpoolBytes(root)
	if err != nil {
		t.Fatalf("active bytes = %v", err)
	}
	if total != int64(len("spooled")) {
		t.Fatalf("active bytes = %d, want the spooled file only", total)
	}
}

func TestReservationRewriteKeepsEightBytes(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "sshc-sftp-spool-current")
	if err := os.Mkdir(current, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := prepareDownloadSpoolDirectory(current); err != nil {
		t.Fatal(err)
	}
	for _, reserved := range []int64{1 << 35, 3} {
		if err := writeDownloadSpoolReservation(current, reserved); err != nil {
			t.Fatal(err)
		}
		if got, err := readDownloadSpoolReservation(current); err != nil || got != reserved {
			t.Fatalf("reservation = %d, %v; want %d", got, err, reserved)
		}
	}
}

func TestReleaseGivesTheSpoolBackEvenWhenTheReservationFileCannotBeWritten(t *testing.T) {
	root := t.TempDir()
	notADirectory := filepath.Join(root, "sshc-sftp-spool-broken")
	if err := os.WriteFile(notADirectory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	spool := &downloadSpool{root: root, directory: notADirectory, reservedBytes: 10}

	spool.release(4)

	if remaining := reservedSpoolBytes(spool); remaining != 6 {
		t.Fatalf("reserved after release = %d, want 6", remaining)
	}
}
