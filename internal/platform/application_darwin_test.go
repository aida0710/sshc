//go:build darwin

package platform_test

import (
	"errors"
	"testing"

	"sshc/internal/platform"
)

// バンドルの要求は macOS のものである。ここが緩めば、保存の時点で弾けたはずの
// 設定が起動の時点まで生き延びる。
func TestCustomTerminalOnDarwinMustBeAnApplicationBundle(t *testing.T) {
	for _, application := range []string{"/usr/bin/foot", "/Applications/Foo", "/Applications/Foo.APP"} {
		choice := platform.TerminalChoice{ID: platform.TerminalCustom, Application: application}
		if err := platform.ValidateTerminalChoice(choice); !errors.Is(err, platform.ErrTerminalApplication) {
			t.Errorf("ValidateTerminalChoice(%q) = %v, want ErrTerminalApplication", application, err)
		}
	}
	ok := platform.TerminalChoice{ID: platform.TerminalCustom, Application: "/Applications/Foo.app"}
	if err := platform.ValidateTerminalChoice(ok); err != nil {
		t.Errorf("ValidateTerminalChoice(bundle) = %v, want nil", err)
	}
}
