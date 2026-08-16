//go:build windows

package keys

import "testing"

// filemode_unix_test.go の対になる側。Windows で同じ約束を担っているのは mode
// ビットではなく DACL なので、確かめる相手を windowsacl へ移す。何も確かめない
// ものについては、なぜ確かめられないのかをその場に書き残す。

func assertScannedPermission(t *testing.T, item *Item, _ string) {
	t.Helper()
	// Permission は Windows では FILE_ATTRIBUTE_READONLY を写しただけの値であり、
	// 誰が読めるかを何ひとつ言っていない。fixture が書き分けた 0600 と 0644 の差は
	// ここには現れない。合成されうる二値であることだけを固定して、この文字列が
	// POSIX の約束として読まれるのを防ぐ。
	if item.Permission != "0666" && item.Permission != "0444" {
		t.Errorf("Permission = %q, want a Windows-synthesised 0666 or 0444", item.Permission)
	}
}

func assertPrivateKeyPermissionRisk(t *testing.T, _, _ *Item) {
	t.Helper()
	// PermissionRisk は Perm()&0o077 から決まる。Windows では 0666 も 0444 もこの
	// 判定に掛かるので、DACL がどうであろうと秘密鍵は必ず危険と報告される。露出
	// した鍵と保護された鍵をここで見分けることはできない。見分けられるようにする
	// のは inventory の側であって、fixture の側ではない。
}

func assertGeneratedKeyIsPrivate(t *testing.T, path string) {
	t.Helper()
	assertRestrictedKeyPath(t, path)
}

func tightenTrashSourceKey(t *testing.T, _ string) {
	t.Helper()
	// 「これ以上緩められない」という 0400 に当たる印は Windows には無い。移動が
	// 保つべきものは所有者と厳密な DACL であり、それは移動したあとで確かめる。
}

func assertTrashEntryIsPrivate(t *testing.T, entryDirectory, keyPath string) {
	t.Helper()
	assertRestrictedKeyPath(t, entryDirectory)
	assertRestrictedKeyPath(t, keyPath)
}
