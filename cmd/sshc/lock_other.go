//go:build !unix

package main

// lockEngineStart は、このプラットフォームでは何もしない。flock が無い。
//
// **ここに別の仕組みを推測で書かない。** 束を配っているのは macOS と Linux で
// あり、どちらも unix である。
func lockEngineStart(string) (func(), error) { return func() {}, nil }
