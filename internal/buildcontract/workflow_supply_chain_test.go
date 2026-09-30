package buildcontract

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// workflowFiles は、.github/workflows にある workflow のファイルをすべて返す。
// ファイル名を並べて持たないのは、workflow を足したときに検査から漏らさないためである。
func workflowFiles(t *testing.T) []string {
	t.Helper()
	var paths []string
	for _, pattern := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(workflowsDirectory(), pattern))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, matches...)
	}
	// 正のコントロール: 1つも無いなら、探す場所を間違えている。
	if len(paths) == 0 {
		t.Fatalf("no workflow files under %s; this test is looking in the wrong place", workflowsDirectory())
	}
	sort.Strings(paths)
	return paths
}

// tag は動かせるので、tag で参照した action は、tag を動かした人のコードになる。
// release.yml には署名鍵と書き込み権限が、pages.yml には Pages への書き込み権限がある。
// ci.yml だけでなく、どの workflow の action も commit で固定する。
func TestEveryWorkflowPinsThirdPartyActionsToCommits(t *testing.T) {
	for _, path := range workflowFiles(t) {
		document, _ := readWorkflowFile(t, path)
		for _, problem := range validateActionPins(document) {
			t.Errorf("%s: %s", filepath.Base(path), problem)
		}
	}
}

// pull_request_target は、fork からの pull request でも、base の secret と書き込み権限で動く。
func TestNoWorkflowRunsOnPullRequestTarget(t *testing.T) {
	for _, path := range workflowFiles(t) {
		document, _ := readWorkflowFile(t, path)
		// 正のコントロール: on を読めていなければ、次の検査は何も見ていない。
		if len(document.On) == 0 {
			t.Errorf("%s: no trigger was read from on", filepath.Base(path))
		}
		if slices.Contains(document.On, "pull_request_target") {
			t.Errorf("%s runs on pull_request_target", filepath.Base(path))
		}
	}
}

func validateActionPins(document workflowDocument) []string {
	pinned := regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)
	isThirdParty := func(uses string) bool {
		return uses != "" && !strings.HasPrefix(uses, "./")
	}
	var problems []string
	for _, id := range slices.Sorted(maps.Keys(document.Jobs)) {
		job := document.Jobs[id]
		if isThirdParty(job.Uses) && !pinned.MatchString(job.Uses) {
			problems = append(problems, fmt.Sprintf("jobs.%s reusable workflow is not pinned to a 40-hex commit: %q", id, job.Uses))
		}
		for stepIndex, step := range job.Steps {
			if isThirdParty(step.Uses) && !pinned.MatchString(step.Uses) {
				problems = append(problems, fmt.Sprintf("jobs.%s step %d action is not pinned to a 40-hex commit: %q", id, stepIndex, step.Uses))
			}
		}
	}
	return problems
}

// workflowPermissionContract は、workflow の権限をどこに置いてよいかである。
type workflowPermissionContract struct {
	// workflow は、workflow 全体の権限である。すべての job が引き継ぐ。
	workflow map[string]string
	// writers は、書き込み権限を持ってよい job と、その job の権限である。
	// ここに無い job は、書き込み権限を持てない。
	writers map[string]map[string]string
}

func validateWorkflowPermissions(document workflowDocument, contract workflowPermissionContract) []string {
	var problems []string
	if !maps.Equal(document.Permissions, contract.workflow) {
		problems = append(problems, fmt.Sprintf("workflow permissions = %v, want %v; every job inherits them", document.Permissions, contract.workflow))
	}
	for _, id := range slices.Sorted(maps.Keys(contract.writers)) {
		job, present := document.Jobs[id]
		if !present {
			problems = append(problems, fmt.Sprintf("jobs.%s is missing", id))
			continue
		}
		if want := contract.writers[id]; !maps.Equal(job.Permissions, want) {
			problems = append(problems, fmt.Sprintf("jobs.%s permissions = %v, want %v", id, job.Permissions, want))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(document.Jobs)) {
		if _, writer := contract.writers[id]; writer {
			continue
		}
		for _, scope := range slices.Sorted(maps.Keys(document.Jobs[id].Permissions)) {
			if access := document.Jobs[id].Permissions[scope]; access == "write" {
				problems = append(problems, fmt.Sprintf("jobs.%s holds %s: %s, but only %v may write", id, scope, access, slices.Sorted(maps.Keys(contract.writers))))
			}
		}
	}
	return problems
}
