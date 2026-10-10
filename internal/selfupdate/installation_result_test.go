package selfupdate

import (
	"context"
	"testing"
)

func TestInstalledVersionControlsRestartConfirmationWithoutChangingTheReviewedTarget(t *testing.T) {
	for _, scenario := range []struct {
		name, manager, installedVersion, problem string
	}{
		{name: "Homebrew installs the reviewed version", manager: "homebrew", installedVersion: "v1.1.0"},
		{name: "Homebrew installs a newer version", manager: "homebrew", installedVersion: "v1.2.0"},
		{name: "Homebrew stays on the current version", manager: "homebrew", installedVersion: "v1.0.0", problem: "update_homebrew_not_updated"},
		{name: "Homebrew stays below the reviewed version", manager: "homebrew", installedVersion: "v1.0.1", problem: "update_homebrew_not_updated"},
		{name: "Homebrew reports a development build", manager: "homebrew", installedVersion: "dev", problem: "update_install_failed"},
		{name: "Homebrew reports a prerelease", manager: "homebrew", installedVersion: "v1.2.0-rc.1", problem: "update_install_failed"},
		{name: "shell installer reports a different version", manager: "install.sh", installedVersion: "v1.2.0", problem: "update_install_failed"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dependencies := updateFixture(t)
			dependencies.Inspect = func(context.Context) (Installation, error) {
				return Installation{Manager: scenario.manager, Executable: "/fixture/sshc", Identity: "same-installation"}, nil
			}
			dependencies.Install = func(context.Context, Plan) (string, error) { return scenario.installedVersion, nil }
			restarted := make(chan Job, 1)
			dependencies.Restart = func(_ context.Context, job Job) error { restarted <- job; return nil }
			service := New(dependencies)
			t.Cleanup(func() { _ = service.Stop() })
			plan, err := service.Prepare(context.Background(), "v1.1.0")
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := service.Start(plan)
			if err != nil {
				t.Fatal(err)
			}
			service.ResponseSent(accepted.ID, nil)
			if scenario.problem != "" {
				failed := waitForUpdateState(t, service, JobFailed)
				if failed.Problem != scenario.problem {
					t.Fatalf("failed job = %+v", failed)
				}
				select {
				case <-restarted:
					t.Fatal("failed installation restarted")
				default:
				}
				return
			}
			installed := waitForUpdateState(t, service, JobRestarting)
			if installed.InstalledVersion != scenario.installedVersion || installed.Target != plan.Target {
				t.Fatalf("persisted installation = %+v", installed)
			}
			dependencies.PID++
			if scenario.installedVersion != plan.Target {
				dependencies.Current = plan.Target
				unconfirmed, err := New(dependencies).Status()
				if err != nil || unconfirmed.State != JobRestartRequired {
					t.Fatalf("reviewed version confirmed the wrong installation: %+v, %v", unconfirmed, err)
				}
			}
			dependencies.Current = scenario.installedVersion
			confirmed, err := New(dependencies).Status()
			if err != nil || confirmed.State != JobSucceeded || confirmed.InstalledVersion != scenario.installedVersion {
				t.Fatalf("installed version was not confirmed: %+v, %v", confirmed, err)
			}
		})
	}
}
