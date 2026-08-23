//go:build windows

package storage

import (
	"strings"
	"testing"
)

// Windows に uid は無い。user.Current がそこで Uid に入れるのは SID であり、
// 数を期待すると、この OS が持っていないものを要求することになる。ここで確かめる
// のは、`%i` に入る値がファイル名の一部として使える形をしていることだけである。
func assertLocalUID(t *testing.T, uid string) {
	t.Helper()
	if !strings.HasPrefix(uid, "S-1-") {
		t.Fatalf("uid %q は SID ではない", uid)
	}
}
