package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"sshc/internal/releasecheck"
	"sshc/internal/selfupdate"
)

func powerShellInstallationFixture(t *testing.T) installation {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "sshc.exe")
	if err := os.WriteFile(executable, []byte("installed executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	executable = resolved
	writePowerShellReceipt(t, executable, "v1.0.0")
	return installation{manager: managerPowerShell, executable: executable}
}

func writePowerShellReceipt(t *testing.T, executable, version string) {
	t.Helper()
	digest, err := fileSHA256(executable)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := json.Marshal(installReceipt{SchemaVersion: 1, Manager: "install.ps1", Repository: installRepository, Version: version, SHA256: digest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(executable), receiptFileName), receipt, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPowerShellUpdatePinsTheScriptAndReleaseAndDelegatesEngineRestart(t *testing.T) {
	for _, caller := range []string{"update", "web-update"} {
		t.Run(caller, func(t *testing.T) {
			found := powerShellInstallationFixture(t)
			script := "& { Write-Output 'fixture' }\n"
			digest := sha256.Sum256([]byte(script))
			downloaded := []string{}
			client := &http.Client{Transport: testHTTPTransport(func(request *http.Request) (*http.Response, error) {
				downloaded = append(downloaded, request.URL.String())
				contents := script
				if strings.HasSuffix(request.URL.Path, "/checksums.txt") {
					contents = hex.EncodeToString(digest[:]) + "  install.ps1\n"
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(contents)), Header: make(http.Header)}, nil
			})}
			var temporaryPath string
			commands := fakeInstallationCommands{
				run: func(_ context.Context, process installationProcess) error {
					if process.name != "powershell.exe" || len(process.args) != 5 || !reflect.DeepEqual(process.args[:4], []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File"}) {
						t.Fatalf("process=%+v", process)
					}
					temporaryPath = process.args[4]
					contents, err := os.ReadFile(temporaryPath)
					if err != nil || string(contents) != script || filepath.Ext(temporaryPath) != ".ps1" {
						t.Fatalf("script=%q error=%v", contents, err)
					}
					for _, entry := range []string{"SSHC_VERSION=v1.1.0", "SSHC_INSTALL_DIR=" + filepath.Dir(found.executable), "SSHC_INSTALL_CALLER=" + caller, "SSHC_INSTALL_TESTING=", "SSHC_TEST_RELEASE_DIR="} {
						if !strings.Contains(strings.Join(process.environment, "\n"), entry+"\n") && process.environment[len(process.environment)-1] != entry {
							t.Errorf("missing environment entry %s", entry)
						}
					}
					if err := os.WriteFile(found.executable, []byte("new executable"), 0o755); err != nil {
						return err
					}
					writePowerShellReceipt(t, found.executable, "v1.1.0")
					_, _ = io.WriteString(process.stdout, "sshc: installing sshc.exe (v1.1.0)\n")
					_, _ = io.WriteString(process.stderr, "sshc: engine restart is left to the caller\n")
					return nil
				},
				output: func(_ context.Context, name string, args ...string) ([]byte, error) {
					if name != found.executable || !reflect.DeepEqual(args, []string{"version"}) {
						t.Fatalf("version command=%s %v", name, args)
					}
					return []byte("sshc v1.1.0 windows/amd64\n"), nil
				},
			}
			var installerOutput bytes.Buffer
			var err error
			if caller == "web-update" {
				// The web update writes install.ps1's output to the job log the page reads while it runs.
				installers := webInstallers{client: client, commands: commands}
				err = installers.runTaggedInstaller(context.Background(), webInstallRequest{found: found, release: releasecheck.Release{Version: "v1.1.0"}, output: &installerOutput})
			} else {
				installer := updateInstaller{client: client, commands: commands, stdout: &installerOutput, stderr: &installerOutput, caller: caller}
				err = installer.install(context.Background(), found, releasecheck.Release{Version: "v1.1.0"})
			}
			if err != nil {
				t.Fatal(err)
			}
			if installerOutput.String() != "sshc: installing sshc.exe (v1.1.0)\nsshc: engine restart is left to the caller\n" {
				t.Fatalf("installer output = %q", installerOutput.String())
			}
			if !reflect.DeepEqual(downloaded, []string{"https://raw.githubusercontent.com/aida0710/sshc/v1.1.0/install.ps1", "https://github.com/aida0710/sshc/releases/download/v1.1.0/checksums.txt"}) {
				t.Fatalf("downloads=%v", downloaded)
			}
			if _, err := os.Stat(temporaryPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("temporary installer remains: %v", err)
			}
		})
	}
}

func TestPowerShellUpdateRejectsMissingOrChangedReceiptsBeforeDownloading(t *testing.T) {
	for _, scenario := range []string{"missing", "changed", "wrong manager"} {
		t.Run(scenario, func(t *testing.T) {
			found := powerShellInstallationFixture(t)
			receipt := filepath.Join(filepath.Dir(found.executable), receiptFileName)
			switch scenario {
			case "missing":
				if err := os.Remove(receipt); err != nil {
					t.Fatal(err)
				}
			case "changed":
				if err := os.WriteFile(found.executable, []byte("changed"), 0o755); err != nil {
					t.Fatal(err)
				}
			case "wrong manager":
				contents, err := os.ReadFile(receipt)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(receipt, bytes.ReplaceAll(contents, []byte("install.ps1"), []byte("install.sh")), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			installer := updateInstaller{}
			if err := installer.install(context.Background(), found, releasecheck.Release{Version: "v1.1.0"}); err == nil {
				t.Fatal("accepted unverified executable")
			}
		})
	}
}

func TestPowerShellUpdateDoesNotRestartTheEngineTwice(t *testing.T) {
	found := powerShellInstallationFixture(t)
	var stdout, stderr bytes.Buffer
	installed := false
	dependencies := updateDependencies{
		executable: func() (string, error) { return found.executable, nil },
		detect:     func(path string) (installation, error) { return detectInstallationForOS(path, "windows") },
		latest: func(context.Context) (releasecheck.Release, error) {
			return releasecheck.Release{Version: "v1.1.0"}, nil
		},
		install: func(context.Context, installation, releasecheck.Release, io.Writer, io.Writer) error {
			installed = true
			return nil
		},
		restartService: func(context.Context, string) (bool, error) {
			t.Fatal("CLI repeated installer restart")
			return false, nil
		},
	}
	if code := runUpdate(context.Background(), updateRun{current: "v1.0.0", yes: true, stdout: &stdout, stderr: &stderr, dependencies: dependencies}); code != 0 || !installed || !strings.Contains(stdout.String(), "install.ps1") || strings.Contains(stdout.String(), "engine --replace") {
		t.Fatalf("code=%d installed=%v stdout=%s stderr=%s", code, installed, stdout.String(), stderr.String())
	}
}

func TestPowerShellWebUpdateRequiresReceiptEvidence(t *testing.T) {
	found := powerShellInstallationFixture(t)
	dependencies := defaultUpdateDependencies()
	dependencies.executable = func() (string, error) { return found.executable, nil }
	dependencies.detect = func(path string) (installation, error) { return detectInstallationForOS(path, "windows") }
	evidence, err := inspectWebInstallation(context.Background(), dependencies)
	if err != nil || evidence.Manager != "install.ps1" || evidence.Executable != found.executable {
		t.Fatalf("evidence=%+v err=%v", evidence, err)
	}
	if err := os.WriteFile(found.executable, []byte("replaced"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err = inspectWebInstallation(context.Background(), dependencies); err != selfupdate.Failure("update_unmanaged") {
		t.Fatalf("changed installation=%v", err)
	}
}

func TestPowerShellChecksumDownloadsStayOnAllowedGitHubHTTPSHosts(t *testing.T) {
	for _, scenario := range []struct {
		url     string
		allowed bool
	}{
		{"https://github.com/aida0710/sshc/releases/download/v1.1.0/checksums.txt", true},
		{"https://release-assets.githubusercontent.com/release-asset/checksums.txt", true},
		{"http://release-assets.githubusercontent.com/checksums.txt", false},
		{"https://github.com.example.org/checksums.txt", false},
		{"https://example.org/checksums.txt", false},
	} {
		t.Run(scenario.url, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, scenario.url, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := allowReleaseAssetRedirect(request, nil); (err == nil) != scenario.allowed {
				t.Fatalf("redirect error=%v allowed=%t", err, scenario.allowed)
			}
			if err := allowReleaseAssetRedirect(request, make([]*http.Request, 10)); err == nil {
				t.Fatal("accepted a redirect loop")
			}
		})
	}
}

func TestPowerShellUpdateReplacesInheritedVariablesUsingPlatformNameRules(t *testing.T) {
	environment := replaceEnvironment([]string{"SSHC_VERSION=old", "sshc_version=lowercase", "KEEP=value"}, map[string]string{"SSHC_VERSION": "v1.1.0"})
	if !slices.Contains(environment, "SSHC_VERSION=v1.1.0") || slices.Contains(environment, "SSHC_VERSION=old") || !slices.Contains(environment, "KEEP=value") {
		t.Fatalf("environment=%v", environment)
	}
	if preserved := slices.Contains(environment, "sshc_version=lowercase"); preserved != (runtime.GOOS != "windows") {
		t.Fatalf("lowercase variable preserved=%t on %s", preserved, runtime.GOOS)
	}
}
