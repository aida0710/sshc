package terminal

import (
	"errors"
	"testing"
)

func TestALocalWorkingDirectoryOnANetworkOrDeviceVolumeIsRefusedBeforeItIsOpened(t *testing.T) {
	// 192.0.2.1 is a documentation address: if the check came after os.Stat,
	// the test would wait on an SMB connection instead of failing at once.
	for _, candidate := range []string{
		`\\192.0.2.1\share`, "//192.0.2.1/share", `\\?\C:\Windows`, `\\.\C:\Windows`, `\??\C:\Windows`,
	} {
		if _, err := ResolveLocalWorkingDirectory(candidate); !errors.Is(err, ErrInvalidLocalWorkingDirectory) {
			t.Errorf("ResolveLocalWorkingDirectory(%q) = %v", candidate, err)
		}
		if got := localWorkingDirectory(candidate); got != "" {
			t.Errorf("localWorkingDirectory(%q) = %q", candidate, got)
		}
	}
}
