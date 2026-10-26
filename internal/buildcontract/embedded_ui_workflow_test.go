package buildcontract

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 組み立てた画面（internal/ui/dist/app）はリポジトリに入れない。CI とリリースは画面を1回だけ
// 組み立て、画面を埋め込む job と読む job がそれを受け取る。OS ごとに組み立てると、ネイティブの
// 依存（rollup、lightningcss など）や Node の違いで、同じ commit の配布物が別々の画面を持ちうる。

const (
	builtUIArtifact  = "web-ui"
	builtUIDirectory = "internal/ui/dist/app"
)

// builtUIUpload は、job のうち組み立てた画面を成果物として渡す step の位置を返す。無ければ -1。
func builtUIUpload(job workflowJob) int {
	for index, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/upload-artifact@") &&
			fmt.Sprint(step.With["name"]) == builtUIArtifact && fmt.Sprint(step.With["path"]) == builtUIDirectory {
			return index
		}
	}
	return -1
}

// builtUIDownload は、job のうち組み立てた画面を受け取る step の位置を返す。無ければ -1。
func builtUIDownload(job workflowJob) int {
	for index, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/download-artifact@") &&
			fmt.Sprint(step.With["name"]) == builtUIArtifact && fmt.Sprint(step.With["path"]) == builtUIDirectory {
			return index
		}
	}
	return -1
}

// firstStepRunning は、run に words のどれかを含む最初の step の位置を返す。無ければ -1。
func firstStepRunning(job workflowJob, words ...string) int {
	for index, step := range job.Steps {
		for _, word := range words {
			if strings.Contains(step.Run, word) {
				return index
			}
		}
	}
	return -1
}

// validateBuiltUIBuilder は、job が画面を組み立てて渡すことを確かめる。
func validateBuiltUIBuilder(id string, job workflowJob) []string {
	build := firstStepRunning(job, "npm run build")
	upload := builtUIUpload(job)
	switch {
	case build < 0:
		return []string{id + " does not build the web UI"}
	case upload < 0:
		return []string{fmt.Sprintf("%s does not upload %s as %s", id, builtUIDirectory, builtUIArtifact)}
	case upload < build:
		return []string{id + " uploads the web UI before building it"}
	}
	return nil
}

// validateBuiltUIReceiver は、job が画面を組み立てる job を待ち、Go を動かす前に画面を受け取ることを確かめる。
func validateBuiltUIReceiver(id string, job workflowJob, builder string, goSteps ...string) []string {
	var problems []string
	needsBuilder := false
	for _, need := range job.Needs {
		needsBuilder = needsBuilder || need == builder
	}
	if !needsBuilder {
		problems = append(problems, fmt.Sprintf("%s does not need jobs.%s, which builds the web UI", id, builder))
	}
	download := builtUIDownload(job)
	first := firstStepRunning(job, goSteps...)
	switch {
	case download < 0:
		problems = append(problems, fmt.Sprintf("%s does not download %s into %s", id, builtUIArtifact, builtUIDirectory))
	case first < 0:
		problems = append(problems, fmt.Sprintf("%s runs none of %q", id, goSteps))
	case first < download:
		problems = append(problems, fmt.Sprintf("%s runs %q before it has the web UI", id, stepLabel(job.Steps[first])))
	}
	return problems
}

func TestCIBuildsTheWebUIOnceForTheJobsThatEmbedIt(t *testing.T) {
	workflow := readWorkflowDocument(t)
	problems := validateBuiltUIBuilder("jobs.ui", workflow.Jobs["ui"])
	problems = append(problems, validateBuiltUIReceiver("jobs.go", workflow.Jobs["go"], "ui", "go test", "go build")...)
	problems = append(problems, validateBuiltUIReceiver("jobs.android", workflow.Jobs["android"], "ui", "make android-bind")...)
	// 画面と文書だけの PR では、Windows と macOS は画面を受け取る手順も飛ばす。
	const onlyForNativeTests = "${{ env.NATIVE_TESTS == 'true' }}"
	if download := builtUIDownload(workflow.Jobs["go"]); download >= 0 && workflow.Jobs["go"].Steps[download].If != onlyForNativeTests {
		problems = append(problems, fmt.Sprintf("jobs.go downloads the web UI with if %q, want %q", workflow.Jobs["go"].Steps[download].If, onlyForNativeTests))
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

func TestReleaseBuildsTheWebUIOnceForEveryArtifact(t *testing.T) {
	workflow, _ := readReleaseWorkflow(t)
	problems := validateBuiltUIBuilder("jobs.ui", workflow.Jobs["ui"])
	for id, goSteps := range map[string][]string{
		"macos":   {"go test", "make release-cli-current"},
		"linux":   {"make test", "make release-cli-current"},
		"windows": {"go test", "nativebuild release-current"},
		"android": {"make android-bind"},
	} {
		problems = append(problems, validateBuiltUIReceiver("jobs."+id, workflow.Jobs[id], "ui", goSteps...)...)
	}
	// 画面はタグのコミットから組み立てる。手動の実行（workflow_dispatch）では main を取り出すので、
	// ref が無いと main の画面がタグのバイナリに入る。
	const releaseCommit = "${{ needs.verify-source.outputs.release-sha }}"
	for _, step := range workflow.Jobs["ui"].Steps {
		if strings.HasPrefix(step.Uses, "actions/checkout@") && fmt.Sprint(step.With["ref"]) != releaseCommit {
			problems = append(problems, fmt.Sprintf("jobs.ui checks out %q, want the release commit %s", step.With["ref"], releaseCommit))
		}
	}
	// 公開する成果物は release- で始まるものだけを集める。画面のファイルを公開物に混ぜない。
	staged := false
	for _, step := range workflow.Jobs["stage-release"].Steps {
		if strings.HasPrefix(step.Uses, "actions/download-artifact@") {
			staged = true
			if pattern := fmt.Sprint(step.With["pattern"]); pattern != "release-*" {
				problems = append(problems, fmt.Sprintf("jobs.stage-release downloads artifacts with pattern %q, want release-*", pattern))
			}
		}
	}
	if !staged {
		problems = append(problems, "jobs.stage-release downloads no artifacts")
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

// 組み立てた画面は git に入れない。入れると、画面を直す PR どうしが組み立て結果でいつもぶつかる。
// internal/ui/dist には、組み立てる前でも go:embed が通るよう README.md だけを置く。
func TestTheBuiltWebUIStaysOutOfTheRepository(t *testing.T) {
	if !strings.Contains(readContractFile(t, ".gitignore"), "\n/"+builtUIDirectory+"/\n") {
		t.Errorf(".gitignore does not ignore /%s/", builtUIDirectory)
	}
	if !strings.Contains(readContractFile(t, "web", "vite.config.ts"), `outDir: "../`+builtUIDirectory+`"`) {
		t.Errorf("web/vite.config.ts does not build into ../%s", builtUIDirectory)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required to list the tracked files")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repository, ".git")); err != nil {
		t.Skip("the source tree is not a git checkout")
	}
	listed, err := exec.Command("git", "-C", repository, "ls-files", "--", "internal/ui/dist").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	if tracked := strings.Fields(string(listed)); len(tracked) != 1 || tracked[0] != "internal/ui/dist/README.md" {
		t.Errorf("tracked files under internal/ui/dist = %q, want only internal/ui/dist/README.md", tracked)
	}
}
