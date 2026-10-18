//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

var freeEngineConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole")

func releaseEngineConsole() error {
	// FreeConsole also succeeds when a detached process has no console.
	success, _, err := freeEngineConsole.Call()
	if success == 0 {
		return err
	}
	return nil
}

func redirectEngineOutput(output *os.File) (func(), error) {
	stdout, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	stderr, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	restore := func() {
		_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, stdout)
		_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, stderr)
	}
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(output.Fd())); err != nil {
		return nil, err
	}
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(output.Fd())); err != nil {
		restore()
		return nil, err
	}
	restoreGo := replaceGoEngineOutput(output)
	return func() { restoreGo(); restore() }, nil
}
