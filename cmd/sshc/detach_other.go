//go:build !unix

package main

import "os/exec"

// detach は、このプラットフォームでは何もしない。
func detach(*exec.Cmd) {}
