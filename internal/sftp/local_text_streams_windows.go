package sftp

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WIN32_STREAM_ID's fixed fields occupy 20 bytes, followed by the UTF-16
// name and data. Preserve that native record, including stream attributes.
const windowsLocalTextStreamHeaderBytes = 20

const (
	windowsLocalTextBackupData      = 1
	windowsLocalTextBackupExtended  = 2
	windowsLocalTextBackupAlternate = 4
	windowsLocalTextStreamModified  = 1
	windowsLocalTextStreamSparse    = 8
)

var (
	windowsLocalTextKernel      = windows.NewLazySystemDLL("kernel32.dll")
	windowsLocalTextBackupRead  = windowsLocalTextKernel.NewProc("BackupRead")
	windowsLocalTextBackupWrite = windowsLocalTextKernel.NewProc("BackupWrite")
	windowsLocalTextBackupSeek  = windowsLocalTextKernel.NewProc("BackupSeek")
)

type windowsLocalTextBackup struct {
	file      *os.File
	procedure *windows.LazyProc
	context   uintptr
}

func (backup *windowsLocalTextBackup) transfer(buffer []byte, abort bool) (int, error) {
	var pointer unsafe.Pointer
	if len(buffer) != 0 {
		pointer = unsafe.Pointer(&buffer[0])
	}
	var transferred uint32
	var stop uintptr
	if abort {
		stop = 1
	}
	ok, _, err := backup.procedure.Call(backup.file.Fd(), uintptr(pointer), uintptr(len(buffer)), uintptr(unsafe.Pointer(&transferred)),
		stop, 0, uintptr(unsafe.Pointer(&backup.context)))
	runtime.KeepAlive(buffer)
	runtime.KeepAlive(backup.file)
	if ok == 0 {
		return 0, err
	}
	return int(transferred), nil
}

func (backup *windowsLocalTextBackup) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	count, err := backup.transfer(buffer, false)
	if count == 0 && err == nil {
		return 0, io.EOF
	}
	return count, err
}

func (backup *windowsLocalTextBackup) Close() error {
	if backup.context == 0 {
		return nil
	}
	_, err := backup.transfer(nil, true)
	backup.context = 0
	return err
}

func (backup *windowsLocalTextBackup) skip(size uint64) error {
	if size == 0 {
		return nil
	}
	var low, high uint32
	ok, _, err := windowsLocalTextBackupSeek.Call(backup.file.Fd(), uintptr(uint32(size)), uintptr(uint32(size>>32)),
		uintptr(unsafe.Pointer(&low)), uintptr(unsafe.Pointer(&high)), uintptr(unsafe.Pointer(&backup.context)))
	runtime.KeepAlive(backup.file)
	if ok == 0 {
		return err
	}
	if uint64(low)|uint64(high)<<32 != size {
		return ErrConflict
	}
	return nil
}

func readWindowsLocalTextStreams(file *os.File) (streams map[string][]byte, err error) {
	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	backup := windowsLocalTextBackup{file: file, procedure: windowsLocalTextBackupRead}
	defer func() {
		err = errors.Join(err, backup.Close())
		_, restored := file.Seek(position, io.SeekStart)
		err = errors.Join(err, restored)
	}()
	streams = make(map[string][]byte)
	remaining := uint64(maxLocalTextMetadataBytes)
	for {
		var header [windowsLocalTextStreamHeaderBytes]byte
		if _, err := io.ReadFull(&backup, header[:]); err != nil {
			if errors.Is(err, io.EOF) {
				return streams, nil
			}
			return nil, err
		}
		kind := binary.LittleEndian.Uint32(header[:4])
		attributes := binary.LittleEndian.Uint32(header[4:8])
		size := binary.LittleEndian.Uint64(header[8:16])
		nameBytes := binary.LittleEndian.Uint32(header[16:20])
		if attributes&(windowsLocalTextStreamModified|windowsLocalTextStreamSparse) != 0 {
			return nil, ErrUnsupportedEntry
		}
		if kind == windowsLocalTextBackupData && nameBytes == 0 {
			if size > MaxEditableFileBytes {
				return nil, ErrTextTooLarge
			}
			if err := backup.skip(size); err != nil {
				return nil, err
			}
			continue
		}
		if kind != windowsLocalTextBackupExtended && kind != windowsLocalTextBackupAlternate {
			// Object IDs, hardlink topology, reparse data and transactional
			// streams cannot safely be cloned onto an ordinary sibling.
			return nil, ErrUnsupportedEntry
		}
		if nameBytes%windowsUTF16CodeUnitBytes != 0 || uint64(nameBytes)+windowsLocalTextStreamHeaderBytes > remaining {
			return nil, ErrUnsupportedEntry
		}
		headerBytes := uint64(nameBytes) + windowsLocalTextStreamHeaderBytes
		if size > remaining-headerBytes {
			return nil, ErrUnsupportedEntry
		}
		record := make([]byte, headerBytes+size)
		copy(record, header[:])
		if _, err := io.ReadFull(&backup, record[windowsLocalTextStreamHeaderBytes:]); err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%d:%s", kind, hex.EncodeToString(record[windowsLocalTextStreamHeaderBytes:headerBytes]))
		if _, duplicate := streams[key]; duplicate {
			return nil, ErrUnsupportedEntry
		}
		streams[key] = record
		remaining -= uint64(len(record))
	}
}

func writeWindowsLocalTextStreams(file *os.File, streams map[string][]byte) (err error) {
	backup := windowsLocalTextBackup{file: file, procedure: windowsLocalTextBackupWrite}
	defer func() { err = errors.Join(err, backup.Close()) }()
	names := make([]string, 0, len(streams))
	for name := range streams {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		record := streams[name]
		written, err := backup.transfer(record, false)
		if err != nil {
			return err
		}
		if written != len(record) {
			return io.ErrShortWrite
		}
	}
	return nil
}
