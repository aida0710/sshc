//go:build linux

package sftp

import "os"

func captureLocalTextMetadata(file *os.File) (localTextMetadata, error) {
	metadata, _, err := unixLocalTextMetadata(file)
	if err != nil {
		return localTextMetadata{}, err
	}
	metadata.revision = localTextMetadataRevision(metadata)
	return metadata, nil
}

func applyLocalTextMetadata(_ *os.File, staged *os.File, snapshot localTextMetadata) error {
	// Privileged executable bits and capabilities need a deliberate policy when
	// changing contents. The text editor only replaces ordinary unprivileged files.
	if _, privileged := snapshot.extendedAttributes["security.capability"]; privileged {
		return ErrUnsupportedEntry
	}
	return applyUnixLocalTextOwnership(staged, snapshot)
}
