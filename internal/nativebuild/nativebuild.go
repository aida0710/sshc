package nativebuild

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode"
)

const (
	nativeVersionEnvironment        = "SSHC_NATIVE_VERSION"
	nativeGOOSEnvironment           = "SSHC_NATIVE_GOOS"
	nativeGOARCHEnvironment         = "SSHC_NATIVE_GOARCH"
	nativeCGOEnvironment            = "SSHC_NATIVE_CGO"
	nativeOutputEnvironment         = "SSHC_NATIVE_OUTPUT"
	nativeReleaseTargetsEnvironment = "SSHC_NATIVE_RELEASE_TARGETS"
	nativeReleaseArchesEnvironment  = "SSHC_NATIVE_RELEASE_ARCHES"
	nativeReleaseDirEnvironment     = "SSHC_NATIVE_RELEASE_DIR"
)

var semverBuildVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

var nativeEnvironmentKeys = []string{
	nativeVersionEnvironment,
	nativeGOOSEnvironment,
	nativeGOARCHEnvironment,
	nativeCGOEnvironment,
	nativeOutputEnvironment,
	nativeReleaseTargetsEnvironment,
	nativeReleaseArchesEnvironment,
	nativeReleaseDirEnvironment,
}

type nativeCommand struct {
	name        string
	args        []string
	environment []string
	directory   string
}

type nativeCommandExecutor interface {
	Run(nativeCommand) error
	Output(nativeCommand) ([]byte, error)
}

type osNativeExecutor struct {
	stdout io.Writer
	stderr io.Writer
}

func (executor osNativeExecutor) Run(command nativeCommand) error {
	if !allowedNativeProgram(command.name) {
		return errors.New("native build program is not allowed")
	}
	process := exec.Command(command.name, command.args...)
	process.Env = command.environment
	process.Dir = command.directory
	process.Stdout = executor.stdout
	process.Stderr = executor.stderr
	return process.Run()
}

func (executor osNativeExecutor) Output(command nativeCommand) ([]byte, error) {
	if !allowedNativeProgram(command.name) {
		return nil, errors.New("native build program is not allowed")
	}
	process := exec.Command(command.name, command.args...)
	process.Env = command.environment
	process.Dir = command.directory
	return process.Output()
}

func allowedNativeProgram(name string) bool {
	switch name {
	case "go", "git", "npm", "sh", "pwsh":
		return true
	default:
		return false
	}
}

type nativeBuildDependencies struct {
	hostOS      string
	hostArch    string
	hostCGO     string
	environment []string
	executor    nativeCommandExecutor
	mkdirAll    func(string, os.FileMode) error
	// verifyBinary はビルド済みバイナリが指定ターゲットと一致することを検証する。
	//
	// 検査がバイナリを読み取るため、テストでは verifyBinary を差し替えられる。
	// executor で組み立てた検査は、ファイルを一つも作らない。ここを直に
	// 呼ぶと、そのすべてが「読めなかった」で落ちる。
	verifyBinary func(path, goos, goarch string) error
}

type nativeBuildRequest struct {
	goos   string
	goarch string
	output string
	cgo    string
}

// RunNativeBuild は Makefile wrapper が使用する移植可能なエントリーポイントである。コマンドは argv、
// 対象値は明示的な継承環境で渡し、パスとバージョンを shell 展開しない。
func RunNativeBuild(args []string, stdout, stderr io.Writer) error {
	environment := os.Environ()
	return runNativeBuild(args, nativeBuildDependencies{
		hostOS:       runtime.GOOS,
		hostArch:     runtime.GOARCH,
		hostCGO:      os.Getenv("CGO_ENABLED"),
		environment:  environment,
		executor:     osNativeExecutor{stdout: stdout, stderr: stderr},
		mkdirAll:     os.MkdirAll,
		verifyBinary: VerifyBinaryArchitecture,
	}, stdout, stderr)
}

func runNativeBuild(args []string, dependencies nativeBuildDependencies, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("command is required")
	}
	if dependencies.executor == nil || dependencies.mkdirAll == nil {
		return errors.New("native build dependencies are incomplete")
	}
	environment, err := canonicalizeNativeEnvironment(dependencies.environment)
	if err != nil {
		return err
	}
	dependencies.environment = environment

	switch args[0] {
	case "build":
		return runExplicitBuild(args[1:], dependencies, stdout, stderr)
	case "host-build":
		return runHostBuild(args[1:], dependencies, stdout, stderr)
	case "guard-host":
		return runHostGuard(args[1:], dependencies, stderr)
	case "matrix":
		return runBuildMatrix(args[1:], dependencies, stdout, stderr)
	case "release-current":
		return runCurrentRelease(args[1:], dependencies, stdout, stderr)
	case "verify-embedded-ui":
		return runEmbeddedUIVerification(args[1:], dependencies, stderr)
	default:
		return errors.New("unsupported native build command")
	}
}

func runHostGuard(args []string, dependencies nativeBuildDependencies, stderr io.Writer) error {
	flags := newNativeFlagSet("guard-host", stderr)
	expectedHost := flags.String("host", "", "required host operating system")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	if !supportedOS(*expectedHost) {
		return errors.New("unsupported required host")
	}
	if dependencies.hostOS != *expectedHost {
		return fmt.Errorf("target requires %s host; actual host is %s", *expectedHost, dependencies.hostOS)
	}
	return nil
}

func runExplicitBuild(args []string, dependencies nativeBuildDependencies, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("build", stderr)
	goos := flags.String("goos", environmentValue(dependencies.environment, nativeGOOSEnvironment), "target operating system")
	goarch := flags.String("goarch", environmentValue(dependencies.environment, nativeGOARCHEnvironment), "target architecture")
	output := flags.String("output", environmentValue(dependencies.environment, nativeOutputEnvironment), "output file")
	cgo := flags.String("cgo", environmentValue(dependencies.environment, nativeCGOEnvironment), "CGO_ENABLED value")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	request := nativeBuildRequest{goos: *goos, goarch: *goarch, output: *output, cgo: *cgo}
	if err := validateBuildRequest(request); err != nil {
		return err
	}
	version, err := resolveVersion(dependencies)
	if err != nil {
		return err
	}
	return buildNativeCLI(request, version, dependencies, stdout)
}

func runHostBuild(args []string, dependencies nativeBuildDependencies, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("host-build", stderr)
	outputDir := flags.String("output-dir", "", "host output directory")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	if err := validateDirectoryPath("OUTPUT directory", *outputDir); err != nil {
		return err
	}
	if !supportedOS(dependencies.hostOS) {
		return errors.New("unsupported host OS")
	}
	if !supportedArchitecture(dependencies.hostArch) {
		return errors.New("unsupported host architecture")
	}
	version, err := resolveVersion(dependencies)
	if err != nil {
		return err
	}
	cgo, err := resolveHostCGO(dependencies)
	if err != nil {
		return err
	}
	name := "sshc"
	if dependencies.hostOS == "windows" {
		name += ".exe"
	}
	request := nativeBuildRequest{
		goos:   dependencies.hostOS,
		goarch: dependencies.hostArch,
		output: filepath.Join(*outputDir, name),
		cgo:    cgo,
	}
	if err := validateBuildRequest(request); err != nil {
		return err
	}
	if err := runWebBuild(dependencies); err != nil {
		return err
	}
	return buildNativeCLI(request, version, dependencies, stdout)
}

func runBuildMatrix(args []string, dependencies nativeBuildDependencies, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("matrix", stderr)
	targets := flags.String("targets", environmentValue(dependencies.environment, nativeReleaseTargetsEnvironment), "space separated GOOS/GOARCH:CGO records")
	outputDir := flags.String("output-dir", environmentValue(dependencies.environment, nativeReleaseDirEnvironment), "release output directory")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	if err := validateDirectoryPath("release output directory", *outputDir); err != nil {
		return err
	}
	requests, err := parseReleaseTargets(*targets, *outputDir)
	if err != nil {
		return err
	}
	version, err := resolveVersion(dependencies)
	if err != nil {
		return err
	}
	if err := rebuildEmbeddedUIAndCompare(dependencies); err != nil {
		return err
	}
	for _, request := range requests {
		if err := buildAndVerifyStandalone(request, version, dependencies, stdout); err != nil {
			return err
		}
	}
	return nil
}

func runCurrentRelease(args []string, dependencies nativeBuildDependencies, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("release-current", stderr)
	arches := flags.String("arches", environmentValue(dependencies.environment, nativeReleaseArchesEnvironment), "space separated release architectures")
	outputDir := flags.String("output-dir", environmentValue(dependencies.environment, nativeReleaseDirEnvironment), "release output directory")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	if !supportedOS(dependencies.hostOS) {
		return errors.New("unsupported host OS")
	}
	if err := validateDirectoryPath("release output directory", *outputDir); err != nil {
		return err
	}
	architectures, err := parseReleaseArchitectures(*arches)
	if err != nil {
		return err
	}
	cgo := "0"
	if dependencies.hostOS == "darwin" {
		cgo = "1"
	}
	suffix := ""
	if dependencies.hostOS == "windows" {
		suffix = ".exe"
	}
	requests := make([]nativeBuildRequest, 0, len(architectures))
	for _, architecture := range architectures {
		request := nativeBuildRequest{
			goos:   dependencies.hostOS,
			goarch: architecture,
			output: filepath.Join(*outputDir, "sshc-"+dependencies.hostOS+"-"+architecture+suffix),
			cgo:    cgo,
		}
		if err := validateBuildRequest(request); err != nil {
			return err
		}
		requests = append(requests, request)
	}
	version, err := resolveVersion(dependencies)
	if err != nil {
		return err
	}
	if err := rebuildEmbeddedUIAndCompare(dependencies); err != nil {
		return err
	}
	for _, request := range requests {
		if err := buildAndVerifyStandalone(request, version, dependencies, stdout); err != nil {
			return err
		}
	}
	return nil
}

// runEmbeddedUIVerification は、リリースの runner と同じ手順で UI を作り直して照合するだけで、
// バイナリは作らない。照合がその OS で通るかを、公開の前に試すための入口である
// （.github/workflows/release-ui-check.yml、docs/releasing.md）。
func runEmbeddedUIVerification(args []string, dependencies nativeBuildDependencies, stderr io.Writer) error {
	flags := newNativeFlagSet("verify-embedded-ui", stderr)
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	return rebuildEmbeddedUIAndCompare(dependencies)
}

// rebuildEmbeddedUIAndCompare は、配布するバイナリに埋め込む UI を作り直し、コミット済みの
// UI と同じであることを確かめる。リリースの build と、公開の前の照合が同じ手順を通る。
func rebuildEmbeddedUIAndCompare(dependencies nativeBuildDependencies) error {
	if err := runWebBuild(dependencies); err != nil {
		return err
	}
	return verifyEmbeddedUIMatchesCommit(dependencies)
}

// npm はその package のディレクトリで走らせる。--prefix に頼らない。
//
// あの旗の意味は下位命令ごとに揃っていない。`npm install --prefix <dir>` は
// Windows ではカレントの package.json を読みに行き、この repository の root には
// それが無いので ENOENT で落ちる。Linux と macOS では同じ呼び出しが通るので、
// Windows でだけ静かに壊れる種類の違いである。
func runWebBuild(dependencies nativeBuildDependencies) error {
	return dependencies.executor.Run(nativeCommand{
		name:        "npm",
		args:        []string{"run", "build"},
		directory:   "web",
		environment: dependencies.environment,
	})
}

// embeddedUIDirectory は、web のビルドの出力先（web/vite.config.ts の outDir）で、
// Go のバイナリが埋め込む UI（internal/ui/embed.go）である。
const embeddedUIDirectory = "internal/ui/dist"

// verifyEmbeddedUIMatchesCommit は、いま作り直した UI が、コミット済みの UI と同じで
// あることを確かめる。配布するバイナリを作る前に呼ぶ。
//
// Homebrew と APK は、コミット済みの UI をそのまま埋め込む。その UI は、CI の web job
// が Linux で作り直して照合している。リリースの runner（macOS・Windows・Linux）で
// 作り直した UI は、その照合を経ていない。OS ごとのネイティブの依存（rollup、
// lightningcss など）や Node の minor の違いで出力がずれれば、同じ tag の配布物が
// 別々の UI を持つことになる。ずれたら配布物を作らずに止める。
//
// 検査は scripts/ci/check-ui-dist.sh と同じ git status である。sh を通さないので、
// Windows の runner でも同じ検査になる。
func verifyEmbeddedUIMatchesCommit(dependencies nativeBuildDependencies) error {
	changes, err := dependencies.executor.Output(nativeCommand{
		name:        "git",
		args:        []string{"status", "--porcelain=v1", "--untracked-files=all", "--", embeddedUIDirectory},
		environment: dependencies.environment,
	})
	if err != nil {
		return fmt.Errorf("compare the rebuilt embedded UI with the committed one: %w", err)
	}
	if listed := strings.TrimSpace(string(changes)); listed != "" {
		return fmt.Errorf("the embedded UI built on this runner differs from the committed %s:\n%s", embeddedUIDirectory, listed)
	}
	return nil
}

func buildAndVerifyStandalone(request nativeBuildRequest, version string, dependencies nativeBuildDependencies, stdout io.Writer) error {
	if err := buildNativeCLI(request, version, dependencies, stdout); err != nil {
		return err
	}
	return verifyStandaloneArtifact(request, dependencies)
}

func verifyStandaloneArtifact(request nativeBuildRequest, dependencies nativeBuildDependencies) error {
	command := nativeCommand{environment: dependencies.environment}
	if request.goos == "windows" {
		command.name = "pwsh"
		command.args = []string{
			"-NoProfile", "-File", "scripts/verify-artifact-name.ps1",
			"-Artifact", request.output,
			"-OS", request.goos,
			"-Architecture", request.goarch,
		}
	} else {
		command.name = "sh"
		command.args = []string{
			"scripts/verify-artifact-name.sh",
			request.output,
			request.goos,
			request.goarch,
		}
	}
	return dependencies.executor.Run(command)
}

func buildNativeCLI(request nativeBuildRequest, version string, dependencies nativeBuildDependencies, stdout io.Writer) error {
	parent := filepath.Dir(filepath.Clean(request.output))
	if err := dependencies.mkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	fmt.Fprintf(stdout, "==> %s/%s (CGO_ENABLED=%s)\n", request.goos, request.goarch, request.cgo)
	if err := buildOne(request, version, dependencies); err != nil {
		return err
	}
	// ファイル名ではなくバイナリヘッダーからターゲットを検証する。
	// 焼いた直後にしか安く確かめられない。配ってからでは、動かない機械の
	// 上でしか分からない。
	if dependencies.verifyBinary == nil {
		return nil
	}
	return dependencies.verifyBinary(request.output, request.goos, request.goarch)
}

func buildOne(request nativeBuildRequest, version string, dependencies nativeBuildDependencies) error {
	return dependencies.executor.Run(nativeCommand{
		name: "go",
		args: []string{
			"build",
			"-trimpath",
			"-ldflags", "-X main.version=" + version,
			"-o", request.output,
			"./cmd/sshc",
		},
		environment: withTargetEnvironment(dependencies.environment, request),
	})
}

func validateBuildRequest(request nativeBuildRequest) error {
	if strings.TrimSpace(request.goos) == "" {
		return errors.New("GOOS is required")
	}
	if strings.TrimSpace(request.goarch) == "" {
		return errors.New("GOARCH is required")
	}
	if strings.TrimSpace(request.output) == "" {
		return errors.New("OUTPUT is required")
	}
	if strings.TrimSpace(request.cgo) == "" {
		return errors.New("CGO is required")
	}
	if !supportedOS(request.goos) {
		return errors.New("unsupported GOOS")
	}
	if !supportedArchitecture(request.goarch) {
		return errors.New("unsupported GOARCH")
	}
	if request.cgo != "0" && request.cgo != "1" {
		return errors.New("CGO must be 0 or 1")
	}
	if err := validateOutputPath(request.output); err != nil {
		return err
	}
	if request.goos == "windows" && !strings.HasSuffix(request.output, ".exe") {
		return errors.New("Windows OUTPUT must end in .exe")
	}
	if request.goos != "windows" && strings.HasSuffix(request.output, ".exe") {
		return errors.New("non-Windows OUTPUT must not end in .exe")
	}
	return nil
}

func validateOutputPath(output string) error {
	if containsControlCharacter(output) {
		return errors.New("OUTPUT contains a control character")
	}
	clean := filepath.Clean(output)
	if clean == "." || clean == string(filepath.Separator) || filepath.Base(clean) == "." {
		return errors.New("OUTPUT must name a file")
	}
	return nil
}

func validateDirectoryPath(label, directory string) error {
	if strings.TrimSpace(directory) == "" {
		return fmt.Errorf("%s is required", label)
	}
	if containsControlCharacter(directory) {
		return fmt.Errorf("%s is invalid", label)
	}
	clean := filepath.Clean(directory)
	if clean == "." || clean == string(filepath.Separator) {
		return fmt.Errorf("%s must be explicit", label)
	}
	return nil
}

func parseReleaseTargets(value, outputDir string) ([]nativeBuildRequest, error) {
	records := strings.Fields(value)
	if len(records) == 0 {
		return nil, errors.New("release targets are required")
	}
	requests := make([]nativeBuildRequest, 0, len(records))
	seen := make(map[string]bool, len(records))
	for _, record := range records {
		platform, cgo, ok := strings.Cut(record, ":")
		if !ok {
			return nil, errors.New("invalid release target record")
		}
		goos, goarch, ok := strings.Cut(platform, "/")
		if !ok {
			return nil, errors.New("invalid release target platform")
		}
		key := goos + "/" + goarch
		if seen[key] {
			return nil, errors.New("release target is duplicated")
		}
		seen[key] = true
		suffix := ""
		if goos == "windows" {
			suffix = ".exe"
		}
		request := nativeBuildRequest{
			goos: goos, goarch: goarch, cgo: cgo,
			output: filepath.Join(outputDir, "sshc-"+goos+"-"+goarch+suffix),
		}
		if err := validateBuildRequest(request); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func parseReleaseArchitectures(value string) ([]string, error) {
	architectures := strings.Fields(value)
	if len(architectures) != 2 {
		return nil, errors.New("release architectures must contain amd64 and arm64")
	}
	seen := make(map[string]bool, 2)
	for _, architecture := range architectures {
		if !supportedArchitecture(architecture) || seen[architecture] {
			return nil, errors.New("release architectures must contain amd64 and arm64")
		}
		seen[architecture] = true
	}
	if !seen["amd64"] || !seen["arm64"] {
		return nil, errors.New("release architectures must contain amd64 and arm64")
	}
	return architectures, nil
}

func resolveHostCGO(dependencies nativeBuildDependencies) (string, error) {
	if dependencies.hostCGO == "0" || dependencies.hostCGO == "1" {
		return dependencies.hostCGO, nil
	}
	output, err := dependencies.executor.Output(nativeCommand{
		name:        "go",
		args:        []string{"env", "CGO_ENABLED"},
		environment: dependencies.environment,
	})
	if err != nil {
		return "", fmt.Errorf("read host CGO setting: %w", err)
	}
	cgo := strings.TrimSpace(string(output))
	if cgo != "0" && cgo != "1" {
		return "", errors.New("host CGO setting must be 0 or 1")
	}
	return cgo, nil
}

func resolveVersion(dependencies nativeBuildDependencies) (string, error) {
	if version := environmentValue(dependencies.environment, nativeVersionEnvironment); version != "" {
		if err := validateBuildVersion(version); err != nil {
			return "", err
		}
		return version, nil
	}
	output, err := dependencies.executor.Output(nativeCommand{
		name:        "git",
		args:        []string{"describe", "--tags", "--exact-match"},
		environment: dependencies.environment,
	})
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return "dev", nil
	}
	version := strings.TrimSpace(string(output))
	if err := validateBuildVersion(version); err != nil {
		return "", err
	}
	return version, nil
}

// ビルドバージョンは dev、または先頭に v を付けられる SemVer 2.0 とする。空白、引用符、
// 制御文字、option 形式の値を除外し、Go linker token と npm positional value の両方へ
// 安全に渡せるようにする。
func validateBuildVersion(version string) error {
	if version == "dev" {
		return nil
	}
	if !semverBuildVersion.MatchString(version) {
		return errors.New("invalid build version")
	}
	withoutMetadata, _, _ := strings.Cut(version, "+")
	if _, prerelease, found := strings.Cut(withoutMetadata, "-"); found {
		for _, identifier := range strings.Split(prerelease, ".") {
			if len(identifier) > 1 && identifier[0] == '0' && strings.IndexFunc(identifier, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
				return errors.New("invalid build version")
			}
		}
	}
	return nil
}

func containsControlCharacter(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func withTargetEnvironment(environment []string, request nativeBuildRequest) []string {
	result := append([]string(nil), environment...)
	// GOENV をここでも畳み込む。Makefile も override GOENV = off を輸出している
	// が、GNU Make の Windows 移植は変数名を大文字小文字で区別するので、呼び出し元
	// が持っていた別表記が同じ環境に生き残り、大文字小文字を区別しない Windows の
	// プロセス環境ではそちらが勝つ。setEnvironmentValue は表記違いをまとめて畳む
	// ので、子の go build が見る GOENV はどの表記でも off ひとつになる。
	result = setEnvironmentValue(result, "GOENV", "off")
	result = setEnvironmentValue(result, "GOOS", request.goos)
	result = setEnvironmentValue(result, "GOARCH", request.goarch)
	result = setEnvironmentValue(result, "CGO_ENABLED", request.cgo)
	return result
}

func setEnvironmentValue(environment []string, key, value string) []string {
	entry := key + "=" + value
	result := make([]string, 0, len(environment)+1)
	replaced := false
	for _, existing := range environment {
		name, _, ok := strings.Cut(existing, "=")
		if ok && strings.EqualFold(name, key) {
			if !replaced {
				result = append(result, entry)
				replaced = true
			}
			continue
		}
		result = append(result, existing)
	}
	if !replaced {
		result = append(result, entry)
	}
	return result
}

func environmentValue(environment []string, key string) string {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

// canonicalizeNativeEnvironment は Make が export した正確な名前だけを子コマンドへ渡す。
// 正規名の値を大小文字違いの継承値より優先し、別名だけの値と正規名の重複は、検証処理が
// コマンド実行や出力ディレクトリ作成を行う前に拒否する。
func canonicalizeNativeEnvironment(environment []string) ([]string, error) {
	type nativeEnvironmentEntry struct {
		canonicalCount int
		aliasCount     int
	}
	entries := make(map[string]nativeEnvironmentEntry, len(nativeEnvironmentKeys))
	canonicalByFold := make(map[string]string, len(nativeEnvironmentKeys))
	for _, key := range nativeEnvironmentKeys {
		canonicalByFold[strings.ToLower(key)] = key
	}

	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		canonical, native := canonicalByFold[strings.ToLower(name)]
		if !native {
			continue
		}
		counts := entries[canonical]
		if name == canonical {
			counts.canonicalCount++
		} else {
			counts.aliasCount++
		}
		entries[canonical] = counts
	}
	for _, key := range nativeEnvironmentKeys {
		counts := entries[key]
		if counts.canonicalCount > 1 {
			return nil, fmt.Errorf("duplicate native build environment variable %s", key)
		}
		if counts.canonicalCount == 0 && counts.aliasCount != 0 {
			return nil, fmt.Errorf("non-canonical native build environment variable %s", key)
		}
	}

	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			result = append(result, entry)
			continue
		}
		canonical, native := canonicalByFold[strings.ToLower(name)]
		if native && name != canonical {
			continue
		}
		result = append(result, entry)
	}
	return result, nil
}

func supportedOS(value string) bool {
	return value == "darwin" || value == "linux" || value == "windows"
}

func supportedArchitecture(value string) bool {
	return value == "amd64" || value == "arm64"
}

func newNativeFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

func parseNativeFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	return nil
}
