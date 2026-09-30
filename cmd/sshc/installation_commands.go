package main

import (
	"context"
	"io"
	"os/exec"
)

// installationCommands は、sshc を入れた管理元（brew、install.sh を動かす sh）と、
// 入れた実行ファイルを、外部のプログラムとして動かす。sshc update が版を入れ替える
// のと、sshc service が Homebrew に安定したパスを尋ねるのに使う。
type installationCommands interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	Run(ctx context.Context, process installationProcess) error
}

// installationProcess は、更新のために最後まで実行させる外部コマンド 1 つである。
// 標準入力には何も渡さない。
type installationProcess struct {
	name string
	args []string
	// environment が nil なら、sshc 自身の環境をそのまま引き継ぐ。
	environment []string
	stdout      io.Writer
	stderr      io.Writer
}

// systemInstallationCommands は、実際にプロセスを起動する installationCommands である。
type systemInstallationCommands struct{}

func (systemInstallationCommands) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	configureUpdateCommand(command)
	return command.Output()
}

func (systemInstallationCommands) Run(ctx context.Context, process installationProcess) error {
	command := exec.CommandContext(ctx, process.name, process.args...)
	configureUpdateCommand(command)
	if process.environment != nil {
		command.Env = process.environment
	}
	command.Stdout, command.Stderr = process.stdout, process.stderr
	return command.Run()
}
