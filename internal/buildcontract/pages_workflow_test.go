package buildcontract

import (
	"path/filepath"
	"strings"
	"testing"
)

// Pagesのbuild jobはnpmの依存を実行する。依存に悪性の版が入っても、Pagesへ書き込む権限と、
// このworkflow名義のOIDC tokenを取れないよう、書き込み権限はdeploy jobだけに渡す。
// checkoutのtokenも.git/configに残さない。
func TestPagesWorkflowGrantsWritePermissionsOnlyToTheDeployJob(t *testing.T) {
	workflow, _ := readWorkflowFile(t, filepath.Join(workflowsDirectory(), "pages.yml"))

	for _, problem := range validateWorkflowPermissions(workflow, workflowPermissionContract{
		workflow: map[string]string{"contents": "read"},
		writers: map[string]map[string]string{
			"deploy": {"pages": "write", "id-token": "write"},
		},
	}) {
		t.Error(problem)
	}

	for id, job := range workflow.Jobs {
		if id == "deploy" {
			continue
		}
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/checkout@") && step.With["persist-credentials"] != false {
				t.Errorf("jobs.%s leaves the checkout token in .git/config for the steps after it", id)
			}
		}
	}
}
