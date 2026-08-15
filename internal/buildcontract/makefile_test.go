package buildcontract

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type makefileContract struct {
	variables map[string][]string
	targets   map[string]string
}

func TestMakefileProvidesNativeBuildContracts(t *testing.T) {
	contract := readMakefileContract(t)

	t.Run("generic CLI build has explicit inputs and shared flags", func(t *testing.T) {
		recipe := requireTarget(t, contract, "build-cli")
		for _, required := range []string{
			"GOOS is required",
			"GOARCH is required",
			"OUTPUT is required",
			"CGO_ENABLED is required",
			`GOOS="$(GOOS)"`,
			`GOARCH="$(GOARCH)"`,
			`CGO_ENABLED="$(CGO_ENABLED)"`,
			`-o "$(OUTPUT)"`,
			"$(GO_BUILD_FLAGS)",
		} {
			if !strings.Contains(recipe, required) {
				t.Errorf("build-cli recipe does not contain %q\nrecipe:\n%s", required, recipe)
			}
		}

		buildRecipe := requireTarget(t, contract, "build")
		if !strings.Contains(buildRecipe, "$(GO_BUILD_FLAGS)") {
			t.Errorf("build recipe must reuse GO_BUILD_FLAGS\nrecipe:\n%s", buildRecipe)
		}
	})

	t.Run("desktop bundles enumerate native resources and scripts", func(t *testing.T) {
		expectedBundles := map[string][]string{
			"DESKTOP_MAC_BUNDLES": {
				"mac-arm64:darwin:arm64:1:sshc",
				"mac-x64:darwin:amd64:1:sshc",
			},
			"DESKTOP_LINUX_BUNDLES": {
				"linux-arm64:linux:arm64:0:sshc",
				"linux-x64:linux:amd64:0:sshc",
			},
			"DESKTOP_WINDOWS_BUNDLES": {
				"win-arm64:windows:arm64:0:sshc.exe",
				"win-x64:windows:amd64:0:sshc.exe",
			},
		}
		for variable, want := range expectedBundles {
			got, ok := contract.variables[variable]
			if !ok {
				t.Errorf("Makefile variable %s is missing", variable)
				continue
			}
			if strings.Join(got, " ") != strings.Join(want, " ") {
				t.Errorf("%s = %q, want %q", variable, got, want)
			}
		}

		for target, script := range map[string]string{
			"desktop-bundle-mac":     "dist:mac",
			"desktop-bundle-linux":   "dist:linux",
			"desktop-bundle-windows": "dist:win",
		} {
			recipe := requireTarget(t, contract, target)
			for _, required := range []string{
				"desktop/resources/$$name/$$executable",
				"$(MAKE) --no-print-directory build-cli",
				"npm run " + script + " --prefix desktop",
			} {
				if !strings.Contains(recipe, required) {
					t.Errorf("%s recipe does not contain %q\nrecipe:\n%s", target, required, recipe)
				}
			}
		}

		if _, exists := contract.targets["desktop-dist"]; exists {
			t.Error("desktop-dist must be removed; packages are built only on their native OS")
		}
		if _, exists := contract.targets["desktop-run"]; !exists {
			t.Error("desktop-run must remain available for host development")
		}
	})

	t.Run("current OS release includes both architectures and validates names", func(t *testing.T) {
		arches := contract.variables["RELEASE_CURRENT_ARCHES"]
		if strings.Join(arches, " ") != "amd64 arm64" {
			t.Errorf("RELEASE_CURRENT_ARCHES = %q, want [amd64 arm64]", arches)
		}

		recipe := requireTarget(t, contract, "release-cli-current")
		for _, required := range []string{
			`go env GOOS`,
			`windows) suffix=".exe"`,
			`darwin) cgo=1`,
			`linux) cgo=0`,
			`sshc-$$goos-$$goarch$$suffix`,
			"$(MAKE) --no-print-directory build-cli",
			"verify-artifact-name.sh",
			"verify-artifact-name.ps1",
		} {
			if !strings.Contains(recipe, required) {
				t.Errorf("release-cli-current recipe does not contain %q\nrecipe:\n%s", required, recipe)
			}
		}
	})
}

func TestArtifactNameVerifierShell(t *testing.T) {
	script := filepath.Join("..", "..", "scripts", "verify-artifact-name.sh")
	tests := []struct {
		name       string
		artifact   string
		goos       string
		goarch     string
		wantAccept bool
	}{
		{name: "darwin amd64", artifact: "dist/sshc-darwin-amd64", goos: "darwin", goarch: "amd64", wantAccept: true},
		{name: "linux arm64", artifact: "dist/sshc-linux-arm64", goos: "linux", goarch: "arm64", wantAccept: true},
		{name: "windows amd64", artifact: `dist/sshc-windows-amd64.exe`, goos: "windows", goarch: "amd64", wantAccept: true},
		{name: "wrong OS", artifact: "dist/sshc-linux-amd64", goos: "darwin", goarch: "amd64"},
		{name: "wrong architecture", artifact: "dist/sshc-darwin-arm64", goos: "darwin", goarch: "amd64"},
		{name: "windows suffix missing", artifact: "dist/sshc-windows-arm64", goos: "windows", goarch: "arm64"},
		{name: "unix suffix present", artifact: "dist/sshc-linux-amd64.exe", goos: "linux", goarch: "amd64"},
		{name: "unsupported OS", artifact: "dist/sshc-freebsd-amd64", goos: "freebsd", goarch: "amd64"},
		{name: "unsupported architecture", artifact: "dist/sshc-linux-386", goos: "linux", goarch: "386"},
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
		})
	}
}

func readMakefileContract(t *testing.T) makefileContract {
	t.Helper()

	path := filepath.Join("..", "..", "Makefile")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	contract := makefileContract{
		variables: make(map[string][]string),
		targets:   make(map[string]string),
	}
	var logical []string
	var currentTarget string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "\t") {
			if currentTarget != "" {
				contract.targets[currentTarget] += strings.TrimSpace(line) + "\n"
			}
			continue
		}

		currentTarget = ""
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		logical = append(logical, strings.TrimSuffix(trimmed, "\\"))
		if strings.HasSuffix(trimmed, "\\") {
			continue
		}
		joined := strings.Join(logical, " ")
		logical = nil

		if name, value, ok := strings.Cut(joined, "="); ok && !strings.Contains(name, ":") {
			name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(name), "?"))
			contract.variables[name] = strings.Fields(value)
			continue
		}
		if header, _, ok := strings.Cut(joined, ":"); ok {
			currentTarget = strings.TrimSpace(header)
			contract.targets[currentTarget] = ""
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return contract
}

func requireTarget(t *testing.T, contract makefileContract, name string) string {
	t.Helper()
	recipe, ok := contract.targets[name]
	if !ok {
		t.Errorf("Makefile target %s is missing", name)
	}
	return recipe
}
