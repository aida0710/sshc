//go:build !windows

package buildcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// verifiedReleaseTag は、publish.sh --verify-onlyへ渡す架空の公開済みtagである。
const verifiedReleaseTag = "v9.8.7"

// publishedReleaseAssets は、publish.shが公開済みReleaseに求める成果物の集合である。
func publishedReleaseAssets() []string {
	return []string{
		"checksums.txt",
		"install.ps1",
		"sshc-android-" + verifiedReleaseTag + ".apk",
		"sshc-demo-" + verifiedReleaseTag + ".tar.gz",
		"sshc-demo-" + verifiedReleaseTag + ".json",
		"sshc-darwin-amd64",
		"sshc-darwin-arm64",
		"sshc-linux-amd64",
		"sshc-linux-arm64",
		"sshc-windows-amd64.exe",
		"sshc-windows-arm64.exe",
	}
}

// writePublishedReleaseFixtures は、gh release downloadが落とす成果物と、それを照合できる
// checksums.txtを作る。
func writePublishedReleaseFixtures(t *testing.T, directory string) {
	t.Helper()
	var checksums strings.Builder
	for _, name := range publishedReleaseAssets() {
		if name == "checksums.txt" {
			continue
		}
		body := []byte("fixture " + name + "\n")
		if err := os.WriteFile(filepath.Join(directory, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		checksums.WriteString(hex.EncodeToString(digest[:]) + "  " + name + "\n")
	}
	if err := os.WriteFile(filepath.Join(directory, "checksums.txt"), []byte(checksums.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// publishedReleaseMetadata は、gh apiが返す公開済みでimmutableなReleaseのJSONである。
func publishedReleaseMetadata(t *testing.T) []byte {
	t.Helper()
	type asset struct {
		Name  string `json:"name"`
		State string `json:"state"`
		Size  int    `json:"size"`
	}
	release := struct {
		TagName    string  `json:"tag_name"`
		Draft      bool    `json:"draft"`
		Prerelease bool    `json:"prerelease"`
		Immutable  bool    `json:"immutable"`
		Assets     []asset `json:"assets"`
	}{TagName: verifiedReleaseTag, Immutable: true}
	for _, name := range publishedReleaseAssets() {
		release.Assets = append(release.Assets, asset{Name: name, State: "uploaded", Size: 1})
	}
	body, err := json.Marshal(release)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// 公開後の検証は途中のdieやset -eで終わることが多い。そのたびにReleaseから落とした
// 成果物一式を一時ディレクトリに残すと、調べ直すたびに溜まり、場所も表示されない。
func TestReleaseVerificationRemovesTheDownloadWhenItFails(t *testing.T) {
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required to run publish.sh", tool)
		}
	}
	jq, _ := exec.LookPath("jq")

	for _, test := range []struct {
		name string
		// failure は偽のghとunzipに、どこで失敗するかを伝える。
		failure string
		// wantOutput は、その場所で止まったことを示す出力である。空なら出力は見ない。
		wantOutput string
	}{
		{name: "download stops under set -e", failure: "download"},
		{name: "APK check calls die", failure: "apk", wantOutput: "APK archive verification failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			commands := filepath.Join(root, "commands")
			fixtures := filepath.Join(root, "fixtures")
			repository := filepath.Join(root, "repository")
			temporaryDirectory := filepath.Join(root, "tmp")
			for _, directory := range []string{commands, fixtures, repository, temporaryDirectory} {
				if err := os.MkdirAll(directory, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			writePublishedReleaseFixtures(t, fixtures)
			ghCalls := filepath.Join(root, "gh-calls")
			metadata := filepath.Join(root, "release.json")
			if err := os.WriteFile(metadata, publishedReleaseMetadata(t), 0o644); err != nil {
				t.Fatal(err)
			}

			fakeCommands := map[string]string{
				"git": "#!/bin/sh\nprintf '%s\\n' \"$SSHC_TEST_REPOSITORY\"\n",
				"gh": `#!/bin/sh
printf '%s\n' "$1 $2" >> "$SSHC_TEST_GH_CALLS"
case "$1 $2" in
  "auth status") exit 0 ;;
  "api repos/aida0710/sshc/releases/tags/` + verifiedReleaseTag + `") cat "$SSHC_TEST_RELEASE_METADATA" ;;
  "release download")
    while [ "$#" -gt 0 ]; do
      case "$1" in --dir) directory=$2; shift 2 ;; *) shift ;; esac
    done
    cp "$SSHC_TEST_FIXTURES"/* "$directory"/
    [ "$SSHC_TEST_FAILURE" != download ] || exit 1
    ;;
  "attestation verify") exit 0 ;;
  *) exit 1 ;;
esac
`,
				"unzip": "#!/bin/sh\n[ \"$SSHC_TEST_FAILURE\" != apk ]\n",
				"curl":  "#!/bin/sh\nexit 1\n",
			}
			for name, body := range fakeCommands {
				if err := os.WriteFile(filepath.Join(commands, name), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "publish.sh"))
			if err != nil {
				t.Fatal(err)
			}
			command := exec.Command("bash", script, "--verify-only", verifiedReleaseTag)
			command.Env = append(os.Environ(),
				"PATH="+commands+":"+filepath.Dir(jq)+":/usr/bin:/bin",
				"TMPDIR="+temporaryDirectory,
				"SSHC_RELEASE_REPOSITORY=aida0710/sshc",
				"SSHC_TEST_REPOSITORY="+repository,
				"SSHC_TEST_RELEASE_METADATA="+metadata,
				"SSHC_TEST_FIXTURES="+fixtures,
				"SSHC_TEST_FAILURE="+test.failure,
				"SSHC_TEST_GH_CALLS="+ghCalls,
			)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("publish.sh --verify-only succeeded although the %s check failed:\n%s", test.failure, output)
			}
			calls, err := os.ReadFile(ghCalls)
			if err != nil {
				t.Fatal(err)
			}
			// 一時ディレクトリを作る前に止まったのなら、このテストは何も確かめていない。
			if !strings.Contains(string(calls), "release download") ||
				(test.wantOutput != "" && !strings.Contains(string(output), test.wantOutput)) {
				t.Fatalf("publish.sh stopped somewhere other than the %s check:\n%s", test.failure, output)
			}
			left, err := os.ReadDir(temporaryDirectory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range left {
				t.Errorf("publish.sh left %s in TMPDIR after the %s check failed", entry.Name(), test.failure)
			}
		})
	}
}

// publishedCommit は、publish.sh の公開の手順に渡す架空の HEAD と origin/main の SHA である。
const publishedCommit = "0123456789abcdef0123456789abcdef01234567"

// publishedPrereleaseTag は、公開の手順を走らせる架空の tag である。安定バージョンだけが走らせる
// 導入例の照合（check-pinned-installers.sh）を、このテストの外に置くためにプレリリースにする。
const publishedPrereleaseTag = "v9.8.7-rc.1"

// writeFreshVPNSnapshot は、publish.sh が古さを警告する VPN イメージの Ubuntu snapshot を、
// 今日の日付で置く。警告の行でテストの出力を埋めないためである。
func writeFreshVPNSnapshot(t *testing.T, repository string) {
	t.Helper()
	directory := filepath.Join(repository, "internal", "vpn", "container")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot := "snapshot=" + time.Now().UTC().Format("20060102") + "T000000Z\n"
	if err := os.WriteFile(filepath.Join(directory, "Dockerfile"), []byte(snapshot), 0o644); err != nil {
		t.Fatal(err)
	}
}

// workflowRun は、gh api が返す workflow の run の一覧の 1 行である。
type workflowRun struct {
	ID         int    `json:"id"`
	HeadSHA    string `json:"head_sha"`
	HeadBranch string `json:"head_branch"`
	Event      string `json:"event"`
	CreatedAt  string `json:"created_at"`
}

// workflowRunList は、gh api repos/…/actions/workflows/<file>/runs の応答の JSON である。
func workflowRunList(t *testing.T, runs ...workflowRun) string {
	t.Helper()
	body, err := json.Marshal(struct {
		WorkflowRuns []workflowRun `json:"workflow_runs"`
	}{WorkflowRuns: append([]workflowRun{}, runs...)})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// 公開する commit でリリースの runner の埋め込み UI の照合（release-ui-check.yml）が
// 成功していなければ、publish.sh は tag を作らずに止まる。照合を走らせ忘れても、
// 失敗を見落としても、公開の当日に Release の途中で止まることになるためである。
func TestPublishTagsOnlyACommitWhoseReleaseUICheckSucceeded(t *testing.T) {
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required to run publish.sh", tool)
		}
	}
	jq, _ := exec.LookPath("jq")
	const uiCheckRunID = 22
	ciRuns := workflowRunList(t, workflowRun{
		ID: 11, HeadSHA: publishedCommit, HeadBranch: "main", Event: "push", CreatedAt: "2026-10-01T00:00:00Z",
	})
	uiCheckOnTheCommit := workflowRun{
		ID: uiCheckRunID, HeadSHA: publishedCommit, HeadBranch: "main", Event: "workflow_dispatch", CreatedAt: "2026-10-01T00:10:00Z",
	}
	uiCheckOnAnotherCommit := uiCheckOnTheCommit
	uiCheckOnAnotherCommit.HeadSHA = strings.Repeat("f", 40)
	uiCheckOnAnotherBranch := uiCheckOnTheCommit
	uiCheckOnAnotherBranch.HeadBranch = "fix/release-ui"

	for _, test := range []struct {
		name        string
		uiCheckRuns string
		conclusion  string
		wantTag     bool
		wantOutput  string
	}{
		{name: "the check was never run", uiCheckRuns: workflowRunList(t),
			wantOutput: "no Release UI check run exists for " + publishedCommit},
		{name: "the check ran on another commit", uiCheckRuns: workflowRunList(t, uiCheckOnAnotherCommit),
			wantOutput: "no Release UI check run exists for " + publishedCommit},
		{name: "the check ran on another branch", uiCheckRuns: workflowRunList(t, uiCheckOnAnotherBranch),
			wantOutput: "no Release UI check run exists for " + publishedCommit},
		{name: "the check failed", uiCheckRuns: workflowRunList(t, uiCheckOnTheCommit), conclusion: "failure",
			wantOutput: "Release UI check failed"},
		// 通ったあとは tag を作って push する。偽の git は push を断るので、そこで止まる。
		{name: "the check succeeded", uiCheckRuns: workflowRunList(t, uiCheckOnTheCommit), conclusion: "success",
			wantTag: true, wantOutput: "tag push failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			commands := filepath.Join(root, "commands")
			repository := filepath.Join(root, "repository")
			for _, directory := range []string{commands, filepath.Join(repository, "docs", "releases")} {
				if err := os.MkdirAll(directory, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			notes := filepath.Join(repository, "docs", "releases", publishedPrereleaseTag+".md")
			if err := os.WriteFile(notes, []byte("# "+publishedPrereleaseTag+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			writeFreshVPNSnapshot(t, repository)
			gitCalls := filepath.Join(root, "git-calls")

			fakeCommands := map[string]string{
				"git": `#!/bin/sh
printf '%s\n' "$*" >> "$SSHC_TEST_GIT_CALLS"
case "$1 $2" in
  "rev-parse --show-toplevel") printf '%s\n' "$SSHC_TEST_REPOSITORY" ;;
  "rev-parse HEAD"|"rev-parse refs/remotes/origin/main") printf '%s\n' "$SSHC_TEST_COMMIT" ;;
  "push origin") exit 1 ;;
esac
exit 0
`,
				"gh": `#!/bin/sh
[ "$1 $2" != "auth status" ] || exit 0
for argument in "$@"; do
  case "$argument" in repos/*) path=$argument ;; esac
done
case "$path" in
  */actions/workflows/ci.yml/runs) printf '%s' "$SSHC_TEST_CI_RUNS" ;;
  */actions/workflows/release-ui-check.yml/runs) printf '%s' "$SSHC_TEST_UI_CHECK_RUNS" ;;
  */actions/runs/11) printf '{"status":"completed","conclusion":"success"}' ;;
  */actions/runs/22) printf '{"status":"completed","conclusion":"%s"}' "$SSHC_TEST_UI_CHECK_CONCLUSION" ;;
  *) exit 1 ;;
esac
`,
				"unzip": "#!/bin/sh\nexit 1\n",
				"curl":  "#!/bin/sh\nexit 1\n",
			}
			for name, body := range fakeCommands {
				if err := os.WriteFile(filepath.Join(commands, name), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release", "publish.sh"))
			if err != nil {
				t.Fatal(err)
			}
			command := exec.Command("bash", script, publishedPrereleaseTag)
			command.Env = append(os.Environ(),
				"PATH="+commands+":"+filepath.Dir(jq)+":/usr/bin:/bin",
				"SSHC_RELEASE_REPOSITORY=aida0710/sshc",
				"SSHC_RELEASE_POLL_SECONDS=0",
				"SSHC_TEST_REPOSITORY="+repository,
				"SSHC_TEST_COMMIT="+publishedCommit,
				"SSHC_TEST_GIT_CALLS="+gitCalls,
				"SSHC_TEST_CI_RUNS="+ciRuns,
				"SSHC_TEST_UI_CHECK_RUNS="+test.uiCheckRuns,
				"SSHC_TEST_UI_CHECK_CONCLUSION="+test.conclusion,
			)
			output, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("publish.sh succeeded against a fake that refuses the tag push:\n%s", output)
			}
			if !strings.Contains(string(output), test.wantOutput) {
				t.Fatalf("publish.sh did not say %q:\n%s", test.wantOutput, output)
			}
			calls, err := os.ReadFile(gitCalls)
			if err != nil {
				t.Fatal(err)
			}
			tagged := strings.Contains(string(calls), "tag -a "+publishedPrereleaseTag+" "+publishedCommit)
			if tagged != test.wantTag {
				t.Fatalf("publish.sh created the tag = %t, want %t:\n%s", tagged, test.wantTag, output)
			}
		})
	}
}
