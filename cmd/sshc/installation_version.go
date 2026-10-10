package main

import (
	"bytes"

	"sshc/internal/releasecheck"
)

func reportedReleaseVersion(line []byte) (string, bool) {
	fields := bytes.Fields(line)
	if len(fields) != 3 || string(fields[0]) != "sshc" {
		return "", false
	}
	return releasecheck.StableTag(string(fields[1]))
}
