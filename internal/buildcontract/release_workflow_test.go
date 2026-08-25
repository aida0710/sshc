package buildcontract

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func releaseWorkflowPath() string {
	return filepath.Join("..", "..", ".github", "workflows", "release.yml")
}

func readReleaseWorkflow(t *testing.T) (workflowDocument, string) {
	t.Helper()
	source, err := os.ReadFile(releaseWorkflowPath())
	if err != nil {
		t.Fatalf("read %s: %v", releaseWorkflowPath(), err)
	}
	document, err := decodeWorkflowDocument(source)
	if err != nil {
		t.Fatalf("decode %s: %v", releaseWorkflowPath(), err)
	}
	return document, string(source)
}

// タグ以外では何も公開されない。
//
// ここに branch の引き金が混ざると、main への push がそのままリリースになる。
// 版はタグから取るので、そうして出たものは版を名乗れないまま公開される。
func TestReleaseWorkflowRunsOnlyForTags(t *testing.T) {
	_, source := readReleaseWorkflow(t)

	trigger := withoutYAMLComments(source)
	if !strings.Contains(trigger, `tags: ["v*"]`) {
		t.Error("the release workflow does not trigger on v* tags")
	}
	if strings.Contains(trigger, "branches:") {
		t.Error("the release workflow triggers on a branch; every push to it would publish")
	}
	if strings.Contains(trigger, "workflow_dispatch") {
		t.Error("the release workflow can be started by hand, which publishes without a tag to name the version")
	}
}

// 束は、それが動く OS の上で作る。
//
// 一台の macOS から四つ作っていたことがあり、そのとき Windows の
// インストーラは存在しなかった。NSIS は Windows でしか組めない。
// この表が縮むと、縮んだぶんが暗黙に配布物から消える。
func TestReleaseWorkflowBuildsEveryPlatformNatively(t *testing.T) {
	document, _ := readReleaseWorkflow(t)

	for job, runner := range map[string]string{
		"macos":   "macos-14",
		"linux":   "ubuntu-24.04",
		"windows": "windows-2025",
		"android": "ubuntu-24.04",
	} {
		found, present := document.Jobs[job]
		if !present {
			t.Errorf("the release has no %s job; that platform would not be published at all", job)
			continue
		}
		if found.RunsOn != runner {
			t.Errorf("%s runs on %q, want %q", job, found.RunsOn, runner)
		}
	}
}

// 一つでも作れなければ、何も公開しない。
//
// 部分的なリリースは、利用者から見ると「その OS だけ対応が消えた版」に
// 見える。落ちたことが分かる方がよい。
func TestReleasePublishWaitsForEveryPlatform(t *testing.T) {
	document, _ := readReleaseWorkflow(t)

	publish, present := document.Jobs["publish"]
	if !present {
		t.Fatal("the release has no publish job")
	}
	for _, required := range []string{"macos", "linux", "windows", "android"} {
		found := false
		for _, need := range publish.Needs {
			if need == required {
				found = true
			}
		}
		if !found {
			t.Errorf("publish does not wait for %s; a release could go out without it", required)
		}
	}
}

func TestReleasePublishesOnlyAfterTheRequiredHomebrewTap(t *testing.T) {
	document, _ := readReleaseWorkflow(t)
	publish, publishPresent := document.Jobs["publish"]
	homebrew, homebrewPresent := document.Jobs["homebrew"]
	if !publishPresent || !homebrewPresent {
		t.Fatal("the release requires both publish and homebrew jobs")
	}
	if !slices.Contains(publish.Needs, "homebrew") {
		t.Error("publish does not wait for Homebrew; a tap failure could leave a public partial release")
	}
	if slices.Contains(homebrew.Needs, "publish") {
		t.Error("Homebrew still runs after publish; the public release is no longer the final commit point")
	}
	for _, required := range []string{"macos", "linux", "windows", "android"} {
		if !slices.Contains(homebrew.Needs, required) {
			t.Errorf("Homebrew does not wait for %s; the tap could point at an unverified release", required)
		}
	}
}

func TestReleaseCollectsTheExactPublicArtifactSet(t *testing.T) {
	_, source := readReleaseWorkflow(t)
	publish := jobSection(source, "  publish:", "  homebrew:")
	if publish == "" {
		t.Fatal("release.yml has no publish job")
	}
	for _, required := range []string{
		"sshc-darwin-amd64", "sshc-darwin-arm64",
		"sshc-linux-amd64", "sshc-linux-arm64",
		"sshc-windows-amd64.exe", "sshc-windows-arm64.exe",
		"sshc-android-${GITHUB_REF_NAME}.apk",
		`[ "$count" -eq 1 ]`,
		`[ "$count" -eq 7 ]`,
		`--verify-tag`,
		`--draft`,
		`jq -e '.draft == true'`,
		`gh api --method DELETE "repos/$GH_REPO/releases/$draft_id"`,
		`.draft == true`,
		`all(.assets[]; .size > 0)`,
		`gh release edit "$RELEASE_TAG" --draft=false --latest`,
	} {
		if !strings.Contains(publish, required) {
			t.Errorf("publish artifact gate is missing %q", required)
		}
	}
	if strings.Contains(publish, `-exec cp {} dist/`) {
		t.Error("publish still silently overwrites duplicate artifact basenames")
	}
	if strings.Index(publish, `gh release create "$RELEASE_TAG"`) > strings.Index(publish, `--draft=false --latest`) {
		t.Error("release publish happens before the complete draft is verified")
	}
}

// すべてのタグに、バージョン管理されたリリースノートを要求する。
func TestReleaseRequiresVersionControlledNotes(t *testing.T) {
	_, source := readReleaseWorkflow(t)
	publish := jobSection(source, "  publish:", "  homebrew:")
	if publish == "" {
		t.Fatal("release.yml has no publish job")
	}
	for _, required := range []string{
		`notes_file="docs/releases/$RELEASE_TAG.md"`,
		`[ ! -f "$notes_file" ]`,
		`--notes-file "$notes_file"`,
	} {
		if !strings.Contains(publish, required) {
			t.Errorf("publish job is missing %q", required)
		}
	}
	for _, forbidden := range []string{"--notes-from-tag", "--generate-notes"} {
		if strings.Contains(publish, forbidden) {
			t.Errorf("publish job still uses %q instead of version-controlled release notes", forbidden)
		}
	}
}

// 署名の無い APK は配布物ではない。
//
// インストールすらできないので、公開しても誰の役にも立たない。debug の鍵で
// 署名するのはもっと悪い。公開されている鍵なので、誰でも「同じアプリの
// 更新」を作れる。だから鍵が無ければ止まり、出来上がったものには
// apksigner が直接聞く。gradle が成功したことは、署名された証拠ではない。
func TestReleaseRefusesAnUnsignedAndroidPackage(t *testing.T) {
	document, _ := readReleaseWorkflow(t)

	android, present := document.Jobs["android"]
	if !present {
		t.Fatal("the release has no android job")
	}
	demandsKey, verifies := false, false
	for _, step := range android.Steps {
		if strings.Contains(step.Run, "ANDROID_KEYSTORE_BASE64") && strings.Contains(step.Run, "exit 1") {
			demandsKey = true
		}
		if strings.Contains(step.Run, "apksigner") && strings.Contains(step.Run, "verify") {
			verifies = true
		}
	}
	if !demandsKey {
		t.Error("the android job does not stop when no signing key is configured")
	}
	if !verifies {
		t.Error("the android job never asks the APK whether it is signed; gradle succeeding is not that answer")
	}
}

// PowerShell は native command の非ゼロ終了で止まらない。
//
// GitHub は step の最後の一つの終了値しか見ないので、前の行が落ちても step は
// 緑になる。実際にそれで、CLI の入っていないインストーラが「ビルド成功」の
// まま次の段へ渡った。捕まえたのは smoke であって、ビルドではない。
//
// 二つ以上の命令を並べる Windows の step は、必ず自分で止まるようにする。
func TestReleaseWindowsStepsStopAtTheFirstFailure(t *testing.T) {
	document, _ := readReleaseWorkflow(t)

	windows, present := document.Jobs["windows"]
	if !present {
		t.Fatal("the release has no windows job")
	}
	for _, step := range windows.Steps {
		commands := 0
		for _, line := range strings.Split(step.Run, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "$") {
				continue
			}
			commands++
		}
		if commands < 2 {
			continue
		}
		if !strings.Contains(step.Run, "PSNativeCommandUseErrorActionPreference") {
			t.Errorf("the Windows step %q runs %d commands without stopping at the first failure; "+
				"only the last exit code is read, so an earlier failure would pass as green",
				step.Name, commands)
		}
	}
}

// 配るものは、自分が何番かを言えなければならない。
//
// APK の versionName はタグから来ていたが、その中で走る engine（AAR）は誰も版を
// 入れておらず、どの版を配っても自分を "dev" と名乗っていた。通知にも handoff
// にも、`sshc status` にもそう出る。
//
// 二つを別々に渡せば「APK は 0.4.2 だが engine は dev」が作れてしまうので、
// 入力がタグひとつであることをここで見る。
func TestTheAndroidEngineCarriesTheReleasedVersion(t *testing.T) {
	_, source := readReleaseWorkflow(t)

	android := jobSection(source, "  android:", "  publish:")
	if android == "" {
		t.Fatal("release.yml に android のジョブが無い")
	}

	if !strings.Contains(android, `ANDROID_VERSION="${GITHUB_REF_NAME#v}"`) {
		t.Error("gomobile bind にタグの版を渡していない。AAR は dev のまま配られる")
	}
	if !strings.Contains(android, `-PsshcVersionName="${GITHUB_REF_NAME#v}"`) {
		t.Error("gradle にタグの版を渡していない")
	}

	makefile, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	// 受け取る側が在ることも見る。workflow が渡していても、Makefile が
	// それを ldflags に載せていなければ、値はどこにも届かない。
	if !strings.Contains(string(makefile), "-X sshc/mobile.version=$(ANDROID_VERSION)") {
		t.Error("Makefile の android-bind が ANDROID_VERSION を ldflags に載せていない")
	}
}
