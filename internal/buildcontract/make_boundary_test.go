package buildcontract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type makeBoundaryCapture struct {
	Arguments          []string          `json:"arguments"`
	Environment        map[string]string `json:"environment"`
	EnvironmentEntries []string          `json:"environmentEntries"`
}

func TestMakeBuildCLITransfersRawInputsWithoutRecipeShellExpansion(t *testing.T) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		t.Skip("GNU Make is unavailable; the Make-to-recipe boundary is unverified on this host and must run in the native Make CI step")
	}
	versionCommand := exec.Command(makePath, "--version")
	versionOutput, err := versionCommand.CombinedOutput()
	if err != nil || !strings.Contains(string(versionOutput), "GNU Make") {
		t.Fatalf("native build contract requires GNU Make; --version error=%v output=%q", err, versionOutput)
	}

	repository := filepath.Join("..", "..")
	temporary := t.TempDir()
	fakeBin := filepath.Join(temporary, "fake-bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeGoSource := filepath.Join(temporary, "fake-go.go")
	fakeGo := filepath.Join(fakeBin, "go")
	if runtime.GOOS == "windows" {
		fakeGo += ".exe"
	}
	source := `package main
import (
	"encoding/json"
	"os"
)
func main() {
	if len(os.Args) == 2 && os.Args[1] == "make-expansion-sentinel" {
		if err := os.WriteFile(os.Getenv("SSHC_TEST_MAKE_SENTINEL"), []byte("expanded"), 0600); err != nil { panic(err) }
		return
	}
	keys := []string{
		"GOENV", "GOOS", "GOARCH", "CGO_ENABLED",
		"SSHC_NATIVE_VERSION", "SSHC_NATIVE_GOOS", "SSHC_NATIVE_GOARCH", "SSHC_NATIVE_CGO", "SSHC_NATIVE_OUTPUT",
		"SSHC_NATIVE_MAC_BUNDLES", "SSHC_NATIVE_LINUX_BUNDLES", "SSHC_NATIVE_WINDOWS_BUNDLES",
		"SSHC_NATIVE_RELEASE_TARGETS", "SSHC_NATIVE_RELEASE_ARCHES", "SSHC_NATIVE_RELEASE_DIR",
	}
	environment := make(map[string]string, len(keys))
	for _, key := range keys {
		value, present := os.LookupEnv(key)
		if present { environment[key] = value }
	}
	file, err := os.Create(os.Getenv("SSHC_TEST_CAPTURE"))
	if err != nil { panic(err) }
	defer file.Close()
	if err := json.NewEncoder(file).Encode(struct {
		Arguments []string ` + "`json:\"arguments\"`" + `
		Environment map[string]string ` + "`json:\"environment\"`" + `
		EnvironmentEntries []string ` + "`json:\"environmentEntries\"`" + `
	}{os.Args[1:], environment, os.Environ()}); err != nil { panic(err) }
}
`
	if err := os.WriteFile(fakeGoSource, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	buildFake := exec.Command("go", "build", "-o", fakeGo, fakeGoSource)
	buildFake.Env = sanitizedGoTestEnvironment(os.Environ())
	if output, err := buildFake.CombinedOutput(); err != nil {
		t.Fatalf("build fake go: %v\n%s", err, output)
	}

	capturePath := filepath.Join(temporary, "capture.json")
	sentinel := filepath.Join(temporary, "quote-breakout-sentinel")
	backtickSentinel := filepath.Join(temporary, "backtick-sentinel")
	makeExpansionSentinel := filepath.Join(temporary, "make-expansion-sentinel")
	output := filepath.Join(temporary, `edge path $dollar $(shell go make-expansion-sentinel) "quote" ; & `+"`"+`touch "`+backtickSentinel+`"`+"`"+` %NAME% (paren)`)
	version := `v1.2.3+$(shell go make-expansion-sentinel)`
	macBundles := "mac-arm64:darwin:arm64:1:sshc mac-x64:darwin:amd64:1:sshc"
	linuxBundles := "linux-arm64:linux:arm64:0:sshc linux-x64:linux:amd64:0:sshc"
	windowsBundles := "win32-arm64:windows:arm64:0:sshc.exe win32-x64:windows:amd64:0:sshc.exe"
	releaseTargets := "darwin/arm64:1 linux/amd64:0"
	releaseArches := "amd64 arm64"
	releaseDirectory := filepath.Join(temporary, "release path")
	if runtime.GOOS == "windows" {
		// The structural recipe assertion below covers cmd.exe quote breakout.
		// Avoid embedding a platform-specific executable payload in this value.
		output = filepath.Join(temporary, `edge path $dollar "quote" ; & `+"`"+`literal`+"`"+` %NAME% (paren)`)
	} else {
		output += `"; touch "` + sentinel + `"; #`
	}

	command := exec.Command(makePath, "--no-print-directory", "build-cli",
		"GOOS=linux",
		"GOARCH=amd64",
		"CGO_ENABLED=0",
		"OUTPUT="+output,
		"VERSION="+version,
		"GOENV="+filepath.Join(temporary, "hostile-goenv"),
		"DESKTOP_MAC_BUNDLES="+macBundles,
		"DESKTOP_LINUX_BUNDLES="+linuxBundles,
		"DESKTOP_WINDOWS_BUNDLES="+windowsBundles,
		"RELEASE_TARGETS="+releaseTargets,
		"RELEASE_CURRENT_ARCHES="+releaseArches,
		"RELEASE_DIR="+releaseDirectory,
	)
	command.Dir = repository
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SSHC_TEST_CAPTURE="+capturePath,
		"SSHC_TEST_MAKE_SENTINEL="+makeExpansionSentinel,
		"gOeNv="+filepath.Join(temporary, "mixed-hostile-goenv"),
		"gOoS=windows",
		"GoArCh=386",
		"cGo_EnAbLeD=1",
		"sshc_native_version=inherited-version-alias",
		"Sshc_Native_Goos=windows",
		"sshc_native_goarch=386",
		"Sshc_Native_Cgo=1",
		"sshc_native_output=must-not-override-public-output",
		"sshc_native_mac_bundles=inherited-mac-alias",
		"Sshc_Native_Linux_Bundles=inherited-linux-alias",
		"sshc_native_windows_bundles=inherited-windows-alias",
		"Sshc_Native_Release_Targets=inherited-target-alias",
		"sshc_native_release_arches=inherited-arches-alias",
		"Sshc_Native_Release_Dir=inherited-dir-alias",
	)
	combined, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("make build-cli: %v\n%s", err, combined)
	}
	for _, path := range []string{sentinel, backtickSentinel, makeExpansionSentinel} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("Make recipe expanded or executed caller input; sentinel exists: %s", path)
		}
	}

	contents, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("read fake go capture: %v", err)
	}
	var capture makeBoundaryCapture
	if err := json.Unmarshal(contents, &capture); err != nil {
		t.Fatalf("decode fake go capture: %v\n%s", err, contents)
	}
	wantArguments := []string{"run", "./internal/buildcontract/cmd/nativebuild", "build"}
	if strings.Join(capture.Arguments, "\x00") != strings.Join(wantArguments, "\x00") {
		t.Fatalf("go arguments = %q, want fixed %q", capture.Arguments, wantArguments)
	}
	wantEnvironment := map[string]string{
		"GOENV":                       "off",
		"GOOS":                        "",
		"GOARCH":                      "",
		"CGO_ENABLED":                 "",
		"SSHC_NATIVE_GOOS":            "linux",
		"SSHC_NATIVE_GOARCH":          "amd64",
		"SSHC_NATIVE_CGO":             "0",
		"SSHC_NATIVE_OUTPUT":          output,
		"SSHC_NATIVE_VERSION":         version,
		"SSHC_NATIVE_MAC_BUNDLES":     macBundles,
		"SSHC_NATIVE_LINUX_BUNDLES":   linuxBundles,
		"SSHC_NATIVE_WINDOWS_BUNDLES": windowsBundles,
		"SSHC_NATIVE_RELEASE_TARGETS": releaseTargets,
		"SSHC_NATIVE_RELEASE_ARCHES":  releaseArches,
		"SSHC_NATIVE_RELEASE_DIR":     releaseDirectory,
	}
	for key, want := range wantEnvironment {
		if got := capture.Environment[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	effectiveHostEnvironment := capture.EnvironmentEntries
	if runtime.GOOS != "windows" {
		// A Unix process can contain differently-cased names at once, while a
		// Windows process environment cannot. Apply the canonical values GNU
		// Make actually exported to a case-insensitive mock of the hostile
		// inherited environment. Native Windows CI exercises the branch above.
		effectiveHostEnvironment = []string{
			"gOeNv=" + filepath.Join(temporary, "mixed-hostile-goenv"),
			"gOoS=windows",
			"GoArCh=386",
			"cGo_EnAbLeD=1",
		}
		for _, key := range []string{"GOENV", "GOOS", "GOARCH", "CGO_ENABLED"} {
			effectiveHostEnvironment = setEnvironmentValue(effectiveHostEnvironment, key, capture.Environment[key])
		}
		t.Log("Windows-native Make execution unavailable; verified case-insensitive process environment with captured canonical Make exports")
	}
	for key, want := range map[string]string{"GOENV": "off", "GOOS": "", "GOARCH": "", "CGO_ENABLED": ""} {
		if got, present := windowsEnvironmentValue(effectiveHostEnvironment, key); !present || got != want {
			t.Errorf("mock Windows host go run effective %s = %q, present=%v, want %q", key, got, present, want)
		}
	}
}

// Go's Windows process launcher deduplicates environment names
// case-insensitively and keeps the last entry. Native Windows uses the captured
// process environment; other hosts use the canonical captured Make exports
// applied to a hostile case-insensitive mock above.
func windowsEnvironmentValue(environment []string, key string) (string, bool) {
	for index := len(environment) - 1; index >= 0; index-- {
		name, value, ok := strings.Cut(environment[index], "=")
		if ok && strings.EqualFold(name, key) {
			return value, true
		}
	}
	return "", false
}

func TestNativeMakeRecipesContainOnlyFixedInputs(t *testing.T) {
	contract := readMakefileContract(t)
	for _, target := range []string{
		"build", "build-cli", "desktop-bundle-mac", "desktop-bundle-linux",
		"desktop-bundle-windows", "desktop-version", "release-binaries", "release-cli-current",
	} {
		recipe := requireTarget(t, contract, target)
		withoutRecursiveMake := strings.ReplaceAll(recipe, "$(MAKE)", "make")
		if strings.Contains(withoutRecursiveMake, "$(") || strings.Contains(withoutRecursiveMake, "${") {
			t.Errorf("%s interpolates a Make value into its recipe:\n%s", target, recipe)
		}
	}

	releaseRecipe := requireTarget(t, contract, "release-cli-current")
	if strings.Contains(releaseRecipe, "npm") {
		t.Errorf("release-cli-current must let the runtime host guard run before npm:\n%s", releaseRecipe)
	}
}

func TestMalformedVersionStopsPublicMakeBeforeNPMOrReleaseDirectory(t *testing.T) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		t.Skip("GNU Make is unavailable; invalid-version ordering is unverified on this host and must run in the native Make CI step")
	}
	repository := filepath.Join("..", "..")
	temporary := t.TempDir()
	fakeBin := filepath.Join(temporary, "fake-bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeNPMSource := filepath.Join(temporary, "fake-npm.go")
	fakeNPM := filepath.Join(fakeBin, "npm")
	if runtime.GOOS == "windows" {
		fakeNPM += ".exe"
	}
	source := `package main
import "os"
func main() {
	if err := os.WriteFile(os.Getenv("SSHC_TEST_NPM_SENTINEL"), []byte("npm executed"), 0600); err != nil { panic(err) }
	os.Exit(91)
}
`
	if err := os.WriteFile(fakeNPMSource, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	buildFake := exec.Command("go", "build", "-o", fakeNPM, fakeNPMSource)
	buildFake.Env = sanitizedGoTestEnvironment(os.Environ())
	if output, err := buildFake.CombinedOutput(); err != nil {
		t.Fatalf("build fake npm: %v\n%s", err, output)
	}

	for _, target := range []string{"build", "release-binaries", "release-cli-current"} {
		t.Run(target, func(t *testing.T) {
			sentinel := filepath.Join(temporary, target+"-npm-sentinel")
			releaseDirectory := filepath.Join(temporary, target+"-release-output")
			command := exec.Command(makePath, "--no-print-directory", target,
				"VERSION=v1.2.3 -w",
				"RELEASE_DIR="+releaseDirectory,
			)
			command.Dir = repository
			command.Env = append(os.Environ(),
				"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"SSHC_TEST_NPM_SENTINEL="+sentinel,
			)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "invalid build version") {
				t.Fatalf("make %s error=%v, want fixed invalid-version rejection\n%s", target, err, output)
			}
			if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
				t.Fatalf("make %s ran npm before rejecting VERSION", target)
			}
			if _, err := os.Stat(releaseDirectory); !os.IsNotExist(err) {
				t.Fatalf("make %s created release directory before rejecting VERSION", target)
			}
		})
	}
}

func sanitizedGoTestEnvironment(environment []string) []string {
	result := append([]string(nil), environment...)
	for _, key := range []string{"GOOS", "GOARCH", "CGO_ENABLED"} {
		result = setEnvironmentValue(result, key, "")
	}
	return setEnvironmentValue(result, "GOENV", "off")
}
