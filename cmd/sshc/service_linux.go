//go:build linux

package main

import (
	"fmt"
	"os"

	"sshc/internal/platform"
	"sshc/internal/platform/linux"
	"sshc/internal/platform/process"
)

// newServiceLoginItem は、systemdが無い環境を安全なno-opにしつつ、unitだけが残った
// 不整合を成功扱いにしない。
func newServiceLoginItem(home string) (serviceLoginItem, error) {
	return newLinuxServiceLoginItem(home, os.Stat, process.NewOutputRunner())
}

func newLinuxServiceLoginItem(
	home string,
	stat func(string) (os.FileInfo, error),
	runner platform.OutputRunner,
) (serviceLoginItem, error) {
	item := linux.LoginItem{Runner: runner, Home: home}
	registered, err := item.Registered()
	if err != nil {
		return nil, fmt.Errorf("inspect login service: %w", err)
	}
	if _, err := stat(linux.DefaultSystemctl); err == nil {
		return item, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect systemctl: %w", err)
	}

	if registered {
		return nil, fmt.Errorf("login service is registered but %s is unavailable", linux.DefaultSystemctl)
	}
	return nil, nil
}
