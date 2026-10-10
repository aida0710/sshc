//go:build !windows && !android

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/releasecheck"
	"sshc/internal/selfupdate"
)

func TestWebHomebrewUpdateFollowsTheFormulaAndRestartsFromTheStablePath(t *testing.T) {
	prefix := t.TempDir()
	for _, tag := range []string{"1.0.0", "1.2.0"} {
		executable := filepath.Join(prefix, "Cellar", "sshc", tag, "bin", "sshc")
		if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf("#!/bin/sh\nprintf 'sshc v%s %s\\n'\n", tag, "darwin/arm64")
		if err := os.WriteFile(executable, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, directory := range []string{"bin", "opt"} {
		if err := os.MkdirAll(filepath.Join(prefix, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	managed := filepath.Join(prefix, "opt", "sshc")
	if err := os.Symlink(filepath.Join("..", "Cellar", "sshc", "1.0.0"), managed); err != nil {
		t.Fatal(err)
	}
	brew := `#!/bin/sh
set -eu
fixture_prefix=$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)
if [ "$#" -eq 3 ] && [ "$1" = --prefix ] && [ "$2" = --installed ] && [ "$3" = aida0710/tap/sshc ]; then
  printf '%s/opt/sshc\n' "$fixture_prefix"
elif [ "$#" -eq 4 ] && [ "$1" = upgrade ] && [ "$2" = --formula ] && [ "$3" = --no-ask ] && [ "$4" = aida0710/tap/sshc ]; then
  rm "$fixture_prefix/opt/sshc"
  ln -s ../Cellar/sshc/1.2.0 "$fixture_prefix/opt/sshc"
else
  exit 1
fi
`
	if err := os.WriteFile(filepath.Join(prefix, "bin", "brew"), []byte(brew), 0o755); err != nil {
		t.Fatal(err)
	}
	installationDependencies := defaultUpdateDependencies()
	installationDependencies.executable = func() (string, error) { return filepath.Join(prefix, "Cellar", "sshc", "1.0.0", "bin", "sshc"), nil }
	installationDependencies.latest = func(context.Context) (releasecheck.Release, error) {
		return releasecheck.Release{Version: "v1.1.0"}, nil
	}
	dependencies := webUpdateDependencies(selfupdate.Dependencies{Current: "v1.0.0", PID: 101, StatePath: filepath.Join(t.TempDir(), selfupdate.StateFileName)}, installationDependencies)
	restarted := make(chan selfupdate.Job, 1)
	dependencies.Restart = func(_ context.Context, job selfupdate.Job) error { restarted <- job; return nil }
	service := selfupdate.New(dependencies)
	t.Cleanup(func() { _ = service.Stop() })
	plan, err := service.Prepare(context.Background(), "v1.1.0")
	if err != nil || plan.Installation.Manager != "homebrew" {
		t.Fatalf("Homebrew preview = %+v, %v", plan, err)
	}
	job, err := service.Start(plan)
	if err != nil {
		t.Fatal(err)
	}
	service.ResponseSent(job.ID, nil)
	select {
	case installed := <-restarted:
		if installed.Target != "v1.1.0" || installed.InstalledVersion != "v1.2.0" {
			t.Fatalf("Homebrew result = %+v", installed)
		}
		resolvedPrefix, err := filepath.EvalSymlinks(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if installed.Plan.Installation.Executable != filepath.Join(resolvedPrefix, "opt", "sshc", "bin", "sshc") {
			t.Fatalf("restart path = %s", installed.Plan.Installation.Executable)
		}
		installationDependencies.executable = func() (string, error) { return filepath.Join(prefix, "Cellar", "sshc", "1.2.0", "bin", "sshc"), nil }
		if err := verifyWebUpdateRestartInstallation(context.Background(), installed, installationDependencies); err != nil {
			t.Fatal(err)
		}
		dependencies.Current, dependencies.PID = "v1.2.0", 202
		confirmed, err := selfupdate.New(dependencies).Status()
		if err != nil || confirmed.State != selfupdate.JobSucceeded {
			t.Fatalf("restart confirmation = %+v, %v", confirmed, err)
		}
	case <-time.After(webUpdateCompletionTimeout):
		job, _ := service.Status()
		t.Fatalf("Homebrew did not reach restart: %+v", job)
	}
}
