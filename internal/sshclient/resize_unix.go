//go:build unix

package sshclient

import (
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

// notifyResize は、端末の大きさが変わったという合図を受け取る。
//
// ビルドタグで分けてあるのは、SIGWINCH が Unix にしか無いからである。
// 他のプラットフォームでは大きさは変わらないものとして扱う。誤りだが、
// 知る手段が無いときに知っているふりをするよりましである。
func notifyResize(c chan<- os.Signal) { signal.Notify(c, unix.SIGWINCH) }
