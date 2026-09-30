package sftp

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
)

// downloadSpoolReservationName is the file in a manager's spool directory that
// records how many bytes the manager reserved, for the other sshc processes of
// the user to count. It holds one big-endian uint64 and nothing else.
const downloadSpoolReservationName = ".reserved"

func readDownloadSpoolReservation(directory string) (int64, error) {
	file, err := openSpoolFileForRead(filepath.Join(directory, downloadSpoolReservationName))
	if err != nil {
		return 0, err
	}
	defer file.Close()
	var encoded [8]byte
	if _, err := io.ReadFull(file, encoded[:]); err != nil {
		return 0, err
	}
	var extra [1]byte
	if count, err := file.Read(extra[:]); err != io.EOF || count != 0 {
		if err != nil {
			return 0, err
		}
		return 0, os.ErrInvalid
	}
	value := binary.BigEndian.Uint64(encoded[:])
	if value > uint64(maxDownloadSpoolBytes) {
		return 0, os.ErrInvalid
	}
	return int64(value), nil
}

func writeDownloadSpoolReservation(directory string, reserved int64) error {
	if reserved < 0 || reserved > maxDownloadSpoolBytes {
		return os.ErrInvalid
	}
	file, err := openOrCreateSpoolFile(filepath.Join(directory, downloadSpoolReservationName))
	if err != nil {
		return err
	}
	defer file.Close()
	return storeDownloadSpoolReservation(file, reserved)
}

// storeDownloadSpoolReservation overwrites the eight bytes in place. Emptying
// the file first would leave it empty if the write then failed on a full disk.
func storeDownloadSpoolReservation(file *os.File, reserved int64) error {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(reserved))
	if _, err := file.WriteAt(encoded[:], 0); err != nil {
		return err
	}
	if err := file.Truncate(int64(len(encoded))); err != nil {
		return err
	}
	return file.Sync()
}
