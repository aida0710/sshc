package sftp_test

import (
	"runtime"
	"testing"

	"sshc/internal/sftp"
)

func TestLocalChildNameRefusesNamesThatAreNotOneEntryOnEverySystem(t *testing.T) {
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b"} {
		if sftp.ValidLocalChildName(name) {
			t.Errorf("ValidLocalChildName(%q) = true, want false", name)
		}
	}
}

// A colon names an alternate data stream or a drive on Windows, and CON, NUL
// and COM1 open a device. Elsewhere they are ordinary file names.
func TestLocalChildNameRefusesStreamsAndDevicesOnlyOnWindows(t *testing.T) {
	onWindows := runtime.GOOS == "windows"
	for _, name := range []string{"a:b", "a::$DATA", "CON", "nul", "com1"} {
		if got := sftp.ValidLocalChildName(name); got == onWindows {
			t.Errorf("ValidLocalChildName(%q) = %t on %s", name, got, runtime.GOOS)
		}
	}
	if !sftp.ValidLocalChildName("notes.txt") {
		t.Error("ValidLocalChildName refused an ordinary file name")
	}
}
