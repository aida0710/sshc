//go:build unix

package storage

import (
	"strconv"
	"testing"
)

// OpenSSH の `%i` は uid である。
func assertLocalUID(t *testing.T, uid string) {
	t.Helper()
	if _, err := strconv.Atoi(uid); err != nil {
		t.Fatalf("uid %q は数値ではない", uid)
	}
}
