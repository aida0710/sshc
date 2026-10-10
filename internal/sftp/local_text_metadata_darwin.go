package sftp

import (
	"encoding/binary"
	"io/fs"
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Darwin attribute buffers contain a length, an attrreference, and a variable
// kauth_filesec. Keep the native ACL bytes rather than resolving account names.
const (
	darwinAttributeBitmapCount    = 5
	darwinAttributeLengthBytes    = 4
	darwinAttributeReferenceBytes = 8
	darwinFileSecurityHeaderBytes = 44
	darwinFileSecurityMagic       = 0x012cc16d
	darwinNoACL                   = ^uint32(0)
)

func captureLocalTextMetadata(file *os.File) (localTextMetadata, error) {
	info, err := file.Stat()
	if err != nil {
		return localTextMetadata{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() {
		return localTextMetadata{}, ErrUnsupportedEntry
	}
	attributes, err := readLocalTextExtendedAttributes(file)
	if err != nil {
		return localTextMetadata{}, err
	}
	security, err := readDarwinTextSecurity(file)
	if err != nil {
		return localTextMetadata{}, err
	}
	retainedBytes := len(security)
	for name, value := range attributes {
		retainedBytes += len(name) + len(value) + 1
	}
	if retainedBytes > maxLocalTextMetadataBytes {
		return localTextMetadata{}, ErrUnsupportedEntry
	}
	// File flags also affect access (immutable/append-only) and must participate
	// in conflict detection. Such files cannot be replaced by the text editor.
	platform := make([]byte, darwinAttributeLengthBytes+len(security))
	binary.LittleEndian.PutUint32(platform, stat.Flags)
	copy(platform[darwinAttributeLengthBytes:], security)
	metadata := localTextMetadata{uid: int(stat.Uid), gid: int(stat.Gid), mode: info.Mode(), extendedAttributes: attributes, platformMetadata: platform}
	metadata.revision = localTextMetadataRevision(metadata)
	return metadata, nil
}

func readDarwinTextSecurity(file *os.File) ([]byte, error) {
	attributes := unix.Attrlist{Bitmapcount: darwinAttributeBitmapCount, Commonattr: unix.ATTR_CMN_EXTENDED_SECURITY}
	buffer := make([]byte, darwinAttributeLengthBytes+darwinAttributeReferenceBytes)
	if err := darwinTextAttributes(file, darwinTextAttributeRequest{operation: unix.SYS_FGETATTRLIST, attributes: &attributes, buffer: buffer, options: unix.FSOPT_REPORT_FULLSIZE}); err != nil {
		return nil, err
	}
	required := int(binary.LittleEndian.Uint32(buffer))
	if required < len(buffer) || required > maxLocalTextMetadataBytes {
		return nil, ErrUnsupportedEntry
	}
	buffer = make([]byte, required)
	if err := darwinTextAttributes(file, darwinTextAttributeRequest{operation: unix.SYS_FGETATTRLIST, attributes: &attributes, buffer: buffer, options: unix.FSOPT_REPORT_FULLSIZE}); err != nil {
		return nil, err
	}
	if int(binary.LittleEndian.Uint32(buffer)) != len(buffer) {
		return nil, ErrConflict
	}
	reference := buffer[darwinAttributeLengthBytes:]
	offset := int(int32(binary.LittleEndian.Uint32(reference)))
	length := int(binary.LittleEndian.Uint32(reference[darwinAttributeLengthBytes:]))
	if length == 0 {
		return nil, nil
	}
	start := darwinAttributeLengthBytes + offset
	if offset < darwinAttributeReferenceBytes || length < darwinFileSecurityHeaderBytes || start > len(buffer) || length > len(buffer)-start {
		return nil, ErrConflict
	}
	security := buffer[start : start+length]
	if binary.LittleEndian.Uint32(security) != darwinFileSecurityMagic {
		return nil, ErrUnsupportedEntry
	}
	return security, nil
}

func applyLocalTextMetadata(_ *os.File, staged *os.File, snapshot localTextMetadata) error {
	if snapshot.mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky) != 0 {
		return ErrUnsupportedEntry
	}
	if len(snapshot.platformMetadata) < darwinAttributeLengthBytes {
		return ErrUnsupportedEntry
	}
	flags := binary.LittleEndian.Uint32(snapshot.platformMetadata)
	if flags&(unix.UF_IMMUTABLE|unix.UF_APPEND|unix.SF_IMMUTABLE|unix.SF_APPEND|unix.SF_RESTRICTED|unix.SF_NOUNLINK|unix.UF_COMPRESSED|unix.SF_DATALESS) != 0 {
		return ErrUnsupportedEntry
	}
	current, err := captureLocalTextMetadata(staged)
	if err != nil {
		return err
	}
	if current.uid != snapshot.uid || current.gid != snapshot.gid {
		if err := staged.Chown(snapshot.uid, snapshot.gid); err != nil {
			return err
		}
	}
	if err := staged.Chmod(snapshot.mode.Perm()); err != nil {
		return err
	}
	if err := applyLocalTextExtendedAttributes(staged, snapshot.extendedAttributes); err != nil {
		return err
	}
	security := snapshot.platformMetadata[darwinAttributeLengthBytes:]
	if err := setDarwinTextSecurity(staged, security); err != nil {
		return err
	}
	return unix.Fchflags(int(staged.Fd()), int(flags))
}

func setDarwinTextSecurity(staged *os.File, security []byte) error {
	if len(security) == 0 {
		// Explicitly remove a directory's inherited ACL when the source has none.
		security = make([]byte, darwinFileSecurityHeaderBytes)
		binary.LittleEndian.PutUint32(security, darwinFileSecurityMagic)
		binary.LittleEndian.PutUint32(security[darwinFileSecurityHeaderBytes-8:], darwinNoACL)
	}
	buffer := make([]byte, darwinAttributeReferenceBytes+len(security))
	binary.LittleEndian.PutUint32(buffer, darwinAttributeReferenceBytes)
	binary.LittleEndian.PutUint32(buffer[darwinAttributeLengthBytes:], uint32(len(security)))
	copy(buffer[darwinAttributeReferenceBytes:], security)
	attributes := unix.Attrlist{Bitmapcount: darwinAttributeBitmapCount, Commonattr: unix.ATTR_CMN_EXTENDED_SECURITY}
	return darwinTextAttributes(staged, darwinTextAttributeRequest{operation: unix.SYS_FSETATTRLIST, attributes: &attributes, buffer: buffer})
}

// x/sys does not expose the descriptor forms of getattrlist/setattrlist. Use
// their Darwin syscalls so ACL access remains bound to the already-open file.
type darwinTextAttributeRequest struct {
	operation  uintptr
	attributes *unix.Attrlist
	buffer     []byte
	options    uintptr
}

func darwinTextAttributes(file *os.File, request darwinTextAttributeRequest) error {
	operation, attributes, buffer, options := request.operation, request.attributes, request.buffer, request.options
	_, _, errno := syscall.Syscall6(operation, file.Fd(), uintptr(unsafe.Pointer(attributes)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), options, 0)
	runtime.KeepAlive(file)
	runtime.KeepAlive(attributes)
	runtime.KeepAlive(buffer)
	if errno != 0 {
		return errno
	}
	return nil
}
