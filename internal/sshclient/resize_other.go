//go:build !unix

package sshclient

import "os"

// notifyResize は、この プラットフォームでは何も届けない。SIGWINCH が無い。
func notifyResize(chan<- os.Signal) {}
