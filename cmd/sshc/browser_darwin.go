//go:build darwin

package main

func browserCommand(url string) (string, []string) { return "/usr/bin/open", []string{url} }
