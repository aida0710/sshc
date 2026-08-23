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

// **PowerShell は native command の非ゼロ終了で止まらない。**
//
// GitHub は step の最後の一つの終了値しか見ないので、前の行が落ちても step は
// 緑になる。実際にそれで、CLI の入っていないインストーラが「ビルド成功」の
// まま次の段へ渡った——捕まえたのは smoke であって、ビルドではない。
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
