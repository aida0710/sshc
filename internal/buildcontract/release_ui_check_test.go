package buildcontract

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// releaseUICheckCommand は、リリースの runner で埋め込み UI を作り直して照合するだけの
// nativebuild の命令である。release.yml の release-current と同じ手順を通る。
const releaseUICheckCommand = "go run ./internal/nativebuild/cmd/nativebuild verify-embedded-ui"

// releaseNativeJobs は、release.yml のうち、runner 上で埋め込み UI を作り直す job である。
var releaseNativeJobs = []string{"macos", "linux", "windows"}

func releaseUICheckPath() string {
	return filepath.Join("..", "..", ".github", "workflows", "release-ui-check.yml")
}

// releaseToolchainActions は、照合の job が release.yml の job と同じ commit で使う action
// である。版の違う setup-go や setup-node で照合が通っても、公開の当日に同じ UI を
// 作れるとは言えない。
var releaseToolchainActions = []string{"actions/checkout", "actions/setup-go", "actions/setup-node"}

// releaseUICheckBoundary は、照合の workflow の引き金と権限と、照合の失敗を run の成功に
// 変えうる項目（if、continue-on-error）である。workflowDocument はこれらを読まないうえ、
// if や continue-on-error は式でも書けるので、ここで別に、型を決めずに読む。
type releaseUICheckBoundary struct {
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		Permissions     any `yaml:"permissions"`
		Environment     any `yaml:"environment"`
		If              any `yaml:"if"`
		ContinueOnError any `yaml:"continue-on-error"`
		Steps           []struct {
			If              any `yaml:"if"`
			ContinueOnError any `yaml:"continue-on-error"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func readReleaseUICheck(t *testing.T) (workflowDocument, string) {
	t.Helper()
	source, err := os.ReadFile(releaseUICheckPath())
	if err != nil {
		t.Fatalf("read %s: %v", releaseUICheckPath(), err)
	}
	document, err := decodeWorkflowDocument(source)
	if err != nil {
		t.Fatalf("decode %s: %v", releaseUICheckPath(), err)
	}
	return document, string(source)
}

func readReleaseUICheckBoundary(t *testing.T) releaseUICheckBoundary {
	t.Helper()
	_, source := readReleaseUICheck(t)
	var boundary releaseUICheckBoundary
	if err := yaml.Unmarshal([]byte(source), &boundary); err != nil {
		t.Fatalf("decode the check's triggers, permissions and conditions: %v", err)
	}
	return boundary
}

// releaseUICheckJob は、照合の workflow のただ 1 つの job を返す。
func releaseUICheckJob(t *testing.T, document workflowDocument) workflowJob {
	t.Helper()
	job, present := document.Jobs["embedded-ui"]
	if !present || len(document.Jobs) != 1 {
		t.Fatalf("release-ui-check.yml must have only the embedded-ui job; it has %d jobs", len(document.Jobs))
	}
	return job
}

// 照合は、公開の当日に止まる runner と同じ runner で試さなければ意味が無い。
// release.yml の runner を変えたら、この workflow も同じ変更で揃える。
func TestReleaseUICheckRunsOnTheReleaseRunners(t *testing.T) {
	release, _ := readReleaseWorkflow(t)
	check, _ := readReleaseUICheck(t)
	job := releaseUICheckJob(t, check)

	want := make([]string, 0, len(releaseNativeJobs))
	for _, id := range releaseNativeJobs {
		releaseJob, present := release.Jobs[id]
		if !present {
			t.Fatalf("release.yml has no %s job", id)
		}
		want = append(want, releaseJob.RunsOn)
	}
	sort.Strings(want)

	if job.RunsOn != "${{ matrix.os }}" || job.Strategy == nil {
		t.Fatal("the check does not run a matrix of runners")
	}
	// 1 つの OS で落ちても、ほかの OS の結果を見られるようにする。
	if job.Strategy.FailFast == nil || *job.Strategy.FailFast {
		t.Error("the check must set fail-fast: false")
	}
	got := make([]string, 0, len(job.Strategy.Matrix.Include))
	for _, entry := range job.Strategy.Matrix.Include {
		got = append(got, entry.OS)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the check runs on %q, want the release runners %q", got, want)
	}
}

// 照合だけを、リリースと同じ nativebuild の手順で走らせる。バイナリは作らない。
func TestReleaseUICheckRunsOnlyTheReleaseComparison(t *testing.T) {
	check, _ := readReleaseUICheck(t)
	job := releaseUICheckJob(t, check)

	runs := make([]string, 0, len(job.Steps))
	for _, step := range job.Steps {
		if step.Run != "" {
			runs = append(runs, strings.TrimSpace(step.Run))
		}
	}
	if want := []string{"npm ci --prefix web", releaseUICheckCommand}; !reflect.DeepEqual(runs, want) {
		t.Errorf("the check runs %q, want exactly %q", runs, want)
	}
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/upload-artifact@") {
			t.Error("the check uploads an artifact; it must not produce anything to publish")
		}
	}

	problems := validateSetupOrder("the check", job, "actions/setup-node")
	problems = append(problems, validatePinnedSetup(job, "actions/setup-go")...)
	for _, problem := range problems {
		t.Error(problem)
	}
}

// UI を作る道具は、release.yml の job と同じ commit の action と同じ Node で用意する。
// release.yml の action や Node を変えたら、この workflow も同じ変更で揃える。
func TestReleaseUICheckSetsUpTheReleaseToolchain(t *testing.T) {
	release, _ := readReleaseWorkflow(t)
	check, _ := readReleaseUICheck(t)
	job := releaseUICheckJob(t, check)

	for _, action := range releaseToolchainActions {
		checkUses := actionReferences(job, action)
		if len(checkUses) != 1 {
			t.Errorf("the check has %d %s steps, want 1", len(checkUses), action)
			continue
		}
		for _, id := range releaseNativeJobs {
			if releaseUses := actionReferences(release.Jobs[id], action); !reflect.DeepEqual(releaseUses, checkUses) {
				t.Errorf("the check uses %q, but the release %s job uses %q", checkUses, id, releaseUses)
			}
		}
	}

	checkNode := setupNodeVersion(t, job)
	for _, id := range releaseNativeJobs {
		if releaseNode := setupNodeVersion(t, release.Jobs[id]); releaseNode != checkNode {
			t.Errorf("the check uses Node %q, but the release %s job uses %q", checkNode, id, releaseNode)
		}
	}
}

// 照合が違いを見つけたら、run も失敗しなければならない。job や step に if や
// continue-on-error を置くと、照合を飛ばしたり失敗を無視したりしたまま run が成功になり、
// 公開の前に試したことにならない。
func TestReleaseUICheckFailsWhenTheComparisonFails(t *testing.T) {
	boundary := readReleaseUICheckBoundary(t)
	for id, job := range boundary.Jobs {
		if job.If != nil {
			t.Errorf("jobs.%s has an if condition", id)
		}
		if job.ContinueOnError != nil {
			t.Errorf("jobs.%s sets continue-on-error", id)
		}
		for index, step := range job.Steps {
			if step.If != nil {
				t.Errorf("jobs.%s step %d has an if condition", id, index)
			}
			if step.ContinueOnError != nil {
				t.Errorf("jobs.%s step %d sets continue-on-error", id, index)
			}
		}
	}
}

// 手で走らせるだけの入口であり、公開・署名・tag の権限も秘密も持たせない。
func TestReleaseUICheckIsManualAndReadOnly(t *testing.T) {
	_, source := readReleaseUICheck(t)
	boundary := readReleaseUICheckBoundary(t)

	triggers := make([]string, 0, len(boundary.On))
	for trigger := range boundary.On {
		triggers = append(triggers, trigger)
	}
	if !reflect.DeepEqual(triggers, []string{"workflow_dispatch"}) {
		t.Errorf("the check triggers on %q, want only workflow_dispatch", triggers)
	}
	if want := map[string]string{"contents": "read"}; !reflect.DeepEqual(boundary.Permissions, want) {
		t.Errorf("the check's permissions = %v, want %v", boundary.Permissions, want)
	}
	for id, job := range boundary.Jobs {
		if job.Permissions != nil {
			t.Errorf("jobs.%s widens the workflow permissions", id)
		}
		if job.Environment != nil {
			t.Errorf("jobs.%s uses a deployment environment", id)
		}
	}
	if strings.Contains(withoutYAMLComments(source), "secrets.") {
		t.Error("the check reads a secret")
	}
}

// 公開の前に走らせる手順は、docs/releasing.md に workflow の名前で書く。
func TestReleasingDocumentRunsTheUICheckBeforePublishing(t *testing.T) {
	documentation := readContractFile(t, "docs", "releasing.md")
	if !strings.Contains(documentation, "gh workflow run release-ui-check.yml") {
		t.Error("docs/releasing.md does not tell how to run the release UI check")
	}
}

// action は tag ではなく commit で固定する。tag は付け替えられるので、同じ workflow が
// 別の中身を実行しうる。GitHub は .yml と .yaml のどちらも workflow として読む。
func TestEveryWorkflowPinsActionsToACommit(t *testing.T) {
	var paths []string
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", pattern))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, matches...)
	}
	if !slices.Contains(paths, releaseUICheckPath()) {
		t.Fatalf("workflows %q do not include %s", paths, releaseUICheckPath())
	}
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, problem := range validateActionPins(stepsOnlyWorkflowDocument(t, path, source)) {
			t.Errorf("%s: %s", filepath.Base(path), problem)
		}
	}
}

// stepsOnlyWorkflowDocument は、workflow の job の step だけを読んだ workflowDocument を返す。
// ほかの項目は workflow ごとに書き方が違う（pages.yml の needs は 1 つの名前で書く）ので読まない。
func stepsOnlyWorkflowDocument(t *testing.T, path string, source []byte) workflowDocument {
	t.Helper()
	var decoded struct {
		Jobs map[string]struct {
			Steps []workflowStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(source, &decoded); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	document := workflowDocument{Jobs: make(map[string]workflowJob, len(decoded.Jobs))}
	for id, job := range decoded.Jobs {
		document.Jobs[id] = workflowJob{Steps: job.Steps}
	}
	return document
}

// actionReferences は、job が action を呼ぶ step の uses（action@commit）を、step の順に返す。
func actionReferences(job workflowJob, action string) []string {
	var references []string
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, action+"@") {
			references = append(references, step.Uses)
		}
	}
	return references
}

func setupNodeVersion(t *testing.T, job workflowJob) string {
	t.Helper()
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/setup-node@") {
			return fmt.Sprint(step.With["node-version"])
		}
	}
	t.Fatalf("job %q does not set up Node", job.Name)
	return ""
}
