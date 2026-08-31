package buildcontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMakefileVerifiesEmbeddedUIAssets(t *testing.T) {
	contract := readMakefileContract(t)
	recipe := requireTarget(t, contract, "verify-ui-dist")
	want := "npm run build --prefix web\nscripts/ci/check-ui-dist.sh\n"
	if recipe != want {
		t.Errorf("verify-ui-dist recipe = %q, want %q", recipe, want)
	}
	if !strings.Contains(contract.source, "verify-generated: generate verify-ui-dist\n") {
		t.Error("verify-generated does not include the embedded UI assets")
	}
}

func TestCIWebJobVerifiesEmbeddedUIAfterBuild(t *testing.T) {
	document := readWorkflowDocument(t)
	job, ok := document.Jobs["web"]
	if !ok {
		t.Fatal("jobs.web is missing")
	}
	build := -1
	verification := -1
	for index, step := range job.Steps {
		switch step.Run {
		case "npm run build":
			build = index
		case "../scripts/ci/check-ui-dist.sh":
			verification = index
		}
	}
	if build < 0 {
		t.Fatal("jobs.web does not build the production UI")
	}
	if verification < 0 {
		t.Fatal("jobs.web does not verify the embedded UI assets")
	}
	if verification != build+1 {
		t.Fatalf("embedded UI verification step = %d, want immediately after build step %d", verification, build)
	}
}

func TestEmbeddedUIVerifierDetectsTrackedAndUntrackedDifferences(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the verifier runs in the Ubuntu generated-files job")
	}
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	script := filepath.Join(repository, "scripts", "ci", "check-ui-dist.sh")
	fixture := t.TempDir()
	runCommand(t, fixture, "git", "init", "--quiet")
	if err := os.MkdirAll(filepath.Join(fixture, "internal", "ui", "dist", "assets"), 0o755); err != nil {
		t.Fatalf("create UI fixture: %v", err)
	}
	writeFixture(t, fixture, filepath.Join("internal", "ui", "dist", "index.html"), "current\n")
	runCommand(t, fixture, "git", "add", "--", filepath.Join("internal", "ui", "dist", "index.html"))
	runCommand(t, fixture, "git", "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "fixture")

	assertUIVerifier(t, fixture, script, true, "")
	writeFixture(t, fixture, filepath.Join("internal", "ui", "dist", "index.html"), "stale\n")
	assertUIVerifier(t, fixture, script, false, " M internal/ui/dist/index.html")

	writeFixture(t, fixture, filepath.Join("internal", "ui", "dist", "index.html"), "current\n")
	writeFixture(t, fixture, filepath.Join("internal", "ui", "dist", "assets", "new-hash.js"), "generated\n")
	assertUIVerifier(t, fixture, script, false, "?? internal/ui/dist/assets/new-hash.js")
}

func assertUIVerifier(t *testing.T, directory, script string, success bool, diagnostic string) {
	t.Helper()
	command := exec.Command("sh", script)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if success {
		if err != nil {
			t.Fatalf("verifier rejected matching assets: %v\n%s", err, output)
		}
		if len(output) != 0 {
			t.Fatalf("verifier emitted output for matching assets: %q", output)
		}
		return
	}
	if err == nil {
		t.Fatalf("verifier accepted differing assets; output: %s", output)
	}
	if !strings.Contains(string(output), diagnostic) {
		t.Fatalf("verifier output %q does not identify %q", output, diagnostic)
	}
	if !strings.Contains(string(output), "make verify-ui-dist") {
		t.Fatalf("verifier output lacks the repair command: %q", output)
	}
}
