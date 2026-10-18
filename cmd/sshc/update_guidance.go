package main

import "fmt"

// windowsInstallerCommand は、Windows で sshc を入れる PowerShell の 1 行である。
// 同じ行をもう一度実行すると、最新の版に置き換わる。docs/release-install.md と
// README に書いた行と同じにする（TestWindowsInstallerCommandMatchesTheDocumentedOne）。
const windowsInstallerCommand = `powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/aida0710/sshc/releases/latest/download/install.ps1 | iex"`

// availableUpdateNotice は、engine が新しい版を見つけたときに出す案内である。
func availableUpdateNotice(version string) string {
	return fmt.Sprintf("sshc: %s is available; run `sshc update`\n", version)
}

// unmanagedInstallationNotice は、sshc update が導入元を見分けられず、更新を断るときの案内である。
func unmanagedInstallationNotice(executable, goos string) string {
	if goos == "windows" {
		return fmt.Sprintf("sshc: %s is not managed by sshc's install.ps1, so it cannot be updated automatically\n", executable) +
			"sshc: install it with:\n  " + windowsInstallerCommand + "\n"
	}
	return fmt.Sprintf("sshc: %s is not managed by Homebrew or sshc's install.sh, so it cannot be updated automatically\n", executable) +
		"sshc: update it with the method that installed it\n"
}
