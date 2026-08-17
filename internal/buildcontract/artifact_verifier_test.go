package buildcontract

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactNameVerifierShell(t *testing.T) {
	// **無いものを「壊れている」と報告しない。** この検査が確かめるのは POSIX
	// シェルのスクリプトであり、それを走らせる sh が無い機械では、確かめられる
	// ものが何も無い。同じパッケージの Make 境界検査が make について同じことを
	// している——Windows の実機には Git 同梱の sh が PATH に載っていないことが
	// あり、そこで落とすと、通らない理由がスクリプトにあるように見える。
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no POSIX shell on this host; the artifact name verifier must run in a step that has one")
	}
	script := filepath.Join("..", "..", "scripts", "verify-artifact-name.sh")
	tests := []struct {
		name        string
		artifact    string
		goos        string
		goarch      string
		wantAccept  bool
		wantMessage string
	}{
		{name: "darwin amd64", artifact: "dist/sshc-darwin-amd64", goos: "darwin", goarch: "amd64", wantAccept: true},
		{name: "linux arm64", artifact: "dist/sshc-linux-arm64", goos: "linux", goarch: "arm64", wantAccept: true},
		{name: "windows amd64", artifact: `dist/sshc-windows-amd64.exe`, goos: "windows", goarch: "amd64", wantAccept: true},
		{name: "wrong OS", artifact: "private/sshc-linux-amd64", goos: "darwin", goarch: "amd64", wantMessage: "artifact name rejected\n"},
		{name: "wrong architecture", artifact: "private/sshc-darwin-arm64", goos: "darwin", goarch: "amd64", wantMessage: "artifact name rejected\n"},
		{name: "windows suffix missing", artifact: "private/sshc-windows-arm64", goos: "windows", goarch: "arm64", wantMessage: "artifact name rejected\n"},
		{name: "unix suffix present", artifact: "private/sshc-linux-amd64.exe", goos: "linux", goarch: "amd64", wantMessage: "artifact name rejected\n"},
		{name: "unsupported OS", artifact: "private/sshc-secret-os-amd64", goos: "secret-os", goarch: "amd64", wantMessage: "artifact OS rejected\n"},
		{name: "unsupported architecture", artifact: "private/sshc-linux-secret-arch", goos: "linux", goarch: "secret-arch", wantMessage: "artifact architecture rejected\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("sh", script, test.artifact, test.goos, test.goarch)
			output, err := command.CombinedOutput()
			if test.wantAccept && err != nil {
				t.Fatalf("valid artifact name was rejected: %v\n%s", err, output)
			}
			if !test.wantAccept && err == nil {
				t.Fatalf("invalid artifact name was accepted: %s", test.artifact)
			}
			if !test.wantAccept && string(output) != test.wantMessage {
				t.Errorf("rejection output = %q, want fixed message %q", output, test.wantMessage)
			}
		})
	}
}

func TestArtifactNameVerifierPowerShell(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh is not installed; PowerShell verifier behavior is unverified on this host")
	}

	script := filepath.Join("..", "..", "scripts", "verify-artifact-name.ps1")
	tests := []struct {
		name        string
		artifact    string
		goos        string
		goarch      string
		wantAccept  bool
		wantMessage string
	}{
		{name: "literal wildcard path", artifact: `private/[literal]/sshc-windows-amd64.exe`, goos: "windows", goarch: "amd64", wantAccept: true},
		{name: "wrong suffix", artifact: `private/[literal]/sshc-windows-amd64`, goos: "windows", goarch: "amd64", wantMessage: "artifact name rejected"},
		{name: "empty path", artifact: "", goos: "windows", goarch: "amd64", wantMessage: "artifact path rejected"},
		{name: "path ending in a separator", artifact: `private/[literal]/`, goos: "windows", goarch: "amd64", wantMessage: "artifact path rejected"},
		{name: "backslash separated path", artifact: `private\[literal]\sshc-windows-amd64.exe`, goos: "windows", goarch: "amd64", wantAccept: true},
		{name: "unsupported OS", artifact: `private/[literal]/sshc-secret-os-amd64`, goos: "secret-os", goarch: "amd64", wantMessage: "artifact OS rejected"},
		{name: "unsupported architecture", artifact: `private/[literal]/sshc-linux-secret-arch`, goos: "linux", goarch: "secret-arch", wantMessage: "artifact architecture rejected"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(pwsh, "-NoProfile", "-File", script, "-Artifact", test.artifact, "-OS", test.goos, "-Architecture", test.goarch)
			output, err := command.CombinedOutput()
			if test.wantAccept && err != nil {
				t.Fatalf("valid artifact name was rejected: %v\n%s", err, output)
			}
			if !test.wantAccept && err == nil {
				t.Fatalf("invalid artifact name was accepted: %s", test.artifact)
			}
			if !test.wantAccept && !strings.Contains(string(output), test.wantMessage) {
				t.Errorf("rejection output = %q, want fixed message containing %q", output, test.wantMessage)
			}
			for _, secret := range []string{test.artifact, test.goos, test.goarch} {
				if !test.wantAccept && secret != "" && strings.Contains(string(output), secret) {
					t.Errorf("rejection output leaked raw invalid input %q: %q", secret, output)
				}
			}
		})
	}
}
