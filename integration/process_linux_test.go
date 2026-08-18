//go:build linux

package integration

import (
	"os"
	"strconv"
	"strings"
)

// readProcEnviron は、Linux が同じ利用者へ見せている環境をそのまま読む。
func readProcEnviron(pid int) (string, error) {
	contents, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(contents), "\x00", "\n"), nil
}
