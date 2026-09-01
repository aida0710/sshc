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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sshc/internal/selfupdate"
)

func TestRunUpdateRefusesAnUnmanagedExecutableBeforeNetworkAccess(t *testing.T) {
	latestCalled := false
	var stderr bytes.Buffer
	code := runUpdate(context.Background(), "dev", true, io.Discard, &stderr, updateDependencies{
		executable: func() (string, error) { return "/tmp/sshc", nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerUnknown}, nil
		},
		latest: func(context.Context) (selfupdate.Release, error) {
			latestCalled = true
			return selfupdate.Release{}, nil
		},
	})
	if code != 1 || latestCalled || !strings.Contains(stderr.String(), "cannot be updated automatically") {
		t.Fatalf("code=%d latest=%v stderr=%q", code, latestCalled, stderr.String())
	}
}

func TestRunUpdateSkipsTheInstallerWhenAlreadyCurrent(t *testing.T) {
	installed := false
	var stdout bytes.Buffer
	code := runUpdate(context.Background(), "0.14.0", true, &stdout, io.Discard, updateDependencies{
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
	})
	if code != 0 || installed || !strings.Contains(stdout.String(), "already the latest") {
		t.Fatalf("code=%d installed=%v stdout=%q", code, installed, stdout.String())
	}
}

func TestRunUpdateDelegatesANewerStableRelease(t *testing.T) {
	var got selfupdate.Release
	var stdout bytes.Buffer
	code := runUpdate(context.Background(), "v0.13.6", true, &stdout, io.Discard, updateDependencies{
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
	})
	if code != 0 || got.Version != "v0.14.0" || !strings.Contains(stdout.String(), "restart any running") {
		t.Fatalf("code=%d release=%#v stdout=%q", code, got, stdout.String())
	}
}

func TestRunUpdateRestartsAnActiveManagedService(t *testing.T) {
	restarted := false
	var stdout bytes.Buffer
	code := runUpdate(context.Background(), "v0.13.6", true, &stdout, io.Discard, updateDependencies{
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
	})
	if code != 0 || !restarted || !strings.Contains(stdout.String(), "managed service restarted") ||
		strings.Contains(stdout.String(), "restart any running") {
		t.Fatalf("code=%d restarted=%v stdout=%q", code, restarted, stdout.String())
	}
}

func TestRunUpdateReportsAPartialSuccessWhenManagedServiceRestartFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runUpdate(context.Background(), "v0.13.6", true, &stdout, &stderr, updateDependencies{
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
	})
	if code != 1 || !strings.Contains(stdout.String(), "updated to v0.14.0") ||
		!strings.Contains(stderr.String(), "update succeeded") ||
		!strings.Contains(stderr.String(), "sshc service install") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunUpdateRejectsAnInvalidRemoteTag(t *testing.T) {
	var stderr bytes.Buffer
	code := runUpdate(context.Background(), "v0.13.6", true, io.Discard, &stderr, updateDependencies{
		executable: func() (string, error) { return "/managed/sshc", nil },
		detect: func(string) (installation, error) {
			return installation{manager: managerShell}, nil
		},
		latest: func(context.Context) (selfupdate.Release, error) {
			return selfupdate.Release{Version: "../../main"}, nil
		},
	})
	if code != 1 || !strings.Contains(stderr.String(), "invalid version") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunUpdateConfirmsBeforeInstallingANewerRelease(t *testing.T) {
	installed := false
	confirmed := 0
	var stdout bytes.Buffer
	code := runUpdate(context.Background(), "v0.13.6", false, &stdout, io.Discard, updateDependencies{
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
	})
	if code != 0 || installed || confirmed != 1 || !strings.Contains(stdout.String(), "canceled") {
		t.Fatalf("code=%d installed=%v confirmed=%d stdout=%q", code, installed, confirmed, stdout.String())
	}
}

func TestShellReceiptBindsTheExecutableDigest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.shのreceiptはWindowsでは更新対象にしない")
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, "sshc")
	contents := []byte("a published sshc binary")
	if err := os.WriteFile(executable, contents, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contents)
	receipt := fmt.Sprintf(`{"schemaVersion":1,"manager":"install.sh","repository":"aida0710/sshc","version":"v0.13.6","sha256":"%s"}`,
		hex.EncodeToString(digest[:]))
	if err := os.WriteFile(filepath.Join(directory, receiptFileName), []byte(receipt), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := detectInstallation(executable)
	if err != nil || found.manager != managerShell {
		t.Fatalf("detect = %#v, %v", found, err)
	}
	if err := os.WriteFile(executable, []byte("manually replaced"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := detectInstallation(executable); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("modified receipt error = %v", err)
	}
}

func TestShellReceiptDoesNotFollowASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available on Windows")
	}
	directory := t.TempDir()
	executable := filepath.Join(directory, "sshc")
	if err := os.WriteFile(executable, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	realReceipt := filepath.Join(directory, "elsewhere.json")
	if err := os.WriteFile(realReceipt, []byte(`{"schemaVersion":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realReceipt, filepath.Join(directory, receiptFileName)); err != nil {
		t.Fatal(err)
	}
	if _, err := detectInstallation(executable); err == nil || !strings.Contains(err.Error(), "regular install receipt") {
		t.Fatalf("symlink receipt error = %v", err)
	}
}

func TestHomebrewCandidateRequiresItsOwnExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Homebrew is not supported on Windows")
	}
	prefix := t.TempDir()
	kegBinary := filepath.Join(prefix, "Cellar", "sshc", "0.13.6", "bin", "sshc")
	if err := os.MkdirAll(filepath.Dir(kegBinary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kegBinary, []byte("brew sshc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	brew := filepath.Join(prefix, "bin", "brew")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := detectInstallation(kegBinary)
	if err != nil || found.manager != managerHomebrew {
		t.Fatalf("detect = %#v, %v", found, err)
	}
	foundInfo, err := os.Stat(found.brew)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo, err := os.Stat(brew)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(foundInfo, wantInfo) {
		t.Fatalf("detected brew %q is not the fixture %q", found.brew, brew)
	}
}

type recordedCommand struct {
	name string
	args []string
}

type fakeUpdateCommands struct {
	output func(context.Context, string, ...string) ([]byte, error)
	run    func(context.Context, string, []string, []string, io.Reader, io.Writer, io.Writer) error
}

func (fake fakeUpdateCommands) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return fake.output(ctx, name, args...)
}

func (fake fakeUpdateCommands) Run(ctx context.Context, name string, args []string, environment []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return fake.run(ctx, name, args, environment, stdin, stdout, stderr)
}

func TestHomebrewUpgradeVerifiesOwnershipAndUsesFixedArguments(t *testing.T) {
	directory := t.TempDir()
	prefix := filepath.Join(directory, "opt", "sshc")
	managed := filepath.Join(prefix, "bin", "sshc")
	if err := os.MkdirAll(filepath.Dir(managed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte("same inode"), 0o755); err != nil {
		t.Fatal(err)
	}
	var ran recordedCommand
	commands := fakeUpdateCommands{
		output: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "/brew" {
				return []byte(prefix + "\n"), nil
			}
			if name == managed {
				return []byte("sshc 0.14.0 darwin/arm64\n"), nil
			}
			return nil, fmt.Errorf("unexpected output command %s %#v", name, args)
		},
		run: func(_ context.Context, name string, args []string, _ []string, _ io.Reader, _, _ io.Writer) error {
			ran = recordedCommand{name: name, args: append([]string(nil), args...)}
			return nil
		},
	}
	err := upgradeHomebrew(context.Background(), installation{
		manager: managerHomebrew, executable: managed, brew: "/brew",
	}, "v0.14.0", commands, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if ran.name != "/brew" || strings.Join(ran.args, " ") != "upgrade --formula --no-ask aida0710/tap/sshc" {
		t.Fatalf("brew command = %s %#v", ran.name, ran.args)
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
	commands := fakeUpdateCommands{
		output: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == executable {
				return []byte("sshc v0.14.0 linux/amd64\n"), nil
			}
			return nil, fmt.Errorf("unexpected output command %s", name)
		},
		run: func(_ context.Context, _ string, _ []string, environment []string, _ io.Reader, _, _ io.Writer) error {
			joined := strings.Join(environment, "\n")
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
	if err := runTaggedInstaller(context.Background(), installation{manager: managerShell, executable: executable},
		"v0.14.0", client, commands, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}
