package buildcontract

import (
	"fmt"
	"strings"
	"testing"
)

// 足したコメントが日本語かは、scripts/ci/check-comment-language.mjs が変更の差分で検査する。
// PR の CI と make test が同じスクリプトを使う。
const commentLanguageScript = "scripts/ci/check-comment-language.mjs"

// Go だけの PR でもコメントを足すので、検査は if の無い jobs.web-checks に置く。差分は PR の
// merge commit とその1つ目の親（base）の間で取るので、取り出すコミットは2つ要る。1つだと
// HEAD^1 が無いので、比べる先が無いとして検査が失敗する。
func TestCIChecksTheLanguageOfAddedCommentsInEveryChange(t *testing.T) {
	job, present := readWorkflowDocument(t).Jobs["web-checks"]
	if !present {
		t.Fatal("jobs.web-checks is missing")
	}
	if job.If != "" {
		t.Errorf("jobs.web-checks runs only if %q; the comment language check must run for every change", job.If)
	}

	const checkCommand = commentLanguageScript + " HEAD^1 HEAD"
	found := false
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/checkout@") && fmt.Sprint(step.With["fetch-depth"]) != "2" {
			t.Errorf("jobs.web-checks checkout fetch-depth = %v, want 2 to reach the merge commit's base", step.With["fetch-depth"])
		}
		if !strings.Contains(step.Run, checkCommand) {
			continue
		}
		found = true
		if step.If != "" {
			t.Errorf("jobs.web-checks step %q runs only if %q", step.Name, step.If)
		}
	}
	if !found {
		t.Errorf("jobs.web-checks must run %q", checkCommand)
	}
}

func TestMakeTestChecksTheLanguageOfAddedComments(t *testing.T) {
	recipe := requireTarget(t, readMakefileContract(t), "test")
	for line := range strings.Lines(recipe) {
		if !strings.HasPrefix(line, "@#") && strings.Contains(line, commentLanguageScript) {
			return
		}
	}
	t.Errorf("make test does not run %s:\n%s", commentLanguageScript, recipe)
}
