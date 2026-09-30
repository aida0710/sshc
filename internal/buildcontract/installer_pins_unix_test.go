//go:build !windows

package buildcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/semver"
)

// newestStableReleaseNotesTag は、docs/releasesにあるリリースノートのうち、最も新しい
// 安定バージョンのtagを返す。
func newestStableReleaseNotesTag(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("..", "..", "docs", "releases"))
	if err != nil {
		t.Fatal(err)
	}
	newest := ""
	for _, entry := range entries {
		tag, isMarkdown := strings.CutSuffix(entry.Name(), ".md")
		if !isMarkdown || semver.Canonical(tag) != tag || semver.Prerelease(tag) != "" {
			continue
		}
		if newest == "" || semver.Compare(tag, newest) > 0 {
			newest = tag
		}
	}
	if newest == "" {
		t.Fatal("docs/releases has no stable release notes")
	}
	return newest
}

// 導入例の固定バージョンは、mainにある最新の安定版のリリースノートと同じである。
// リリースノートを足した変更で導入例を上げ忘れると、ここで落ちる。README だけ更新して
// pages が古いバージョンを案内し続けたことがある。
func TestTheDocumentedInstallersPinTheNewestStableRelease(t *testing.T) {
	tag := newestStableReleaseNotesTag(t)
	check, err := filepath.Abs(filepath.Join("..", "..", filepath.FromSlash(pinnedInstallerCheck)))
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("sh", check, tag).CombinedOutput()
	if err != nil {
		t.Fatalf("the installer examples do not all pin %s: %v\n%s", tag, err, output)
	}
}

// 照合scriptは、shの例とPowerShellの例のどちらか1か所でも古ければ失敗する。
func TestThePinnedInstallerCheckRejectsAnyStaleExample(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(pinnedInstallerCheck)))
	if err != nil {
		t.Fatal(err)
	}
	shellExample := func(tag string) string {
		return "SSHC_VERSION=" + tag + " sh -c \\\n" +
			"  'curl -fsSL https://raw.githubusercontent.com/aida0710/sshc/" + tag + "/install.sh | sh'\n"
	}
	powerShellExample := func(tag string) string {
		return "$env:SSHC_VERSION = '" + tag + "'\n" +
			"irm https://github.com/aida0710/sshc/releases/download/" + tag + "/install.ps1 | iex\n"
	}
	shellDocuments := []string{
		"README.md", "install.sh", "pages/guide/install.md", "pages/en/guide/install.md",
	}

	for _, test := range []struct {
		name string
		// documents は、リポジトリ内のパスと本文の組である。
		documents map[string]string
		// wantFailure は、失敗の出力に含まれるべき文である。空なら成功を期待する。
		wantFailure string
	}{
		{
			name:      "every example pins the tag",
			documents: map[string]string{"docs/release-install.md": shellExample("v1.2.3") + powerShellExample("v1.2.3")},
		},
		{
			name:        "the PowerShell example is stale",
			documents:   map[string]string{"docs/release-install.md": shellExample("v1.2.3") + powerShellExample("v1.2.2")},
			wantFailure: "docs/release-install.md does not pin the installer to v1.2.3",
		},
		{
			name: "a pages guide is stale",
			documents: map[string]string{
				"docs/release-install.md":   shellExample("v1.2.3") + powerShellExample("v1.2.3"),
				"pages/en/guide/install.md": shellExample("v1.2.2"),
			},
			wantFailure: "pages/en/guide/install.md does not pin the installer to v1.2.3",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			documents := map[string]string{pinnedInstallerCheck: string(script)}
			for _, path := range shellDocuments {
				documents[path] = shellExample("v1.2.3")
			}
			for path, body := range test.documents {
				documents[path] = body
			}
			for path, body := range documents {
				fixture := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(fixture), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fixture, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			output, err := exec.Command("sh", filepath.Join(root, pinnedInstallerCheck), "v1.2.3").CombinedOutput()
			if test.wantFailure == "" {
				if err != nil {
					t.Fatalf("check = %v\n%s", err, output)
				}
				return
			}
			if err == nil {
				t.Fatalf("check passed although an example is stale:\n%s", output)
			}
			if !strings.Contains(string(output), test.wantFailure) {
				t.Errorf("check output does not name the stale example %q:\n%s", test.wantFailure, output)
			}
		})
	}
}
