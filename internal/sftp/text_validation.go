package sftp

import (
	"bytes"
	"unicode/utf8"
)

func validText(contents []byte) bool {
	return utf8.Valid(contents) && !bytes.ContainsRune(contents, '\x00')
}
