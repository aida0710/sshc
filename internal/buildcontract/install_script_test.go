package buildcontract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readInstallScript は install.sh の契約テストで使用する内容を返す。
func readInstallScript(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// install.sh の対応ターゲットが Makefile の RELEASE_TARGETS と一致することを検証する。
func TestTheInstallScriptKnowsEveryMachineTheReleaseBuilds(t *testing.T) {
	contract := readMakefileContract(t)
	targets := contract.variables["RELEASE_TARGETS"]
	if len(targets) == 0 {
		t.Fatal("RELEASE_TARGETS is missing from the Makefile")
	}
	script := readInstallScript(t)

	for _, target := range targets {
		platform, _, _ := strings.Cut(target, ":")
		goos, goarch, ok := strings.Cut(platform, "/")
		if !ok {
			t.Fatalf("RELEASE_TARGETS entry %q is not goos/goarch", target)
		}
		if goos == "windows" {
			// Windows では standalone binary のファイル名と配置方法を案内する。
			artifact := fmt.Sprintf("sshc-windows-%s.exe", goarch)
			for _, required := range []string{artifact, "rename it to sshc.exe", "place it on PATH"} {
				if !strings.Contains(script, required) {
					t.Errorf("install.sh Windows instructions are missing %q", required)
				}
			}
			if strings.Contains(script, "Windows has an installer") {
				t.Error("install.sh advertises a Windows installer that is not released")
			}
			continue
		}
		if !strings.Contains(script, fmt.Sprintf("goos=%s", goos)) {
			t.Errorf("install.sh does not map uname to %s, but the release builds it", goos)
		}
		if !strings.Contains(script, fmt.Sprintf("goarch=%s", goarch)) {
			t.Errorf("install.sh does not map uname to %s, but the release builds it", goarch)
		}
	}
}

// ダウンロードしたバイナリを、公開された checksum でインストール前に検証する。
func TestTheInstallScriptRefusesAnythingItCannotVerify(t *testing.T) {
	script := readInstallScript(t)
	for _, required := range []string{
		"checksums.txt",
		"sha256sum",
		"shasum",
		"does not match its published checksum",
		"cannot be verified",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("install.sh must verify what it downloads; %q is missing", required)
		}
	}
	// checksum の検証前にバイナリを配置してはならない。
	if strings.Index(script, "published checksum") > strings.Index(script, `mv "$work/sshc" "$target"`) {
		t.Error("install.sh installs before it verifies the checksum")
	}
}

// install.sh がシェル設定や権限を変更せず、必要な操作だけを案内することを検証する。
func TestTheInstallScriptTouchesNothingItWasNotGiven(t *testing.T) {
	// コメントとメッセージから sudo などの文字列を除外し、実行文だけを検査する。
	var executable []string
	for _, line := range strings.Split(readInstallScript(t), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "note ") || strings.HasPrefix(trimmed, "say ") ||
			strings.HasPrefix(trimmed, "die ") || strings.HasPrefix(trimmed, "|| die ") {
			continue
		}
		executable = append(executable, trimmed)
	}
	body := strings.Join(executable, "\n")

	for _, forbidden := range []string{"sudo ", ">> $HOME", ">> $rc", ">>$rc"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("install.sh must not run %q itself", forbidden)
		}
	}
	// PATH が未設定の場合の案内は必要である。
	script := readInstallScript(t)
	if !strings.Contains(script, "is not on your PATH") {
		t.Error("install.sh must say so when the place it installed into is not on PATH")
	}
	if !strings.Contains(script, "$rc") {
		t.Error("install.sh must spell the line that puts it on PATH, even though it will not run it")
	}
}

// curl または wget の一方だけがある環境をサポートする。
func TestTheInstallScriptWorksWithEitherDownloader(t *testing.T) {
	script := readInstallScript(t)
	for _, tool := range []string{"curl", "wget"} {
		if !strings.Contains(script, "command -v "+tool) {
			t.Errorf("install.sh does not look for %s", tool)
		}
	}
}

// 稼働中の engine のバージョンは、機械可読な status 出力から取得する。
func TestTheInstallScriptReadsTheRunningEngineVersionFromJSON(t *testing.T) {
	script := readInstallScript(t)
	if !strings.Contains(script, "sshc status --json") {
		t.Error("install.sh must request JSON when reading the running engine version")
	}
	if strings.Contains(script, "sshc status 2>/dev/null") {
		t.Error("install.sh reads the human-readable status table as JSON")
	}
	if !strings.Contains(script, `'s/.*"version"`) {
		t.Error("install.sh does not extract the version field from status JSON")
	}
}
