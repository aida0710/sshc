package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsUpdateNoticesUseUpdateAndExplainUnmanagedInstallations(t *testing.T) {
	if notice := availableUpdateNotice("v0.14.0"); notice != "sshc: v0.14.0 is available; run `sshc update`\n" {
		t.Fatalf("notice=%q", notice)
	}
	notice := unmanagedInstallationNotice(`C:\sshc\sshc.exe`, "windows")
	if !strings.Contains(notice, "not managed by sshc's install.ps1") || !strings.Contains(notice, "\n  "+windowsInstallerCommand+"\n") {
		t.Fatalf("notice=%q", notice)
	}
}

func TestUpdateNoticesOutsideWindowsKeepToSshcUpdate(t *testing.T) {
	if notice := availableUpdateNotice("v0.14.0"); notice != "sshc: v0.14.0 is available; run `sshc update`\n" {
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
