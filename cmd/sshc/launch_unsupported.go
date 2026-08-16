//go:build !darwin && !linux

package main

import (
	"context"
	"errors"
)

// unsupportedDesktop は、起こし方が分かっていない環境の答えである。
//
// **推測して起こさない。** 起こし方が環境で割れるものを当てにいけば、当たらな
// かった日に利用者の計算機で意図しないものが動く。起こせないと言えば、接続経路
// は保存済みを使わずに繋ぐ方へ退く——接続そのものは常にできる。
type unsupportedDesktop struct{}

func newDesktopLauncher() desktopLauncher { return unsupportedDesktop{} }

func (unsupportedDesktop) Available() (bool, error) { return false, nil }

func (unsupportedDesktop) Launch(context.Context) error {
	return errors.New("this platform has no sshc desktop application")
}

func launchBackground(context.Context) bool { return false }
