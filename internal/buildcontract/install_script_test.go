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
			// Windows は専用のPowerShell installerへ案内する。成果物名との一致は
			// install.ps1の契約テストで検証する。
			if !strings.Contains(script, "releases/latest/download/install.ps1") {
				t.Error("install.sh does not direct Windows users to install.ps1")
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

func readWindowsInstallScript(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestTheWindowsInstallerVerifiesAndSafelyPublishesEveryWindowsArtifact(t *testing.T) {
	contract := readMakefileContract(t)
	script := readWindowsInstallScript(t)
	for _, target := range contract.variables["RELEASE_TARGETS"] {
		platform, _, _ := strings.Cut(target, ":")
		goos, goarch, ok := strings.Cut(platform, "/")
		if ok && goos == "windows" && !strings.Contains(script, fmt.Sprintf("'%s'", goarch)) {
			t.Errorf("install.ps1 does not recognize windows/%s", goarch)
		}
	}
	for _, required := range []string{
		"checksums.txt",
		"Get-FileHash",
		"SHA256",
		"File]::Replace",
		"previous executable was left unchanged",
		"LocalApplicationData",
		"SetEnvironmentVariable('Path'",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("install.ps1 contract lacks %q", required)
		}
	}
	verify := strings.Index(script, "does not match its published checksum")
	publish := strings.Index(script, "Publish-Executable $download $target")
	if verify < 0 || publish < 0 || verify > publish {
		t.Error("install.ps1 publishes before checking the download")
	}
}

func TestTheDocumentedWindowsInstallerComesFromTheGitHubRelease(t *testing.T) {
	for _, path := range []string{"README.md", filepath.Join("docs", "release-install.md"), "install.ps1"} {
		body, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if !strings.Contains(text, "https://github.com/aida0710/sshc/releases/latest/download/install.ps1") {
			t.Errorf("%s lacks the GitHub Release installer URL", path)
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
	install := strings.Index(script, `mv "$staged" "$target"`)
	if install < 0 || strings.Index(script, "published checksum") > install {
		t.Error("install.sh installs before it verifies the checksum")
	}
}

func TestTheInstallScriptPublishesAtomicallyAndWritesABoundReceipt(t *testing.T) {
	script := readInstallScript(t)
	for _, required := range []string{
		`mktemp "$dir/.sshc.install.XXXXXX"`,
		`mv "$staged" "$target"`,
		`.sshc-install-receipt.json`,
		`"manager":"install.sh"`,
		`"sha256":"%s"`,
		`"$incoming" "$actual"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("atomic install/receipt contract lacks %q", required)
		}
	}
	for _, forbidden := range []string{`mv "$work/sshc" "$target"`, `"$target.$$"`} {
		if strings.Contains(script, forbidden) {
			t.Errorf("install.sh still uses unsafe publication %q", forbidden)
		}
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

func TestTheDocumentedInstallerPinsScriptAndArtifactsToOneRelease(t *testing.T) {
	for _, path := range []string{"README.md", filepath.Join("docs", "release-install.md"), "install.sh"} {
		body, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if strings.Contains(text, "raw.githubusercontent.com/aida0710/sshc/main/install.sh") {
			t.Errorf("%s still executes the mutable main installer", path)
		}
		for _, required := range []string{"SSHC_VERSION=v0.27.2", "/sshc/v0.27.2/install.sh"} {
			if !strings.Contains(text, required) {
				t.Errorf("%s lacks version-pinned installer fragment %q", path, required)
			}
		}
	}

	script := readInstallScript(t)
	for _, required := range []string{"SSHC_VERSION is not a semantic version", "grep -Eq '^v(0|[1-9][0-9]*)"} {
		if !strings.Contains(script, required) {
			t.Errorf("install.sh does not reject an unsafe version input: lacks %q", required)
		}
	}
}

func TestStartupScriptStartsTheForegroundEngineWithoutUpdatingSource(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "startup.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(body)
	if !strings.Contains(script, "exec ./bin/sshc engine") {
		t.Error("startup.sh builds the CLI but does not start the engine")
	}
	for _, forbidden := range []string{"\ngit pull", "\nmake update", "exec ./bin/sshc\n"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("startup.sh mixes startup with %q", forbidden)
		}
	}
}
