//go:build windows

package main

import "os/exec"

// WindowsはHomebrew／install.shの更新対象外であり、子process設定は使用しない。
func configureUpdateCommand(_ *exec.Cmd) {}
