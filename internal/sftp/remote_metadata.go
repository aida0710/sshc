package sftp

import (
	"io/fs"
	"strconv"

	pkgsftp "github.com/pkg/sftp"
)

type ownershipInfo interface{ Ownership() (Ownership, bool) }

func ownershipFrom(info fs.FileInfo) (Ownership, bool) {
	if owner, ok := info.(ownershipInfo); ok {
		return owner.Ownership()
	}
	owner, ok := info.Sys().(*pkgsftp.FileStat)
	flags, retained := metadataFlagsFrom(info)
	if ok && retained && flags&sftpOwnerAttributes != 0 {
		return Ownership{UID: owner.UID, GID: owner.GID}, true
	}
	return Ownership{}, false
}

func metadataFlagsFrom(info fs.FileInfo) (uint32, bool) {
	attributes, ok := info.Sys().(*pkgsftp.FileStat)
	if !ok {
		return 0, false
	}
	for index := len(attributes.Extended) - 1; index >= 0; index-- {
		attribute := attributes.Extended[index]
		if attribute.ExtType == metadataFlagsAttribute {
			flags, err := strconv.ParseUint(attribute.ExtData, 10, 32)
			return uint32(flags), err == nil
		}
	}
	return 0, false
}

// With no permissions attribute the library synthesizes mode 0, which looks like
// a regular file. Never let that authorize SETSTAT on an unknown entry type.
func metadataTypeKnown(info fs.FileInfo) bool {
	flags, retained := metadataFlagsFrom(info)
	return !retained || flags&sftpPermissionsAttributes != 0
}
