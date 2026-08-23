package buildcontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// **綴りは中身を保証しない。**
//
// `verify-artifact-name` が見るのはファイル名だけであり、nativebuild/machine.go
// 自身が「`sshc-linux-arm64` という名前の amd64 バイナリはその検査を通る」と
// 書いている。束の smoke を消したあと、リリースは `make test` → build → upload
// だけになり、**上げるバイナリを一度も起動しなかった。**
//
// smoke が確かめるのは、開発機の go test では出ない類の壊れ方である:
// 版が入っていない（-X が外れる）、画面が入っていない（go:embed が空でも
// ビルドは通る）、engine が起きない。
func TestEveryReleaseJobSmokesWhatItBuilt(t *testing.T) {
	_, source := readReleaseWorkflow(t)

	for _, job := range []struct {
		id, next, invocation string
	}{
		{"  macos:", "  linux:", "scripts/ci/cli-smoke.sh dist"},
		{"  linux:", "  windows:", "scripts/ci/cli-smoke.sh dist"},
		{"  windows:", "  android:", "./scripts/ci/cli-smoke.ps1 -DistDir dist"},
	} {
		section := jobSection(source, job.id, job.next)
		if section == "" {
			t.Errorf("release.yml に %s のジョブが無い", strings.TrimSpace(job.id))
			continue
		}
		if !strings.Contains(section, job.invocation) {
			t.Errorf("%s は作ったバイナリを起こしていない（%s が無い）",
				strings.TrimSpace(job.id), job.invocation)
		}
		// **版を渡していなければ、-X が外れても気づけない。**
		if !strings.Contains(section, "GITHUB_REF_NAME") && !strings.Contains(section, "github.ref_name") {
			t.Errorf("%s は smoke にタグの版を渡していない", strings.TrimSpace(job.id))
		}
	}
}

// **2 本の smoke は、同じものを見ていなければならない。**
//
// POSIX のシェルと PowerShell で別々に書いてあるので、片方にだけ検査を足すと、
// その OS でだけ通る壊れ方ができる。**別々に書くことは、違うものを見てよい理由に
// ならない。** ここが数えるのは、両方が持っている必要のある 4 つの問いである。
func TestBothSmokeScriptsAskTheSameQuestions(t *testing.T) {
	read := func(name string) string {
		path := filepath.Join("..", "..", "scripts", "ci", name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return string(body)
	}
	shell := read("cli-smoke.sh")
	powershell := read("cli-smoke.ps1")

	for _, question := range []struct {
		what          string
		unix, windows string
	}{
		{"版を名乗れること", `"sshc $expected $goos/$goarch"`, `"sshc $ExpectedVersion $goos/$goarch"`},
		{"engine が居ないときに次の一手を言えること", `"sshc engine"`, `"*sshc engine*"`},
		{"起こすと handoff を出すこと", `.ssh/sshc/cli`, `.ssh\sshc\cli`},
		{"status が走っている engine を報告すること", `"running (pid"`, `"*running (pid*"`},
		{"入口が画面を返すこと", `<div id="root">`, `<div id="root">`},
	} {
		if !strings.Contains(shell, question.unix) {
			t.Errorf("cli-smoke.sh が「%s」を見ていない", question.what)
		}
		if !strings.Contains(powershell, question.windows) {
			t.Errorf("cli-smoke.ps1 が「%s」を見ていない", question.what)
		}
	}

	// **PowerShell は native command の非ゼロ終了で止まらない。** 明示しないと、
	// 落ちた行の後も進んで step は緑になる。
	for _, required := range []string{"$ErrorActionPreference = 'Stop'", "$PSNativeCommandUseErrorActionPreference = $true"} {
		if !strings.Contains(powershell, required) {
			t.Errorf("cli-smoke.ps1 に %s が無い。落ちても緑になる", required)
		}
	}
	if !strings.Contains(shell, "set -euo pipefail") {
		t.Error("cli-smoke.sh が set -euo pipefail を言っていない")
	}
}
