//go:build windows

package main

import (
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestBackgroundEngineReleasesAnAttachedWindowsConsole(t *testing.T) {
	const childEnvironment = "SSHC_TEST_RELEASE_CONSOLE"
	if os.Getenv(childEnvironment) == "1" {
		kernel := windows.NewLazySystemDLL("kernel32.dll")
		window := kernel.NewProc("GetConsoleWindow")
		attached, _, _ := window.Call()
		if attached == 0 {
			allocated, _, _ := kernel.NewProc("AllocConsole").Call()
			if allocated == 0 {
				os.Exit(21)
			}
		}
		if err := releaseEngineConsole(); err != nil {
			os.Exit(22)
		}
		attached, _, _ = window.Call()
		if attached != 0 {
			os.Exit(23)
		}
		// An already detached background launch must also succeed.
		if err := releaseEngineConsole(); err != nil {
			os.Exit(24)
		}
		os.Exit(0)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestBackgroundEngineReleasesAnAttachedWindowsConsole$")
	command.Env = append(os.Environ(), childEnvironment+"=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("console helper: %v, %s", err, output)
	}
}
