//go:build !darwin && !windows

package main

import "os"

// **画面が無ければ開かない。** DISPLAY も WAYLAND_DISPLAY も無い機械には、
// 前へ出す先が無い。
func browserCommand(url string) (string, []string) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return "", nil
	}
	return "xdg-open", []string{url}
}
