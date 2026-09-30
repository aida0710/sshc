package buildcontract

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readContractFile(t *testing.T, path ...string) string {
	t.Helper()
	parts := append([]string{"..", ".."}, path...)
	body, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestGradleDistributionAndDependenciesAreChecksumPinned(t *testing.T) {
	wrapper := readContractFile(t, "android", "gradle", "wrapper", "gradle-wrapper.properties")
	if !strings.Contains(wrapper, "distributionSha256Sum=84fbba45c7f4c64abc77460e1c00f541e9f960e3c7ed2538f1ede19eacd873ae") {
		t.Error("Gradle 9.7.0 distribution SHA-256 is not pinned")
	}
	jar, err := os.ReadFile(filepath.Join("..", "..", "android", "gradle", "wrapper", "gradle-wrapper.jar"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(jar)); got != "7a9ce74cff467ca1bf60a4fcd9f05185acceda4d0f382434d393e17864262c5d" {
		t.Errorf("Gradle wrapper JAR SHA-256 = %s, want the official 9.7.0 wrapper", got)
	}
	metadata := readContractFile(t, "android", "gradle", "verification-metadata.xml")
	for _, required := range []string{
		"<verify-metadata>true</verify-metadata>",
		`group="com.android.tools.build" name="gradle" version="9.3.1"`,
		`group="junit" name="junit" version="4.13.2"`,
	} {
		if !strings.Contains(metadata, required) {
			t.Errorf("Gradle dependency verification lacks %q", required)
		}
	}
}

func TestCIAndReleaseUseOnePinnedAndroidNDK(t *testing.T) {
	version := strings.TrimSpace(readContractFile(t, ".github", "android-ndk-version"))
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(version) {
		t.Fatalf("Android NDK revision is not canonical: %q", version)
	}
	for _, workflowPath := range [][]string{
		{".github", "workflows", "ci.yml"},
		{".github", "workflows", "release.yml"},
	} {
		workflow := readContractFile(t, workflowPath...)
		for _, required := range []string{
			"scripts/ci/prepare-android-ndk.sh",
			"SSHC_ANDROID_NDK_HOME: ${{ steps.android-ndk.outputs.path }}",
			`ANDROID_NDK_HOME="$SSHC_ANDROID_NDK_HOME"`,
		} {
			if !strings.Contains(workflow, required) {
				t.Errorf("%s does not use the pinned NDK boundary %q", filepath.Join(workflowPath...), required)
			}
		}
		if strings.Contains(workflow, "ANDROID_NDK_LATEST_HOME") {
			t.Errorf("%s still accepts the runner's moving latest NDK", filepath.Join(workflowPath...))
		}
	}

	prepare := readContractFile(t, "scripts", "ci", "prepare-android-ndk.sh")
	for _, required := range []string{
		`.github/android-ndk-version`,
		`"ndk;$version"`,
		`source.properties`,
		`Pkg.Revision`,
	} {
		if !strings.Contains(prepare, required) {
			t.Errorf("NDK preparation does not verify %q", required)
		}
	}
}

func TestReleaseRequiresExactSHACIAndAuthenticatedArtifacts(t *testing.T) {
	workflow := readContractFile(t, ".github", "workflows", "release.yml")
	for _, required := range []string{
		"verify-source:",
		"needs: [verify-source]",
		"scripts/ci/verify-release-source.sh",
		"actions/attest-build-provenance@4d101475d8b20a2381f78447822ac1eab6504dd8",
		".github/android-release-signer.sha256",
		"the APK was signed by an unexpected certificate",
		".github/github.com.known_hosts",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release supply-chain boundary lacks %q", required)
		}
	}
	if strings.Contains(workflow, "ssh-keyscan github.com") {
		t.Error("Homebrew release trusts a host key learned from the same unauthenticated connection")
	}

	gate := readContractFile(t, "scripts", "ci", "verify-release-source.sh")
	for _, required := range []string{
		`git merge-base --is-ancestor "$RELEASE_SHA" refs/remotes/origin/main`,
		`actions/workflows/ci.yml/runs?head_sha=$RELEASE_SHA`,
		`.head_sha == $sha and`,
		`.head_branch == "main" and`,
		`(.event == "push" or .event == "workflow_dispatch") and`,
		`.conclusion == "success"`,
	} {
		if !strings.Contains(gate, required) {
			t.Errorf("exact-SHA release gate lacks %q", required)
		}
	}

	fingerprint := strings.TrimSpace(readContractFile(t, ".github", "android-release-signer.sha256"))
	if !regexp.MustCompile(`^[0-9A-F]{64}$`).MatchString(fingerprint) {
		t.Errorf("Android release signer fingerprint is not canonical SHA-256: %q", fingerprint)
	}

	knownHosts := readContractFile(t, ".github", "github.com.known_hosts")
	for _, algorithm := range []string{"ssh-ed25519", "ecdsa-sha2-nistp256", "ssh-rsa"} {
		if !strings.Contains(knownHosts, "github.com "+algorithm+" ") {
			t.Errorf("pinned GitHub host keys lack %s", algorithm)
		}
	}
}

func TestReleaseFailsWhenTheHomebrewDeployKeyIsMissing(t *testing.T) {
	workflow := readContractFile(t, ".github", "workflows", "release.yml")
	if count := strings.Count(workflow, "    environment: release"); count != 1 {
		t.Errorf("Android signing, release staging, and Homebrew must share one protected job; found %d jobs", count)
	}
	start := strings.Index(workflow, "  stage-release:")
	if start < 0 {
		t.Fatal("release workflow has no protected staging job")
	}
	homebrew := workflow[start:]
	for _, required := range []string{
		`if [ -z "${TAP_KEY:-}" ]`,
		`the release cannot update its required tap`,
		`exit 1`,
	} {
		if !strings.Contains(homebrew, required) {
			t.Errorf("Homebrew deploy-key failure boundary lacks %q", required)
		}
	}
	if strings.Contains(homebrew, "leaving the tap alone") {
		t.Error("a missing Homebrew deploy key still reports release success")
	}
}

func TestOperatorReleaseScriptPreservesTheReleaseGates(t *testing.T) {
	script := readContractFile(t, "scripts", "release", "publish.sh")
	for _, required := range []string{
		`[ -z "$(git status --porcelain)" ]`,
		`[ "$head_sha" = "$remote_main" ]`,
		`actions/workflows/ci.yml/runs`,
		`.head_sha == $sha and .head_branch == "main"`,
		`git tag -a "$tag" "$head_sha"`,
		`git push origin "refs/tags/$tag"`,
		`.environment.name == "release"`,
		`state=approved`,
		`.immutable == true`,
		`gh attestation verify "$artifact"`,
		`--cert-identity-regex "$signer_pattern" --deny-self-hosted-runners`,
		`/\\.github/workflows/release\\.yml@refs/(tags/%s|heads/main)$`,
		`verify_checksum_file`,
		`Homebrew source checksum does not match`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("operator release script lacks %q", required)
		}
	}
	for _, forbidden := range []string{"git push --force", "git tag -f", "git push -f"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("operator release script can rewrite protected history with %q", forbidden)
		}
	}

	documentation := readContractFile(t, "docs", "releasing.md")
	for _, required := range []string{
		"scripts/release/publish.sh v0.33.2",
		"scripts/release/publish.sh --verify-only v0.17.3",
		"タグを動かしたり削除したりせず終了",
	} {
		if !strings.Contains(documentation, required) {
			t.Errorf("release operator documentation lacks %q", required)
		}
	}
}

// publish.shは確認の入力なしにrelease environmentを承認する（上のテストがstate=approvedを確かめる）。
// 保護設定の説明が「tagのpushとは別に人が公開ごとに承認する」と読めると、
// required reviewerがもう1段の判断になっていると誤解させる。
func TestReleaseProtectionDocumentationCountsPublishingAsTheApproval(t *testing.T) {
	documentation := readContractFile(t, "docs", "release-install.md")
	for _, required := range []string{
		"`scripts/release/publish.sh`を実行したことを、その公開の承認とみなす",
		"publish.sh以外から始まったrun",
	} {
		if !strings.Contains(documentation, required) {
			t.Errorf("release protection documentation lacks %q", required)
		}
	}
}

// --repoだけの検証は、同じリポジトリのほかのbranchやworkflowが署名したattestationも通す。
// 利用者に案内する検証は、署名したRelease workflowとtagのrefまで指定する。
func TestDocumentedAttestationCheckNamesTheReleaseWorkflowAndTag(t *testing.T) {
	for _, path := range [][]string{{"docs", "release-install.md"}, {"docs", "design.md"}} {
		documentation := readContractFile(t, path...)
		for _, required := range []string{
			"--signer-workflow aida0710/sshc/.github/workflows/release.yml",
			"--source-ref refs/tags/<tag>",
			"--deny-self-hosted-runners",
			"--source-ref refs/heads/main",
		} {
			if !strings.Contains(documentation, required) {
				t.Errorf("%s does not pin the attestation check with %q", strings.Join(path, "/"), required)
			}
		}
	}
}

// Release workflowの一時的な失敗では、同じtagで作り直す。新しいpatch versionを作ると、
// READMEや導入例で固定したバージョンを書き換え、reviewを通し直すことになる。
func TestReleaseDocumentationRebuildsTheSameTagAfterATransientFailure(t *testing.T) {
	documentation := readContractFile(t, "docs", "releasing.md")
	for _, required := range []string{
		"gh run rerun <run-id> --failed",
		"gh workflow run release.yml --repo aida0710/sshc --ref main -f tag=<tag>",
		"scripts/release/publish.sh --verify-only <tag>",
		"新しいパッチバージョンが必要になるのは、タグのコミット自体に不具合がある場合だけ",
	} {
		if !strings.Contains(documentation, required) {
			t.Errorf("release operator documentation lacks the recovery step %q", required)
		}
	}
}

// tagのpush後にRelease workflowが見つからずに止まったときは、tagがもうあるのでpublish.shを
// 実行し直しても進まない。止まった理由の文から、遅れて始まったrunを探す手順へたどれるようにする。
func TestReleaseDocumentationRecoversAWorkflowThatDidNotStart(t *testing.T) {
	script := readContractFile(t, "scripts", "release", "publish.sh")
	notStarted := strings.Index(script, "release workflow did not start after the tag push")
	if notStarted < 0 {
		t.Fatal("publish.sh no longer reports a Release workflow that did not start")
	}
	if message, _, _ := strings.Cut(script[notStarted:], "\n"); !strings.Contains(message, "docs/releasing.md") {
		t.Errorf("publish.sh does not point a Release workflow that did not start to docs/releasing.md: %s", message)
	}

	documentation := readContractFile(t, "docs", "releasing.md")
	for _, required := range []string{
		"### タグのpush後にReleaseワークフローが始まらない場合",
		"gh run list --repo aida0710/sshc --workflow release.yml --branch <tag>",
	} {
		if !strings.Contains(documentation, required) {
			t.Errorf("release operator documentation lacks the recovery step %q", required)
		}
	}
}
