package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"sshc/internal/selfupdate"
)

func TestRunUpdateRefusesAnUnmanagedExecutableBeforeNetworkAccess(t *testing.T) {
	latestCalled := false
	var stderr bytes.Buffer
	code := runUpdate(context.Background(), updateRun{current: "dev", yes: true, stdout: io.Discard, stderr: &stderr, dependencies: updateDependencies{
		executable: func() (string, error) { return "/tmp/sshc", nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerUnknown}, nil
		},
		latest: func(context.Context) (selfupdate.Release, error) {
			latestCalled = true
			return selfupdate.Release{}, nil
		},
	}})
	if code != 1 || latestCalled || !strings.Contains(stderr.String(), "cannot be updated automatically") {
		t.Fatalf("code=%d latest=%v stderr=%q", code, latestCalled, stderr.String())
	}
}

func TestRunUpdateSkipsTheInstallerWhenAlreadyCurrent(t *testing.T) {
	installed := false
	var stdout bytes.Buffer
	code := runUpdate(context.Background(), updateRun{current: "0.14.0", yes: true, stdout: &stdout, stderr: io.Discard, dependencies: updateDependencies{
		executable: func() (string, error) { return "/managed/sshc", nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerHomebrew}, nil
		},
		latest: func(context.Context) (selfupdate.Release, error) {
			return selfupdate.Release{Version: "v0.14.0"}, nil
		},
		install: func(context.Context, installation, selfupdate.Release, io.Writer, io.Writer) error {
			installed = true
			return nil
		},
	}})
	if code != 0 || installed || !strings.Contains(stdout.String(), "already the latest") {
		t.Fatalf("code=%d installed=%v stdout=%q", code, installed, stdout.String())
	}
}

func TestRunUpdateDelegatesANewerStableRelease(t *testing.T) {
	var got selfupdate.Release
	var stdout bytes.Buffer
	code := runUpdate(context.Background(), updateRun{current: "v0.13.6", yes: true, stdout: &stdout, stderr: io.Discard, dependencies: updateDependencies{
		executable: func() (string, error) { return "/managed/sshc", nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerShell}, nil
		},
		latest: func(context.Context) (selfupdate.Release, error) {
			return selfupdate.Release{Version: "0.14.0"}, nil
		},
		install: func(_ context.Context, _ installation, release selfupdate.Release, _, _ io.Writer) error {
			got = release
			return nil
		},
	}})
	if code != 0 || got.Version != "v0.14.0" || !strings.Contains(stdout.String(), "restart any running") {
		t.Fatalf("code=%d release=%#v stdout=%q", code, got, stdout.String())
	}
}

func TestRunUpdateRestartsAnActiveManagedService(t *testing.T) {
	restarted := false
	var stdout bytes.Buffer
	code := runUpdate(context.Background(), updateRun{current: "v0.13.6", yes: true, stdout: &stdout, stderr: io.Discard, dependencies: updateDependencies{
		executable: func() (string, error) { return "/managed/sshc", nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerShell}, nil
		},
		latest: func(context.Context) (selfupdate.Release, error) {
			return selfupdate.Release{Version: "v0.14.0"}, nil
		},
		install: func(context.Context, installation, selfupdate.Release, io.Writer, io.Writer) error {
			return nil
		},
		serviceExecutable: func(context.Context, installation) (string, error) {
			return "/managed/sshc", nil
		},
		restartService: func(_ context.Context, executable string) (bool, error) {
			if executable != "/managed/sshc" {
				t.Fatalf("service executable = %q", executable)
			}
			restarted = true
			return true, nil
		},
		engineStatus: (&fixedEngineStatus{answer: lockedVault}).read,
	}})
	if code != 0 || !restarted || !strings.Contains(stdout.String(), "managed service restarted") ||
		!strings.Contains(stdout.String(), "sshc: "+vaultLockedAdvice+"\n") ||
		strings.Contains(stdout.String(), "restart any running") {
		t.Fatalf("code=%d restarted=%v stdout=%q", code, restarted, stdout.String())
	}
}

func TestRunUpdateReportsAPartialSuccessWhenManagedServiceRestartFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runUpdate(context.Background(), updateRun{current: "v0.13.6", yes: true, stdout: &stdout, stderr: &stderr, dependencies: updateDependencies{
		executable: func() (string, error) { return "/managed/sshc", nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerShell}, nil
		},
		latest: func(context.Context) (selfupdate.Release, error) {
			return selfupdate.Release{Version: "v0.14.0"}, nil
		},
		install: func(context.Context, installation, selfupdate.Release, io.Writer, io.Writer) error {
			return nil
		},
		serviceExecutable: func(context.Context, installation) (string, error) {
			return "/managed/sshc", nil
		},
		restartService: func(context.Context, string) (bool, error) {
			return false, errors.New("systemctl failed")
		},
	}})
	if code != 1 || !strings.Contains(stdout.String(), "updated to v0.14.0") ||
		!strings.Contains(stderr.String(), "update succeeded") ||
		!strings.Contains(stderr.String(), "sshc service install") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunUpdateTellsToReinstallAServiceDefinitionFromAnOlderVersion(t *testing.T) {
	var stdout bytes.Buffer
	code := runUpdate(context.Background(), updateRun{current: "v0.13.6", yes: true, stdout: &stdout, stderr: io.Discard, dependencies: updateDependencies{
		executable: func() (string, error) { return "/managed/sshc", nil },
		detect:     func(string) (installation, error) { return installation{manager: managerShell}, nil },
		latest: func(context.Context) (selfupdate.Release, error) {
			return selfupdate.Release{Version: "v0.14.0"}, nil
		},
		install: func(context.Context, installation, selfupdate.Release, io.Writer, io.Writer) error { return nil },
		serviceExecutable: func(context.Context, installation) (string, error) {
			return "/managed/sshc", nil
		},
		restartService: func(context.Context, string) (bool, error) { return false, errOutdatedServiceDefinition },
	}})
	if code != 0 || !strings.HasSuffix(stdout.String(), "sshc: "+outdatedServiceDefinitionAdvice+"\n") ||
		strings.Contains(stdout.String(), "restart any running") {
		t.Fatalf("code=%d stdout=%q", code, stdout.String())
	}
}

func TestRunUpdateTellsHomebrewUsersToRefreshTheTapWhenTheOldVersionStays(t *testing.T) {
	var stderr bytes.Buffer
	code := runUpdate(context.Background(), updateRun{current: "v0.13.6", yes: true, stdout: io.Discard, stderr: &stderr, dependencies: updateDependencies{
		executable: func() (string, error) { return "/managed/sshc", nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerHomebrew}, nil
		},
		latest: func(context.Context) (selfupdate.Release, error) {
			return selfupdate.Release{Version: "v0.14.0"}, nil
		},
		install: func(context.Context, installation, selfupdate.Release, io.Writer, io.Writer) error {
			return fmt.Errorf("verify the upgraded Homebrew executable: %w", errHomebrewTapNotRefreshed)
		},
	}})
	if code != 1 || !strings.Contains(stderr.String(), "run `brew update`, then `sshc update` again") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunUpdateRejectsAnInvalidRemoteTag(t *testing.T) {
	var stderr bytes.Buffer
	code := runUpdate(context.Background(), updateRun{current: "v0.13.6", yes: true, stdout: io.Discard, stderr: &stderr, dependencies: updateDependencies{
		executable: func() (string, error) { return "/managed/sshc", nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerShell}, nil
		},
		latest: func(context.Context) (selfupdate.Release, error) {
			return selfupdate.Release{Version: "../../main"}, nil
		},
	}})
	if code != 1 || !strings.Contains(stderr.String(), "invalid version") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunUpdateConfirmsBeforeInstallingANewerRelease(t *testing.T) {
	installed := false
	confirmed := 0
	var stdout bytes.Buffer
	code := runUpdate(context.Background(), updateRun{current: "v0.13.6", yes: false, stdout: &stdout, stderr: io.Discard, dependencies: updateDependencies{
		executable: func() (string, error) { return "/managed/sshc", nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerShell, executable: "/managed/sshc"}, nil
		},
		latest: func(context.Context) (selfupdate.Release, error) {
			return selfupdate.Release{Version: "v0.14.0"}, nil
		},
		install: func(context.Context, installation, selfupdate.Release, io.Writer, io.Writer) error {
			installed = true
			return nil
		},
		confirm: func(context.Context, string) (bool, error) {
			confirmed++
			return false, nil
		},
	}})
	if code != 0 || installed || confirmed != 1 || !strings.Contains(stdout.String(), "canceled") {
		t.Fatalf("code=%d installed=%v confirmed=%d stdout=%q", code, installed, confirmed, stdout.String())
	}
}

type recordedCommand struct {
	name string
	args []string
}

type fakeInstallationCommands struct {
	output func(context.Context, string, ...string) ([]byte, error)
	run    func(context.Context, installationProcess) error
}

func (fake fakeInstallationCommands) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return fake.output(ctx, name, args...)
}

func (fake fakeInstallationCommands) Run(ctx context.Context, process installationProcess) error {
	return fake.run(ctx, process)
}

// homebrewFormulaFixture は、brew --prefix が返す formula の prefix と、その下の
// 実行ファイルを一時ディレクトリに作る。
func homebrewFormulaFixture(t *testing.T) (prefix, managed string) {
	t.Helper()
	prefix = filepath.Join(t.TempDir(), "opt", "sshc")
	managed = filepath.Join(prefix, "bin", "sshc")
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte("same inode"), 0o755); err != nil {
		t.Fatal(err)
	}
	return prefix, managed
}

func TestHomebrewUpgradeVerifiesOwnershipAndUsesFixedArguments(t *testing.T) {
	prefix, managed := homebrewFormulaFixture(t)
	var ran recordedCommand
	commands := fakeInstallationCommands{
		output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "/brew" {
				return []byte(prefix + "\n"), nil
			}
			if name == managed {
				return []byte("sshc 0.14.0 darwin/arm64\n"), nil
			}
			return nil, fmt.Errorf("unexpected output command %s %#v", name, args)
		},
		run: func(_ context.Context, process installationProcess) error {
			ran = recordedCommand{name: process.name, args: append([]string(nil), process.args...)}
			return nil
		},
	}
	installer := updateInstaller{commands: commands, stdout: io.Discard, stderr: io.Discard}
	err := installer.upgradeHomebrew(context.Background(), installation{
		manager: managerHomebrew, executable: managed, brew: "/brew",
	}, "v0.14.0")
	if err != nil {
		t.Fatal(err)
	}
	if ran.name != "/brew" || strings.Join(ran.args, " ") != "upgrade --formula --no-ask aida0710/tap/sshc" {
		t.Fatalf("brew command = %s %#v", ran.name, ran.args)
	}
}

func TestHomebrewUpgradeBlamesTheTapOnlyWhenTheOldVersionStays(t *testing.T) {
	for _, test := range []struct {
		name         string
		version      func() ([]byte, error)
		blamesTheTap bool
	}{
		{
			name:         "brew upgrade kept the old version",
			version:      func() ([]byte, error) { return []byte("sshc v0.13.6 darwin/arm64\n"), nil },
			blamesTheTap: true,
		},
		{
			name:         "the upgraded executable does not run",
			version:      func() ([]byte, error) { return nil, errors.New("exec format error") },
			blamesTheTap: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			prefix, managed := homebrewFormulaFixture(t)
			commands := fakeInstallationCommands{
				output: func(_ context.Context, name string, args ...string) ([]byte, error) {
					switch name {
					case "/brew":
						return []byte(prefix + "\n"), nil
					case managed:
						return test.version()
					}
					return nil, fmt.Errorf("unexpected output command %s %#v", name, args)
				},
				run: func(context.Context, installationProcess) error { return nil },
			}
			installer := updateInstaller{commands: commands, stdout: io.Discard, stderr: io.Discard}
			err := installer.upgradeHomebrew(context.Background(), installation{
				manager: managerHomebrew, executable: managed, brew: "/brew",
			}, "v0.14.0")
			if err == nil {
				t.Fatal("upgradeHomebrew succeeded without the new version")
			}
			if blamed := errors.Is(err, errHomebrewTapNotRefreshed); blamed != test.blamesTheTap {
				t.Fatalf("upgradeHomebrew() = %v; blames the tap = %t, want %t", err, blamed, test.blamesTheTap)
			}
		})
	}
}

type installerTransport func(*http.Request) (*http.Response, error)

func (transport installerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestTaggedInstallerUsesTheExactReleaseAndInstallDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is not supported on Windows")
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, "sshc")
	if err := os.WriteFile(executable, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: installerTransport(func(request *http.Request) (*http.Response, error) {
		if got := request.URL.String(); got != "https://raw.githubusercontent.com/aida0710/sshc/v0.14.0/install.sh" {
			t.Fatalf("installer URL = %s", got)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("#!/bin/sh\n")), Header: make(http.Header)}, nil
	})}
	commands := fakeInstallationCommands{
		output: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == executable {
				return []byte("sshc v0.14.0 linux/amd64\n"), nil
			}
			return nil, fmt.Errorf("unexpected output command %s", name)
		},
		run: func(_ context.Context, process installationProcess) error {
			joined := strings.Join(process.environment, "\n")
			if !strings.Contains(joined, "SSHC_VERSION=v0.14.0") || !strings.Contains(joined, "SSHC_INSTALL_DIR="+directory) {
				t.Fatalf("installer environment lacks fixed version/directory")
			}
			contents := []byte("new")
			if err := os.WriteFile(executable, contents, 0o755); err != nil {
				return err
			}
			digest := sha256.Sum256(contents)
			receipt := fmt.Sprintf(`{"schemaVersion":1,"manager":"install.sh","repository":"aida0710/sshc","version":"v0.14.0","sha256":"%s"}`,
				hex.EncodeToString(digest[:]))
			return os.WriteFile(filepath.Join(directory, receiptFileName), []byte(receipt), 0o644)
		},
	}
	installer := updateInstaller{client: client, commands: commands, stdout: io.Discard, stderr: io.Discard}
	if err := installer.runTaggedInstaller(context.Background(),
		installation{manager: managerShell, executable: executable}, "v0.14.0"); err != nil {
		t.Fatal(err)
	}
}

func TestTaggedInstallerRedirectStaysOnHTTPSRawGitHub(t *testing.T) {
	for _, test := range []struct {
		name    string
		target  string
		allowed bool
	}{
		{name: "https to the same host", target: "https://raw.githubusercontent.com/aida0710/sshc/v0.14.0/install.sh", allowed: true},
		{name: "another host", target: "https://example.com/install.sh", allowed: false},
		{name: "downgrade to http", target: "http://raw.githubusercontent.com/aida0710/sshc/v0.14.0/install.sh", allowed: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, test.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = allowTaggedInstallerRedirect(request, nil)
			if allowed := err == nil; allowed != test.allowed {
				t.Fatalf("allowTaggedInstallerRedirect(%s) = %v, want allowed=%t", test.target, err, test.allowed)
			}
		})
	}
}

func TestTaggedInstallerClientDoesNotFollowARedirectOffRawGitHub(t *testing.T) {
	// redirect 先に届いたことは handler の goroutine が書くので、atomic で受け渡す。
	var reached atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		reached.Store(true)
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, destination.URL+"/install.sh", http.StatusFound)
	}))
	defer origin.Close()

	response, err := taggedInstallerHTTPClient().Get(origin.URL + "/install.sh")
	if err == nil {
		response.Body.Close()
		t.Fatal("the installer client followed a redirect off https://raw.githubusercontent.com")
	}
	if reached.Load() {
		t.Fatal("the redirect destination received a request")
	}
}

func TestUpdateDoesNotAskToUnlockAPasswordlessVaultAfterRestartingTheService(t *testing.T) {
	status := &fixedEngineStatus{answer: passwordlessVault}
	var stdout bytes.Buffer
	code := runUpdate(context.Background(), updateRun{current: "v0.13.6", yes: true, home: "/home/test", stdout: &stdout, stderr: io.Discard, dependencies: updateDependencies{
		executable: func() (string, error) { return "/managed/sshc", nil },
		detect:     func(string) (installation, error) { return installation{manager: managerShell}, nil },
		latest: func(context.Context) (selfupdate.Release, error) {
			return selfupdate.Release{Version: "v0.14.0"}, nil
		},
		install: func(context.Context, installation, selfupdate.Release, io.Writer, io.Writer) error { return nil },
		serviceExecutable: func(context.Context, installation) (string, error) {
			return "/managed/sshc", nil
		},
		restartService: func(context.Context, string) (bool, error) { return true, nil },
		engineStatus:   status.read,
	}})
	if code != 0 || !strings.HasSuffix(stdout.String(), "sshc: managed service restarted\n") {
		t.Fatalf("code=%d stdout=%q", code, stdout.String())
	}
	if len(status.asked) != 1 || status.asked[0] != "/home/test" {
		t.Fatalf("asked homes = %q, want the update's home once", status.asked)
	}
}
