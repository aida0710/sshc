//go:build windows

package main

import "os/exec"

// configureBrowserLauncher は、Windows では何もしない。ターミナルのシグナルから
// 子を切り離す process group と session は Unix の仕組みである。
func configureBrowserLauncher(_ *exec.Cmd) {}
