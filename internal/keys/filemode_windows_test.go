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
	// **この体系では、どちらの鍵にも印を付けない。** mode ビットには誰が読めるかが
	// 入っておらず、Unix と同じ式で見れば必ず両方が「危険」になる。常に真の警告は
	// 何も伝えないので、判断しなかったことをそのまま表す。ここで本当に答えるべき
	// 問いは DACL の側にあり、それはまだ書かれていない。
	if exposed.PermissionRisk || safe.PermissionRisk {
		t.Errorf("a key was flagged from mode bits that carry no access information")
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
