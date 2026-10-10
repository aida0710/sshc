package sftp

import (
	"strings"
	"unicode/utf8"
)

// SFTP paths and link targets use the OpenAPI maximum of 4096 characters.
const maxRemoteMetadataPathCharacters = 4096

func validateLinkTarget(target string) error {
	if target == "" || strings.ContainsRune(target, 0) || utf8.RuneCountInString(target) > maxRemoteMetadataPathCharacters {
		return ErrInvalidPath
	}
	return nil
}

func cleanMetadataPath(candidate string) (string, error) {
	if utf8.RuneCountInString(candidate) > maxRemoteMetadataPathCharacters {
		return "", ErrInvalidPath
	}
	return cleanPublicPath(candidate, false)
}
