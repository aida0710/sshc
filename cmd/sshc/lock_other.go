//go:build !unix

package main

// lockEngineStart は、このプラットフォームでは何もしない。flock が無い。
func lockEngineStart(string) (func(), error) { return func() {}, nil }
