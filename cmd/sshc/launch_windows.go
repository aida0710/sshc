//go:build windows

package main

import (
	"context"
	"os/exec"

	"sshc/internal/platform/windowsregistry"
)

// windowsDesktop は、インストーラが記録した実体を直接起こす。
//
// **shell を通さないし、PATH も引かない。** `cmd.exe /c` も PowerShell も
// 使わないのは、そこへ渡した瞬間にパスが引用と展開の対象になるからである。
// PATH を引かないのは、誰の PATH に何が置かれているかで起こすものが変わる
// からで、これは `/usr/bin/open` と Linux の記述子が守っているのと同じ性質で
// ある。**起こすものは、記録された絶対パスひとつだけである。**
type windowsDesktop struct {
	// read は登録された実体を返す。テストが差し替えるためにある——本物の
	// レジストリを書き換えるテストは、走らせた人の機械に痕跡を残す。
	read func() (string, error)
}

func newDesktopLauncher() desktopLauncher {
	return windowsDesktop{read: windowsregistry.ReadDesktopExecutable}
}

// Available は、登録された実体がいまも起こせるかを答える。
//
// **画面の有無を問わない。** Windows でそれを確かめる手立ては、対話セッション
// かどうかという別の問いになる。ここで答えられるのは「登録があり、それが指す
// ものが起こせるか」であり、登録が無いことも壊れていることも直し方を持つので、
// どちらも error として返す。
func (launcher windowsDesktop) Available() (bool, error) {
	if _, err := launcher.read(); err != nil {
		return false, err
	}
	return true, nil
}

func (launcher windowsDesktop) Launch(ctx context.Context) error {
	path, err := launcher.read()
	if err != nil {
		return err
	}
	// 二つ目の実体は、既存の実体へ知らせて自分は終わる。待たないのは、外殻が
	// 上がりきるより先に端末を返してよいからである。
	return exec.CommandContext(ctx, path).Start()
}
