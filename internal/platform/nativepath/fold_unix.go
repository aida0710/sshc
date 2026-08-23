//go:build !windows

package nativepath

// foldIdentity は Unix では何もしない。表記が違えば別のファイルである。
func foldIdentity(path string) string { return path }
