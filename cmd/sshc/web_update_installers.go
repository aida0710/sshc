package main

import (
	"context"
	"io"
	"net/http"

	"sshc/internal/releasecheck"
)

// webInstallRequest は、Web の画面から始めた更新で、インストーラーへ渡すものである。
type webInstallRequest struct {
	found   installation
	release releasecheck.Release
	// output は、インストーラーの標準出力と標準エラーを、書かれた順に受け取る。画面は
	// これをジョブのログから読んで、動いている間に表示する。
	output io.Writer
}

// webInstallers は、Web の画面から始めた更新でインストーラーを動かす。sshc update と
// 違って、出力は呼び出した人のターミナルではなく、ジョブのログへ書く。
type webInstallers struct {
	client   *http.Client
	commands installationCommands
}

// runTaggedInstaller は、install.sh か install.ps1 を動かす。sshc エンジンの再起動は
// 再起動の helper に任せるので、caller を web-update にする。
func (installers webInstallers) runTaggedInstaller(ctx context.Context, request webInstallRequest) error {
	installer := updateInstaller{client: installers.client, commands: installers.commands,
		stdout: request.output, stderr: request.output, caller: "web-update"}
	return installer.install(ctx, request.found, request.release)
}

func (installers webInstallers) upgradeHomebrew(ctx context.Context, request webInstallRequest) (string, error) {
	installer := updateInstaller{commands: installers.commands, stdout: request.output, stderr: request.output}
	return installer.upgradeHomebrewFromWeb(ctx, request.found)
}
