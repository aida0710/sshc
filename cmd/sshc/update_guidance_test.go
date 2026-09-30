package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsUpdateNoticesGiveTheInstallerCommandOnItsOwnLine(t *testing.T) {
	commandLine := "\n  " + windowsInstallerCommand + "\n"
	for name, notice := range map[string]string{
		"available update": availableUpdateNotice("v0.14.0", "windows"),
		"unmanaged":        unmanagedInstallationNotice(`C:\sshc\sshc.exe`, "windows"),
	} {
		if !strings.Contains(notice, commandLine) {
			t.Errorf("%s notice does not put the installer command on its own line:\n%s", name, notice)
		}
		if strings.Contains(notice, "`sshc update`") {
			t.Errorf("%s notice tells Windows to run sshc update, which refuses there:\n%s", name, notice)
		}
	}
}

func TestUpdateNoticesOutsideWindowsKeepToSshcUpdate(t *testing.T) {
	if notice := availableUpdateNotice("v0.14.0", "darwin"); notice != "sshc: v0.14.0 is available; run `sshc update`\n" {
		t.Errorf("available update notice = %q", notice)
	}
	notice := unmanagedInstallationNotice("/usr/local/bin/sshc", "linux")
	if strings.Contains(notice, "install.ps1") || !strings.Contains(notice, "the method that installed it") {
		t.Errorf("unmanaged notice = %q", notice)
	}
}

func TestWindowsInstallerCommandMatchesTheDocumentedOne(t *testing.T) {
	for _, path := range []string{"README.md", filepath.Join("docs", "release-install.md")} {
		body, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), windowsInstallerCommand+"\n") {
			t.Errorf("%s does not document the installer command the CLI prints:\n%s", path, windowsInstallerCommand)
		}
	}
}
