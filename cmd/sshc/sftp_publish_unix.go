//go:build !windows

package main

import "os"

func publishSFTPDownload(source, destination string) error {
	return os.Rename(source, destination)
}
