package buildcontract

import (
	"strings"
	"testing"
)

// staticcheck は scripts/ci/staticcheck.sh が、Linux・Windows・macOS・Android の組み立てでまとめて流す。
// CI と make test は同じスクリプトを呼ぶ。片方だけに足すと、手元と CI で解析する組み立てと
// staticcheck の版が食い違う。
const staticcheckScript = "scripts/ci/staticcheck.sh"

// 静的解析は、必須の検査「Dependency security」（jobs.security）の手順として流す。staticcheck の版は
// go.mod の tool で固定するので、CI が go install で別に入れると、版が2か所に分かれる。
// if を付けると解析しない変更ができ、continue-on-error を付けると指摘があっても必須の検査が通る。
func TestCIRunsStaticcheckThroughTheSharedScript(t *testing.T) {
	job, present := readWorkflowDocument(t).Jobs["security"]
	if !present {
		t.Fatal("jobs.security is missing")
	}
	if job.If != "" {
		t.Errorf("jobs.security runs only if %q; staticcheck must run for every change", job.If)
	}
	found := false
	for _, step := range job.Steps {
		if strings.TrimSpace(step.Run) == staticcheckScript {
			found = true
			if step.If != "" || step.ContinueOnError != nil {
				t.Errorf("jobs.security step %q must run %s for every change without if or continue-on-error", stepLabel(step), staticcheckScript)
			}
			continue
		}
		if strings.Contains(step.Run, "staticcheck") {
			t.Errorf("jobs.security step %q runs staticcheck without %s:\n%s", stepLabel(step), staticcheckScript, step.Run)
		}
	}
	if !found {
		t.Errorf("jobs.security must run %s", staticcheckScript)
	}
}

func TestMakeTestRunsStaticcheckThroughTheSharedScript(t *testing.T) {
	contract := readMakefileContract(t)
	if recipe := requireTarget(t, contract, "staticcheck"); strings.TrimSpace(recipe) != staticcheckScript {
		t.Errorf("make staticcheck recipe = %q, want %q", recipe, staticcheckScript)
	}
	recipe := requireTarget(t, contract, "test")
	for line := range strings.Lines(recipe) {
		if strings.TrimSpace(line) == "$(MAKE) staticcheck" {
			return
		}
	}
	t.Errorf("make test does not run make staticcheck:\n%s", recipe)
}
