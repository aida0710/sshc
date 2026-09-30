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
)

// verifiedReleaseTag は、publish.sh --verify-onlyへ渡す架空の公開済みtagである。
const verifiedReleaseTag = "v9.8.7"

// publishedReleaseAssets は、publish.shが公開済みReleaseに求める成果物の集合である。
func publishedReleaseAssets() []string {
	return []string{
		"checksums.txt",
		"install.ps1",
		"sshc-android-" + verifiedReleaseTag + ".apk",
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
