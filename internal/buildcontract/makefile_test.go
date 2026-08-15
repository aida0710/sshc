package buildcontract

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type makefileContract struct {
	variables map[string][]string
	targets   map[string]string
	source    string
}

func TestMakefileProvidesPortableNativeBuildContracts(t *testing.T) {
	contract := readMakefileContract(t)
	helper := "go run ./internal/buildcontract/cmd/nativebuild"
	if !strings.Contains(contract.source, "unexport GOOS GOARCH CGO_ENABLED") {
		t.Error("Makefile must keep target variables out of the host go run environment")
	}

	t.Run("native entries delegate to the argv based helper", func(t *testing.T) {
		wantRecipes := map[string][]string{
			"build": {
				helper + ` host-build --output-dir "bin"`,
			},
			"build-cli": {
				helper + ` build --goos "$(GOOS)" --goarch "$(GOARCH)" --output "$(OUTPUT)" --cgo "$(CGO_ENABLED)"`,
			},
			"desktop-version": {
				helper + ` desktop-version --directory "desktop"`,
			},
			"release-binaries": {
				helper + ` matrix --targets "$(RELEASE_TARGETS)" --output-dir "$(RELEASE_DIR)"`,
			},
			"release-cli-current": {
				helper + ` release-current --arches "$(RELEASE_CURRENT_ARCHES)" --output-dir "$(RELEASE_DIR)"`,
			},
		}
		for target, required := range wantRecipes {
			recipe := requireTarget(t, contract, target)
			for _, fragment := range required {
				if !strings.Contains(recipe, fragment) {
					t.Errorf("%s recipe does not contain %q\nrecipe:\n%s", target, fragment, recipe)
				}
			}
		}
	})

	t.Run("native entries contain no POSIX-only recipe syntax", func(t *testing.T) {
		for _, target := range []string{
			"build",
			"build-cli",
			"desktop-bundle-mac",
			"desktop-bundle-linux",
			"desktop-bundle-windows",
			"desktop-version",
			"release-cli-current",
		} {
			recipe := requireTarget(t, contract, target)
			for _, forbidden := range []string{
				"set -",
				"for ",
				"case ",
				"if [",
				"mkdir -p",
				`GOOS="`,
				`GOARCH="`,
				`CGO_ENABLED="`,
				"$${VERSION}",
			} {
				if strings.Contains(recipe, forbidden) {
					t.Errorf("%s recipe contains POSIX-only fragment %q\nrecipe:\n%s", target, forbidden, recipe)
				}
			}
		}
	})

	t.Run("generic CLI build passes every explicit input", func(t *testing.T) {
		recipe := requireTarget(t, contract, "build-cli")
		for _, required := range []string{
			`--goos "$(GOOS)"`,
			`--goarch "$(GOARCH)"`,
			`--output "$(OUTPUT)"`,
			`--cgo "$(CGO_ENABLED)"`,
		} {
			if !strings.Contains(recipe, required) {
				t.Errorf("build-cli recipe does not contain %q\nrecipe:\n%s", required, recipe)
			}
		}
	})

	t.Run("desktop targets bind the correct bundles host guards and scripts", func(t *testing.T) {
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
				"win32-arm64:windows:arm64:0:sshc.exe",
				"win32-x64:windows:amd64:0:sshc.exe",
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

		targets := []struct {
			name     string
			variable string
			host     string
			script   string
		}{
			{name: "desktop-bundle-mac", variable: "DESKTOP_MAC_BUNDLES", host: "darwin", script: "dist:mac"},
			{name: "desktop-bundle-linux", variable: "DESKTOP_LINUX_BUNDLES", host: "linux", script: "dist:linux"},
			{name: "desktop-bundle-windows", variable: "DESKTOP_WINDOWS_BUNDLES", host: "windows", script: "dist:win"},
		}
		for _, target := range targets {
			recipe := requireTarget(t, contract, target.name)
			for _, required := range []string{
				`guard-host --host ` + target.host,
				`desktop --host ` + target.host,
				`--resource-root "desktop/resources"`,
				`--bundles "$(` + target.variable + `)"`,
				"$(MAKE) --no-print-directory desktop",
				"npm run " + target.script + " --prefix desktop",
			} {
				if !strings.Contains(recipe, required) {
					t.Errorf("%s recipe does not contain %q\nrecipe:\n%s", target.name, required, recipe)
				}
			}
			guardIndex := strings.Index(recipe, "guard-host --host "+target.host)
			installIndex := strings.Index(recipe, "$(MAKE) --no-print-directory desktop")
			webBuildIndex := strings.Index(recipe, "npm run build --prefix web")
			cliBuildIndex := strings.Index(recipe, "desktop --host "+target.host)
			if guardIndex < 0 || installIndex < 0 || webBuildIndex < 0 || cliBuildIndex < 0 ||
				!(guardIndex < installIndex && installIndex < webBuildIndex && webBuildIndex < cliBuildIndex) {
				t.Errorf("%s must guard host before install, then build web before CLI resources\nrecipe:\n%s", target.name, recipe)
			}
			for _, other := range targets {
				if other.name == target.name {
					continue
				}
				for _, forbidden := range []string{
					"--host " + other.host,
					"$(" + other.variable + ")",
					"npm run " + other.script + " --prefix desktop",
				} {
					if strings.Contains(recipe, forbidden) {
						t.Errorf("%s recipe is swapped with %q\nrecipe:\n%s", target.name, forbidden, recipe)
					}
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

	t.Run("current OS release delegates host detection to the helper", func(t *testing.T) {
		arches := contract.variables["RELEASE_CURRENT_ARCHES"]
		if strings.Join(arches, " ") != "amd64 arm64" {
			t.Errorf("RELEASE_CURRENT_ARCHES = %q, want [amd64 arm64]", arches)
		}

		recipe := requireTarget(t, contract, "release-cli-current")
		if strings.Contains(recipe, "GOOS") || strings.Contains(recipe, "go env GOOS") {
			t.Errorf("release-cli-current must not use overrideable target GOOS for host detection\nrecipe:\n%s", recipe)
		}
	})
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
		contract.source += line + "\n"
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
