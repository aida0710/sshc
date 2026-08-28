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

func TestAndroidPrivateStateIsExcludedFromBackupAndDeviceTransfer(t *testing.T) {
	manifest := readContractFile(t, "android", "app", "src", "main", "AndroidManifest.xml")
	for _, required := range []string{
		`android:allowBackup="false"`,
		`android:fullBackupContent="@xml/backup_rules"`,
		`android:dataExtractionRules="@xml/data_extraction_rules"`,
	} {
		if !strings.Contains(manifest, required) {
			t.Errorf("Android manifest lacks private-state backup boundary %q", required)
		}
	}

	legacy := readContractFile(t, "android", "app", "src", "main", "res", "xml", "backup_rules.xml")
	modern := readContractFile(t, "android", "app", "src", "main", "res", "xml", "data_extraction_rules.xml")
	for _, domain := range []string{
		"root", "file", "database", "sharedpref", "external",
		"device_root", "device_file", "device_database", "device_sharedpref",
	} {
		exclusion := `<exclude domain="` + domain + `" path="." />`
		if !strings.Contains(legacy, exclusion) {
			t.Errorf("legacy backup rules do not exclude %s", domain)
		}
		if count := strings.Count(modern, exclusion); count != 2 {
			t.Errorf("Android 12 rules exclude %s %d times, want cloud backup and device transfer", domain, count)
		}
	}
	for _, section := range []string{"<cloud-backup>", "<device-transfer>"} {
		if !strings.Contains(modern, section) {
			t.Errorf("Android 12 extraction rules lack %s", section)
		}
	}
}

func TestAndroidDataSyncEngineHasABoundedLifetime(t *testing.T) {
	manifest := readContractFile(t, "android", "app", "src", "main", "AndroidManifest.xml")
	if !strings.Contains(manifest, `android:foregroundServiceType="dataSync"`) ||
		!strings.Contains(manifest, `android:stopWithTask="true"`) {
		t.Fatal("the dataSync foreground engine must stop with its user task")
	}

	service := readContractFile(t, "android", "app", "src", "main", "java", "com", "github", "aida0710", "sshc", "EngineService.java")
	for _, required := range []string{
		"public void onTimeout(int startId, int foregroundServiceType)",
		"stopServiceAndEngine(startId);",
		"shutdown.request();",
		"Executors.newSingleThreadExecutor",
		"ENGINE.execute(this::startEngine);",
		"stopForeground(STOP_FOREGROUND_REMOVE);",
		"stopSelf(startId);",
		"return START_NOT_STICKY;",
	} {
		if !strings.Contains(service, required) {
			t.Errorf("bounded foreground lifecycle lacks %q", required)
		}
	}
	if strings.Contains(service, "return START_STICKY;") {
		t.Error("the foreground engine still asks Android to recreate it as a permanent service")
	}
	if strings.Contains(service, "private void shutdown()") {
		t.Error("the Android main looper still performs the blocking Go shutdown itself")
	}

	activity := readContractFile(t, "android", "app", "src", "main", "java", "com", "github", "aida0710", "sshc", "MainActivity.java")
	for _, required := range []string{
		"long failure = service.failure();",
		"showFailure(failure, service.failureCode(), service.failureDetail());",
		"startForegroundService(new Intent(this, EngineService.class));",
		"service.retry()",
		"unbindService(connection);",
	} {
		if !strings.Contains(activity, required) {
			t.Errorf("failed engine start cannot expose diagnostics and retry safely: lacks %q", required)
		}
	}
	if strings.Contains(activity, "releaseService();\n        showFailure") {
		t.Error("failure screen unbinds the stopped service, so its in-place retry cannot work")
	}
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
		"actions: read",
		"attestations: write",
		"id-token: write",
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
	if count := strings.Count(workflow, "    environment: release"); count != 3 {
		t.Errorf("Android signing, release staging, and Homebrew must all use the release environment; found %d jobs", count)
	}
	start := strings.Index(workflow, "  homebrew:")
	if start < 0 {
		t.Fatal("release workflow has no Homebrew job")
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
		"scripts/release/publish.sh v0.18.0",
		"scripts/release/publish.sh --verify-only v0.17.3",
		"tagを動かしたり削除したりせず終了",
	} {
		if !strings.Contains(documentation, required) {
			t.Errorf("release operator documentation lacks %q", required)
		}
	}
}
