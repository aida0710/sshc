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
	Arguments   []string          `json:"arguments"`
	Environment map[string]string `json:"environment"`
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
	}{os.Args[1:], environment}); err != nil { panic(err) }
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
	releaseTargets := "darwin/arm64:1 linux/amd64:0"
	releaseArches := "amd64 arm64"
	releaseDirectory := filepath.Join(temporary, "release path")
	if runtime.GOOS == "windows" {
		// 下の recipe 構造検査で cmd.exe の引用符脱出を検証する。この値にはプラットフォーム
		// 固有の実行ペイロードを埋め込まない。
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
		"RELEASE_TARGETS="+releaseTargets,
		"RELEASE_CURRENT_ARCHES="+releaseArches,
		"RELEASE_DIR="+releaseDirectory,
	)
	command.Dir = repository
	// 継承環境から Make の輸出を上書きさせない、という約束を確かめる。名前の表記は
	// ホストの環境の意味論に合わせる。Windows のプロセス環境は大文字小文字を区別
	// しないので、そこに「別表記」というものは存在せず、表記を変えて渡すことは
	// GNU Make の Windows 移植の変数表を突くだけになる。別表記そのものの扱いは
	// canonicalizeNativeEnvironment の単体テストが両ホストで見ている。
	hostileNames := []string{
		"gOeNv", "gOoS", "GoArCh", "cGo_EnAbLeD",
		"sshc_native_version", "Sshc_Native_Goos", "sshc_native_goarch", "Sshc_Native_Cgo",
		"sshc_native_output", "sshc_native_mac_bundles", "Sshc_Native_Linux_Bundles",
		"sshc_native_windows_bundles", "Sshc_Native_Release_Targets",
		"sshc_native_release_arches", "Sshc_Native_Release_Dir",
	}
	if runtime.GOOS == "windows" {
		for index, name := range hostileNames {
			hostileNames[index] = strings.ToUpper(name)
		}
	}
	hostileValues := []string{
		filepath.Join(temporary, "mixed-hostile-goenv"), "windows", "386", "1",
		"inherited-version-alias", "windows", "386", "1",
		"must-not-override-public-output", "inherited-mac-alias", "inherited-linux-alias",
		"inherited-windows-alias", "inherited-target-alias",
		"inherited-arches-alias", "inherited-dir-alias",
	}
	command.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SSHC_TEST_CAPTURE="+capturePath,
		"SSHC_TEST_MAKE_SENTINEL="+makeExpansionSentinel,
	)
	for index, name := range hostileNames {
		command.Env = append(command.Env, name+"="+hostileValues[index])
	}
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
	wantArguments := []string{"run", "./internal/nativebuild/cmd/nativebuild", "build"}
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
		"SSHC_NATIVE_RELEASE_TARGETS": releaseTargets,
		"SSHC_NATIVE_RELEASE_ARCHES":  releaseArches,
		"SSHC_NATIVE_RELEASE_DIR":     releaseDirectory,
	}
	for key, want := range wantEnvironment {
		if got := capture.Environment[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	// wantEnvironment は fake go が os.LookupEnv で引いた値を見ている。この引き方は
	// ホスト自身の規則（Unix では完全一致、Windows では大文字小文字を無視した照合）
	// なので、go run が実際に受け取る値をどちらのホストでも表している。
}

func TestNativeMakeRecipesContainOnlyFixedInputs(t *testing.T) {
	contract := readMakefileContract(t)
	for _, target := range []string{
		"build", "build-cli", "release-binaries", "release-cli-current",
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

// sanitizedGoTestEnvironment は、make のレシピを走らせる前に、この検査を起動した
// 側の行き先指定を消す。
//
// GOOS を継いだまま make を呼ぶと、レシピが何を組み立てるかが呼び出し側で変わる。
// GOENV を off にするのは、開発機の go env の設定を持ち込まないためである。
//
// 表記の畳み込みは、internal/nativebuild が本番でやっているものと同じ形だが、
// 写しである。あちらの非公開のツールを、この検査のために公開したくない。
func sanitizedGoTestEnvironment(environment []string) []string {
	result := append([]string(nil), environment...)
	for _, pair := range [][2]string{{"GOOS", ""}, {"GOARCH", ""}, {"CGO_ENABLED", ""}, {"GOENV", "off"}} {
		result = replaceEnvironmentValue(result, pair[0], pair[1])
	}
	return result
}

func replaceEnvironmentValue(environment []string, key, value string) []string {
	prefix := key + "="
	kept := environment[:0]
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			kept = append(kept, entry)
		}
	}
	return append(kept, prefix+value)
}
