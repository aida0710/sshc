//go:build !linux && !windows && !darwin && !freebsd && !openbsd

package machinestatus

// readPlatform は、読み方を用意していない OS では何も読まない（名前と OS だけを出す）。
func readPlatform() Reading { return Reading{} }
