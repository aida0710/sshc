//go:build unix && !linux && !darwin

package terminal

import "os"

// enableUTF8Input は、sshc が配らない Unix では端末の設定を変えない。
func enableUTF8Input(*os.File) error { return nil }
