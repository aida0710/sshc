//go:build !darwin

package main

// launchApp は、darwin 以外では何もしない。
//
// **起こし方が環境で割れるものを、推測して起こさない。** 起こせなければ
// 保存済みを使わずに繋ぐ経路へ退く——接続そのものは常にできる。
func launchApp() bool { return false }
