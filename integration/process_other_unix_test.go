//go:build unix && !linux

package integration

import "errors"

// この OS に /proc は無い。呼び出し側が ps へ退く。
func readProcEnviron(int) (string, error) { return "", errors.New("no /proc on this system") }
