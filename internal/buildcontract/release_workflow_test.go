package buildcontract

import (
	"os"
	"path/filepath"
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

// **タグ以外では何も公開されない。**
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

// **束は、それが動く OS の上で作る。**
//
// 一台の macOS から四つ作っていたことがあり、そのとき Windows の
// インストーラは存在しなかった——NSIS は Windows でしか組めない。
// この表が縮むと、縮んだぶんが黙って配布物から消える。
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

// **開いていない束を配らない。**
//
// 中身が入れ替わっていてもビルドは通る。実際に、Windows の資源の道が
// 合わなくなったとき、空の resources\cli が利用者の PATH に載った。
// smoke はその一段だけを見ている——束を開いて、動かして、確かめる。
func TestReleaseOpensEveryDesktopBundleBeforePublishing(t *testing.T) {
	document, _ := readReleaseWorkflow(t)

	for job, smoke := range map[string]string{
		"macos":   "scripts/macos/package-smoke.sh",
		"linux":   "scripts/linux/package-smoke.sh",
		"windows": "scripts/windows/package-smoke.ps1",
	} {
		found := document.Jobs[job]
		opened := false
		for _, step := range found.Steps {
			if strings.Contains(step.Run, smoke) {
				opened = true
			}
		}
		if !opened {
			t.Errorf("the %s job never runs %s; it would publish a bundle nobody opened", job, smoke)
		}
	}
}

// **一つでも作れなければ、何も公開しない。**
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

// **Windows の束の一覧が二か所にある。**
//
// あちらは make を使えないので（recipe が POSIX のシェルを前提にしている）、
// workflow が nativebuild を直接呼び、Makefile が環境で渡している値を flag で
// 渡している。二つが離れると、束に入る CLI が想定と違う——ビルドは通り、
// 配ってから壊れる種類の間違いである。
func TestReleaseWindowsBundlesMatchTheMakefile(t *testing.T) {
	_, source := readReleaseWorkflow(t)
	contract := readMakefileContract(t)

	wanted := contract.variables["DESKTOP_WINDOWS_BUNDLES"]
	if len(wanted) == 0 {
		t.Fatal("the Makefile no longer defines DESKTOP_WINDOWS_BUNDLES")
	}
	if !strings.Contains(source, "--bundles \""+strings.Join(wanted, " ")+"\"") {
		t.Errorf("the release workflow does not pass the Makefile's Windows bundles (%s)", strings.Join(wanted, " "))
	}
}

// **署名の無い APK は配布物ではない。**
//
// インストールすらできないので、公開しても誰の役にも立たない。debug の鍵で
// 署名するのはもっと悪い——公開されている鍵なので、誰でも「同じアプリの
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
