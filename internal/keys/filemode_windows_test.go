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

func assertPrivateKeyPermissionRisk(t *testing.T, exposed, safe *Item) {
	t.Helper()
	// **fixture が書き分けた 0600 と 0644 の差は、ここには現れない。** mode
	// ビットには誰が読めるかが入っていないので、その二つは Windows では同じ
	// ものである。判定しているのは DACL であり、fixture はどちらも私的な道で
	// 書いているので、**どちらも閉じている。**
	//
	// 露出を実際に作って判定させるのは windowsacl の側である
	// （exposure_windows_test.go が icacls で他人に読みを与えて確かめる）。
	// ここが確かめるのは、**mode ビットからは何も flag しない**ことである。
	if exposed.PermissionRisk || safe.PermissionRisk {
		t.Errorf("a key written through the private path was flagged as exposed")
	}
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
