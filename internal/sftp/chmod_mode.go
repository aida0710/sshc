package sftp

import (
	"io/fs"
	"strconv"
)

func ParseChmodMode(mode string) (fs.FileMode, error) {
	if len(mode) != 3 && (len(mode) != 4 || mode[0] != '0') {
		return 0, fs.ErrInvalid
	}
	for _, digit := range mode {
		if digit < '0' || digit > '7' {
			return 0, fs.ErrInvalid
		}
	}
	parsed, err := strconv.ParseUint(mode, 8, 9)
	return fs.FileMode(parsed), err
}
