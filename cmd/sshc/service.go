package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
)

// ServiceSubcommand は、インストールと削除がログインサービスを保守するときの入口。
// 任意の実行ファイルを登録できる一般用途のサービス管理コマンドではない。
const ServiceSubcommand = "service"

const serviceUsage = "usage: sshc service refresh|disable"

// serviceLoginItem はWebの設定スイッチと同じ境界である。ここでもOS固有のplistや
// systemd unitを組み立て直さず、既存の実装に状態遷移だけを依頼する。
type serviceLoginItem interface {
	Registered() (bool, error)
	Enable(context.Context, string) error
	Disable(context.Context) error
}

// serviceInvocation は、actionの正しさにかかわらずserviceという語を予約する。
// 打ち間違えた保守コマンドをSSHホスト名として扱わないためである。
func serviceInvocation(argv []string) bool {
	return len(argv) > 1 && argv[1] == ServiceSubcommand
}

func serviceArgumentsValid(arguments []string) bool {
	return len(arguments) == 1 && (arguments[0] == "refresh" || arguments[0] == "disable")
}

// runServiceCommand はactionを確定してから初めてHOMEとOSのサービス状態へ触る。
// usageの打ち間違いが、そのマシン固有のエラーや副作用へ変わらないための入口である。
func runServiceCommand(
	ctx context.Context,
	arguments []string,
	homeDirectory func() (string, error),
	loginItem func(string) (serviceLoginItem, error),
	executable func() (string, error),
	stdout io.Writer,
	stderr io.Writer,
) int {
	if !serviceArgumentsValid(arguments) {
		fmt.Fprintln(stderr, serviceUsage)
		return 2
	}
	home, err := homeDirectory()
	if err != nil {
		fmt.Fprintf(stderr, "sshc: resolve home directory: %v\n", err)
		return 1
	}
	item, err := loginItem(home)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: inspect login service: %v\n", err)
		return 1
	}
	return runService(ctx, arguments, item, executable, stdout, stderr)
}

// runService はブラウザもサーバーもSSHも起動せず、ログインサービスだけを保守する。
// executableはrefreshが本当に必要な場合にだけ呼び、argvからプログラムパスを受けない。
func runService(
	ctx context.Context,
	arguments []string,
	item serviceLoginItem,
	executable func() (string, error),
	stdout io.Writer,
	stderr io.Writer,
) int {
	if !serviceArgumentsValid(arguments) {
		fmt.Fprintln(stderr, serviceUsage)
		return 2
	}

	if item == nil {
		fmt.Fprintln(stdout, "sshc: login service is not enabled; nothing changed")
		return 0
	}
	registered, err := item.Registered()
	if err != nil {
		fmt.Fprintf(stderr, "sshc: inspect login service registration: %v\n", err)
		return 1
	}
	if !registered {
		fmt.Fprintln(stdout, "sshc: login service is not enabled; nothing changed")
		return 0
	}

	switch arguments[0] {
	case "refresh":
		program, err := executable()
		if err != nil {
			fmt.Fprintf(stderr, "sshc: resolve this executable: %v\n", err)
			return 1
		}
		if !filepath.IsAbs(program) {
			fmt.Fprintln(stderr, "sshc: resolved executable path is not absolute")
			return 1
		}
		if err := item.Enable(ctx, filepath.Clean(program)); err != nil {
			fmt.Fprintf(stderr, "sshc: refresh login service: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "sshc: login service refreshed; vault is locked after restart")
		return 0
	case "disable":
		if err := item.Disable(ctx); err != nil {
			fmt.Fprintf(stderr, "sshc: disable login service: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "sshc: login service disabled")
		return 0
	default:
		panic("validated service action became unreachable")
	}
}
