//go:build !windows

// Package acltest builds the Windows security fixtures that the private-state
// tests need. Outside Windows it holds nothing; the fixtures it builds have no
// meaning on a filesystem that has no DACL.
package acltest
