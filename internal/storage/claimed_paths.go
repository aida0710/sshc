package storage

import "sshc/internal/platform/nativepath"

// claimedPaths は、ひとつのリクエストや記録がすでに扱ったパスの台帳。
//
// 同じファイルを指す表記（Windows の大小文字違いなど）は nativepath.Identity で
// ひとつの鍵にまとめる。sameJournalPath と同じ判断を、1 件あたり一定の時間で行う。
// バックアップを数千件抱えるマスターパスワード変更でも、重複の検査で全体の時間が
// 件数の 2 乗に伸びない。
type claimedPaths map[string]struct{}

func newClaimedPaths(capacity int) claimedPaths {
	return make(claimedPaths, capacity)
}

// claim は path を台帳に載せる。すでに載っていれば偽を返し、何も変えない。
func (c claimedPaths) claim(path string) bool {
	key := nativepath.Identity(path)
	if _, claimed := c[key]; claimed {
		return false
	}
	c[key] = struct{}{}
	return true
}

// contains は、path が台帳に載っているかを返す。
func (c claimedPaths) contains(path string) bool {
	_, claimed := c[nativepath.Identity(path)]
	return claimed
}
