package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestServiceRestartRefusesAnIneligibleServiceBeforeConfirmation(t *testing.T) {
	for _, test := range []struct {
		name    string
		manager fakeServiceManager
		want    string
	}{
		{name: "absent", manager: fakeServiceManager{state: serviceAbsent}, want: "service is not installed"},
		{name: "inactive", manager: fakeServiceManager{state: serviceInactive}, want: "installed but inactive"},
		{name: "unmanaged", manager: fakeServiceManager{state: serviceUnmanaged}, want: "not managed by sshc"},
		{name: "outdated", manager: fakeServiceManager{state: serviceActive, restartPlanErr: errOutdatedServiceDefinition}, want: outdatedServiceDefinitionAdvice},
		{name: "another installation", manager: fakeServiceManager{state: serviceActive, restartPlanErr: errServiceExecutableMismatch}, want: "different sshc executable"},
		{name: "status failure", manager: fakeServiceManager{err: errors.New("user manager is unavailable")}, want: "user manager is unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			confirmed := false
			var stdout, stderr bytes.Buffer
			code := runService(context.Background(), serviceRun{
				action: "restart", home: t.TempDir(), stdout: &stdout, stderr: &stderr,
				dependencies: serviceDependencies{
					manager:    func(string) (engineServiceManager, error) { return &test.manager, nil },
					executable: func(context.Context) (string, error) { return "/opt/sshc/bin/sshc", nil },
					confirm: func(context.Context, string) (bool, error) {
						confirmed = true
						return true, nil
					},
				},
			})
			if code != exitFailure || confirmed || test.manager.restartExecutable != "" || stdout.Len() != 0 || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("code=%d confirmed=%v restart=%q stdout=%q stderr=%q", code, confirmed, test.manager.restartExecutable, stdout.String(), stderr.String())
			}
		})
	}
}

func TestServiceRestartRejectsAnUnverifiedInstallation(t *testing.T) {
	manager := &fakeServiceManager{state: serviceActive}
	var stdout, stderr bytes.Buffer
	code := runService(context.Background(), serviceRun{
		action: "restart", yes: true, home: t.TempDir(), stdout: &stdout, stderr: &stderr,
		dependencies: serviceDependencies{
			manager: func(string) (engineServiceManager, error) { return manager, nil },
			executable: func(context.Context) (string, error) {
				return "", errors.New("installation receipt does not match")
			},
		},
	})
	if code != exitFailure || manager.restartExecutable != "" || stdout.Len() != 0 || !strings.Contains(stderr.String(), "choose a stable service executable") {
		t.Fatalf("code=%d restart=%q stdout=%q stderr=%q", code, manager.restartExecutable, stdout.String(), stderr.String())
	}
}

func TestServiceRestartShowsThePlanAndHonorsConfirmation(t *testing.T) {
	for _, test := range []struct {
		name      string
		yes       bool
		approve   bool
		restarted bool
	}{
		{name: "declined"},
		{name: "approved", approve: true, restarted: true},
		{name: "yes skips the prompt", yes: true, restarted: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := &fakeServiceManager{state: serviceActive}
			var stdout, stderr bytes.Buffer
			confirmations := 0
			code := runService(context.Background(), serviceRun{
				action: "restart", yes: test.yes, home: t.TempDir(), stdout: &stdout, stderr: &stderr,
				dependencies: serviceDependencies{
					manager:    func(string) (engineServiceManager, error) { return manager, nil },
					executable: func(context.Context) (string, error) { return "/opt/sshc/bin/sshc", nil },
					confirm: func(context.Context, string) (bool, error) {
						confirmations++
						if !strings.Contains(stdout.String(), "restart service using /opt/sshc/bin/sshc") || manager.restartExecutable != "" {
							t.Fatal("the plan was not shown before confirming the restart")
						}
						return test.approve, nil
					},
					engineStatus: (&fixedEngineStatus{answer: passwordlessVault}).read,
				},
			})
			if code != 0 || stderr.Len() != 0 || (manager.restartExecutable != "") != test.restarted || strings.Contains(stdout.String(), "managed service restarted") != test.restarted {
				t.Fatalf("code=%d restart=%q stdout=%q stderr=%q", code, manager.restartExecutable, stdout.String(), stderr.String())
			}
			if test.restarted && manager.restartExecutable != "/opt/sshc/bin/sshc" {
				t.Fatalf("restart executable=%q", manager.restartExecutable)
			}
			if (confirmations == 0) != test.yes {
				t.Fatalf("confirmations=%d yes=%v", confirmations, test.yes)
			}
		})
	}
}

func TestServiceRestartFailsWhenTheServiceChangesOrReadinessFails(t *testing.T) {
	for _, test := range []struct {
		name    string
		err     error
		skipped bool
		code    int
		want    string
	}{
		{name: "changed after confirmation", skipped: true, code: exitFailure, want: "sshc service status"},
		{name: "operation lock held", err: errors.New("another sshc service operation is in progress"), code: exitFailure, want: "operation is in progress"},
		{name: "readiness failed", err: errors.New("restarted service did not become ready"), code: exitFailure, want: "did not become ready"},
		{name: "interrupted", err: context.Canceled, code: exitInterrupted},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := &fakeServiceManager{state: serviceActive, restartErr: test.err}
			status := &fixedEngineStatus{answer: lockedVault}
			var stdout, stderr bytes.Buffer
			code := runService(context.Background(), serviceRun{
				action: "restart", home: t.TempDir(), stdout: &stdout, stderr: &stderr,
				dependencies: serviceDependencies{
					manager:    func(string) (engineServiceManager, error) { return manager, nil },
					executable: func(context.Context) (string, error) { return "/opt/sshc/bin/sshc", nil },
					confirm: func(context.Context, string) (bool, error) {
						manager.restartSkipped = test.skipped
						return true, nil
					},
					engineStatus: status.read,
				},
			})
			if code != test.code || strings.Contains(stdout.String(), "managed service restarted") || len(status.asked) != 0 || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("code=%d stdout=%q stderr=%q status homes=%q", code, stdout.String(), stderr.String(), status.asked)
			}
		})
	}
}

func TestServiceRestartTellsHowToUnlockTheRestartedVault(t *testing.T) {
	status := &fixedEngineStatus{answer: lockedVault}
	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runService(context.Background(), serviceRun{
		action: "restart", yes: true, home: home, stdout: &stdout, stderr: &stderr,
		dependencies: serviceDependencies{
			manager:      func(string) (engineServiceManager, error) { return &fakeServiceManager{state: serviceActive}, nil },
			executable:   func(context.Context) (string, error) { return "/opt/sshc/bin/sshc", nil },
			engineStatus: status.read,
		},
	})
	if code != 0 || stderr.Len() != 0 || !strings.HasSuffix(stdout.String(), "sshc: "+vaultLockedAdvice+"\n") || len(status.asked) != 1 || status.asked[0] != home {
		t.Fatalf("code=%d stdout=%q stderr=%q status homes=%q", code, stdout.String(), stderr.String(), status.asked)
	}
}

func TestServiceRestartAcceptsOnlyConfirmationFlags(t *testing.T) {
	for _, flag := range []string{"-y", "--yes"} {
		called, err := parseInvocation([]string{"sshc", "service", "restart", flag})
		if err != nil || called.Kind != invocationService || !called.Yes || len(called.Args) != 1 || called.Args[0] != "restart" {
			t.Fatalf("restart %s = %#v, %v", flag, called, err)
		}
	}
	for _, args := range [][]string{{"extra"}, {"--json"}, {"--force"}, {"-y", "--yes"}} {
		if _, err := parseInvocation(append([]string{"sshc", "service", "restart"}, args...)); err == nil {
			t.Fatalf("restart accepted %q", args)
		}
	}
}
