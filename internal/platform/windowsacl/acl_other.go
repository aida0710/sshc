//go:build !windows

// Package windowsacl は、呼び出し側へ private-state policy の境界を与える。
package windowsacl

// RestrictFile は、Unix では対応する書込箇所が既に 0600 を適用するため no-op にする。
func RestrictFile(path string) error { return nil }

// RestrictDirectory は、Unix では対応する作成箇所が既に 0700 を適用するため no-op にする。
func RestrictDirectory(path string) error { return nil }

// IsRestrictedToCurrentUser は Windows DACL だけが対象であり、Unix の検査は既存の
// file-mode assertion が担う。
func IsRestrictedToCurrentUser(path string) (bool, error) { return true, nil }
