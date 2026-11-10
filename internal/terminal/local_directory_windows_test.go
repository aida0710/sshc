package terminal

import (
	"errors"
	"testing"
)

func TestALocalWorkingDirectoryOnANetworkOrDeviceVolumeIsRefusedBeforeItIsOpened(t *testing.T) {
	// 192.0.2.1 は文書用のアドレスである。検査が os.Stat のあとにあると、試験はすぐに失敗せず、SMB の接続を
	// 待つことになる。
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
