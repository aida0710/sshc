//go:build linux

package main

import (
	"os"

	"sshc/internal/platform/linux"
	"sshc/internal/platform/process"
)

func newPlatformParts(home string) platformParts {
	// この組み立ての中では、OpenSSH のプログラムを起動するすべてのサブシステムが
	// ひとつのプロセスランナーとひとつのツールチェーンを共有する。これにより argv、
	// 子プロセスの環境、出力の上限を決める場所はひとつだけになる。
	runner := process.NewOutputRunner()
	toolchain := linux.NewToolchain()
	parts := platformParts{
		Runner:    runner,
		Toolchain: toolchain,
		Browser:   linux.NewBrowser(runner),
		KeyAgent:  process.NewKeyAgent(runner, toolchain, os.LookupEnv),
		// Terminal は設定しない。Linux では端末を開かず、コマンドを表示して
		// 利用者が実行する。理由はタスク 8 にある。nil は diagnostics が
		// 「端末が設定されていない」と報告する、支持された状態である。
	}
	// systemd が無い環境で LoginItem を組み立てると、画面にはスイッチが出るのに
	// 押すと必ず失敗する。それより「非対応」と答える方が正しいので、この一度
	// だけの起動時チェックで systemctl の有無を probe し、無ければ LoginItem は
	// nil のままにする。macOS で LoginItem コントローラを渡さないビルドと同じ
	// 状態であり、httpserver 側はその nil を非対応として扱う。
	if _, err := os.Stat(linux.DefaultSystemctl); err == nil {
		parts.LoginItem = linux.LoginItem{Runner: runner, Home: home}
	}
	return parts
}
