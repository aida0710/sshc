//go:build !windows

package nativepath

// foldIdentity は Unix では何もしない。綴りが違えば別のファイルである。
func foldIdentity(path string) string { return path }
