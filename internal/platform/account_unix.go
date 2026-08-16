//go:build !windows

package platform

// LocalAccountName は、OpenSSH の `%u` が指す名前を返す。
func LocalAccountName(name string) string { return name }
