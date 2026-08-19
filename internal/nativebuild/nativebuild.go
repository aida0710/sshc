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
	nativeMacBundlesEnvironment     = "SSHC_NATIVE_MAC_BUNDLES"
	nativeLinuxBundlesEnvironment   = "SSHC_NATIVE_LINUX_BUNDLES"
	nativeWindowsBundlesEnvironment = "SSHC_NATIVE_WINDOWS_BUNDLES"
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
	nativeMacBundlesEnvironment,
	nativeLinuxBundlesEnvironment,
	nativeWindowsBundlesEnvironment,
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

type nativeBuildDeps struct {
	hostOS      string
	hostArch    string
	hostCGO     string
	environment []string
	executor    nativeCommandExecutor
	mkdirAll    func(string, os.FileMode) error
	// verifyBinary は、焼けた実体が本当にその行き先のものかを見る。
	//
	// **継ぎ目にしてあるのは、検査が実体を読むからである。** 記録するだけの
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

// RunNativeBuild is the portable entry point used by the Makefile wrapper.
// It passes commands as argv and target values through an explicit inherited
// environment, so paths and versions never need shell interpolation.
func RunNativeBuild(args []string, stdout, stderr io.Writer) error {
	environment := os.Environ()
	return runNativeBuild(args, nativeBuildDeps{
		hostOS:       runtime.GOOS,
		hostArch:     runtime.GOARCH,
		hostCGO:      os.Getenv("CGO_ENABLED"),
		environment:  environment,
		executor:     osNativeExecutor{stdout: stdout, stderr: stderr},
		mkdirAll:     os.MkdirAll,
		verifyBinary: VerifyBinaryArchitecture,
	}, stdout, stderr)
}

func runNativeBuild(args []string, deps nativeBuildDeps, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("command is required")
	}
	if deps.executor == nil || deps.mkdirAll == nil {
		return errors.New("native build dependencies are incomplete")
	}
	environment, err := canonicalizeNativeEnvironment(deps.environment)
	if err != nil {
		return err
	}
	deps.environment = environment

	switch args[0] {
	case "build":
		return runExplicitBuild(args[1:], deps, stdout, stderr)
	case "host-build":
		return runHostBuild(args[1:], deps, stdout, stderr)
	case "guard-host":
		return runHostGuard(args[1:], deps, stderr)
	case "desktop":
		return runDesktopBuild(args[1:], deps, stdout, stderr)
	case "desktop-version":
		return runDesktopVersion(args[1:], deps, stderr)
	case "matrix":
		return runBuildMatrix(args[1:], deps, stdout, stderr)
	case "release-current":
		return runCurrentRelease(args[1:], deps, stdout, stderr)
	default:
		return errors.New("unsupported native build command")
	}
}

func runHostGuard(args []string, deps nativeBuildDeps, stderr io.Writer) error {
	flags := newNativeFlagSet("guard-host", stderr)
	expectedHost := flags.String("host", "", "required host operating system")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	if !supportedOS(*expectedHost) {
		return errors.New("unsupported required host")
	}
	if deps.hostOS != *expectedHost {
		return fmt.Errorf("target requires %s host; actual host is %s", *expectedHost, deps.hostOS)
	}
	return nil
}

func runExplicitBuild(args []string, deps nativeBuildDeps, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("build", stderr)
	goos := flags.String("goos", environmentValue(deps.environment, nativeGOOSEnvironment), "target operating system")
	goarch := flags.String("goarch", environmentValue(deps.environment, nativeGOARCHEnvironment), "target architecture")
	output := flags.String("output", environmentValue(deps.environment, nativeOutputEnvironment), "output file")
	cgo := flags.String("cgo", environmentValue(deps.environment, nativeCGOEnvironment), "CGO_ENABLED value")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	request := nativeBuildRequest{goos: *goos, goarch: *goarch, output: *output, cgo: *cgo}
	if err := validateBuildRequest(request); err != nil {
		return err
	}
	version, err := resolveVersion(deps)
	if err != nil {
		return err
	}
	return buildNativeCLI(request, version, deps, stdout)
}

func runHostBuild(args []string, deps nativeBuildDeps, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("host-build", stderr)
	outputDir := flags.String("output-dir", "", "host output directory")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	if err := validateDirectoryPath("OUTPUT directory", *outputDir); err != nil {
		return err
	}
	if !supportedOS(deps.hostOS) {
		return errors.New("unsupported host OS")
	}
	if !supportedArchitecture(deps.hostArch) {
		return errors.New("unsupported host architecture")
	}
	version, err := resolveVersion(deps)
	if err != nil {
		return err
	}
	cgo, err := resolveHostCGO(deps)
	if err != nil {
		return err
	}
	name := "sshc"
	if deps.hostOS == "windows" {
		name += ".exe"
	}
	request := nativeBuildRequest{
		goos:   deps.hostOS,
		goarch: deps.hostArch,
		output: filepath.Join(*outputDir, name),
		cgo:    cgo,
	}
	if err := validateBuildRequest(request); err != nil {
		return err
	}
	if err := runWebBuild(deps); err != nil {
		return err
	}
	return buildNativeCLI(request, version, deps, stdout)
}

func runDesktopBuild(args []string, deps nativeBuildDeps, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("desktop", stderr)
	expectedHost := flags.String("host", "", "required host operating system")
	resourceRoot := flags.String("resource-root", "", "desktop resource root")
	bundles := flags.String("bundles", "", "space separated native bundle records")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	if !supportedOS(*expectedHost) {
		return errors.New("unsupported desktop host")
	}
	if deps.hostOS != *expectedHost {
		return fmt.Errorf("desktop target requires %s host; actual host is %s", *expectedHost, deps.hostOS)
	}
	if err := validateDirectoryPath("resource root", *resourceRoot); err != nil {
		return err
	}
	bundleValue := *bundles
	if bundleValue == "" {
		bundleValue = environmentValue(deps.environment, desktopBundleEnvironment(*expectedHost))
	}
	requests, err := parseDesktopBundles(bundleValue, *expectedHost, *resourceRoot)
	if err != nil {
		return err
	}
	version, err := resolveVersion(deps)
	if err != nil {
		return err
	}
	if err := deps.executor.Run(nativeCommand{
		name:        "npm",
		args:        []string{"install"},
		directory:   "desktop",
		environment: deps.environment,
	}); err != nil {
		return err
	}
	if err := runWebBuild(deps); err != nil {
		return err
	}
	for _, request := range requests {
		if err := buildNativeCLI(request, version, deps, stdout); err != nil {
			return err
		}
	}
	if err := updateDesktopVersion("desktop", version, deps); err != nil {
		return err
	}
	return deps.executor.Run(nativeCommand{
		name:        "npm",
		args:        []string{"run", desktopDistScript(*expectedHost)},
		directory:   "desktop",
		environment: deps.environment,
	})
}

func runDesktopVersion(args []string, deps nativeBuildDeps, stderr io.Writer) error {
	flags := newNativeFlagSet("desktop-version", stderr)
	directory := flags.String("directory", "", "desktop package directory")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	if err := validateDirectoryPath("desktop directory", *directory); err != nil {
		return err
	}
	version, err := resolveVersion(deps)
	if err != nil {
		return err
	}
	if version == "dev" {
		return nil
	}
	return updateDesktopVersion(*directory, version, deps)
}

func updateDesktopVersion(directory, version string, deps nativeBuildDeps) error {
	if version == "dev" {
		return nil
	}
	return deps.executor.Run(nativeCommand{
		name:      "npm",
		directory: directory,
		args: []string{
			"version",
			"--allow-same-version",
			"--no-git-tag-version",
			"--",
			strings.TrimPrefix(version, "v"),
		},
		environment: deps.environment,
	})
}

func runBuildMatrix(args []string, deps nativeBuildDeps, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("matrix", stderr)
	targets := flags.String("targets", environmentValue(deps.environment, nativeReleaseTargetsEnvironment), "space separated GOOS/GOARCH:CGO records")
	outputDir := flags.String("output-dir", environmentValue(deps.environment, nativeReleaseDirEnvironment), "release output directory")
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
	version, err := resolveVersion(deps)
	if err != nil {
		return err
	}
	if err := runWebBuild(deps); err != nil {
		return err
	}
	for _, request := range requests {
		if err := buildAndVerifyStandalone(request, version, deps, stdout); err != nil {
			return err
		}
	}
	return nil
}

func runCurrentRelease(args []string, deps nativeBuildDeps, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("release-current", stderr)
	arches := flags.String("arches", environmentValue(deps.environment, nativeReleaseArchesEnvironment), "space separated release architectures")
	outputDir := flags.String("output-dir", environmentValue(deps.environment, nativeReleaseDirEnvironment), "release output directory")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	if !supportedOS(deps.hostOS) {
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
	if deps.hostOS == "darwin" {
		cgo = "1"
	}
	suffix := ""
	if deps.hostOS == "windows" {
		suffix = ".exe"
	}
	requests := make([]nativeBuildRequest, 0, len(architectures))
	for _, architecture := range architectures {
		request := nativeBuildRequest{
			goos:   deps.hostOS,
			goarch: architecture,
			output: filepath.Join(*outputDir, "sshc-"+deps.hostOS+"-"+architecture+suffix),
			cgo:    cgo,
		}
		if err := validateBuildRequest(request); err != nil {
			return err
		}
		requests = append(requests, request)
	}
	version, err := resolveVersion(deps)
	if err != nil {
		return err
	}
	if err := runWebBuild(deps); err != nil {
		return err
	}
	for _, request := range requests {
		if err := buildAndVerifyStandalone(request, version, deps, stdout); err != nil {
			return err
		}
	}
	return nil
}

// **npm はその package のディレクトリで走らせる。--prefix に頼らない。**
//
// あの旗の意味は下位命令ごとに揃っていない。`npm ci --prefix desktop` は
// desktop/package.json を読むが、`npm install --prefix desktop` は Windows では
// カレントの package.json を読みに行き、この repository の root にはそれが無い
// ので ENOENT で落ちる——Linux と macOS では同じ呼び出しが通るので、
// **Windows でだけ、束に CLI が入らないまま electron-builder が警告一つで
// 進み、空の resources\cli を配ることになっていた。**
func runWebBuild(deps nativeBuildDeps) error {
	return deps.executor.Run(nativeCommand{
		name:        "npm",
		args:        []string{"run", "build"},
		directory:   "web",
		environment: deps.environment,
	})
}

func buildAndVerifyStandalone(request nativeBuildRequest, version string, deps nativeBuildDeps, stdout io.Writer) error {
	if err := buildNativeCLI(request, version, deps, stdout); err != nil {
		return err
	}
	return verifyStandaloneArtifact(request, deps)
}

func verifyStandaloneArtifact(request nativeBuildRequest, deps nativeBuildDeps) error {
	command := nativeCommand{environment: deps.environment}
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
	return deps.executor.Run(command)
}

func buildNativeCLI(request nativeBuildRequest, version string, deps nativeBuildDeps, stdout io.Writer) error {
	parent := filepath.Dir(filepath.Clean(request.output))
	if err := deps.mkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	fmt.Fprintf(stdout, "==> %s/%s (CGO_ENABLED=%s)\n", request.goos, request.goarch, request.cgo)
	if err := buildOne(request, version, deps); err != nil {
		return err
	}
	// **名前ではなく中身を見る。** 束ごとに正しい実体を入れているかは、
	// 焼いた直後にしか安く確かめられない——配ってからでは、動かない機械の
	// 上でしか分からない。
	if deps.verifyBinary == nil {
		return nil
	}
	return deps.verifyBinary(request.output, request.goos, request.goarch)
}

func buildOne(request nativeBuildRequest, version string, deps nativeBuildDeps) error {
	return deps.executor.Run(nativeCommand{
		name: "go",
		args: []string{
			"build",
			"-trimpath",
			"-ldflags", "-X main.version=" + version,
			"-o", request.output,
			"./cmd/sshc",
		},
		environment: withTargetEnvironment(deps.environment, request),
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

func parseDesktopBundles(value, hostOS, resourceRoot string) ([]nativeBuildRequest, error) {
	records := strings.Fields(value)
	if len(records) != 2 {
		return nil, errors.New("desktop bundles must contain exactly two architectures")
	}
	seen := make(map[string]bool, 2)
	requests := make([]nativeBuildRequest, 0, 2)
	for _, record := range records {
		parts := strings.Split(record, ":")
		if len(parts) != 5 {
			return nil, errors.New("invalid desktop bundle record")
		}
		name, goos, goarch, cgo, executable := parts[0], parts[1], parts[2], parts[3], parts[4]
		if goos != hostOS || name != desktopBundleName(hostOS, goarch) {
			return nil, errors.New("desktop bundle does not match host layout")
		}
		if seen[goarch] {
			return nil, errors.New("desktop bundle architecture is duplicated")
		}
		seen[goarch] = true
		wantCGO := "0"
		if hostOS == "darwin" {
			wantCGO = "1"
		}
		wantExecutable := "sshc"
		if hostOS == "windows" {
			wantExecutable = "sshc.exe"
		}
		if cgo != wantCGO || executable != wantExecutable {
			return nil, errors.New("desktop bundle has invalid CGO or executable suffix")
		}
		output := filepath.Join(resourceRoot, name, executable)
		if err := ensurePathWithin(resourceRoot, output); err != nil {
			return nil, err
		}
		request := nativeBuildRequest{goos: goos, goarch: goarch, output: output, cgo: cgo}
		if err := validateBuildRequest(request); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	if !seen["amd64"] || !seen["arm64"] {
		return nil, errors.New("desktop bundles must include amd64 and arm64")
	}
	return requests, nil
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

func desktopBundleName(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "darwin/arm64":
		return "mac-arm64"
	case "darwin/amd64":
		return "mac-x64"
	case "linux/arm64":
		return "linux-arm64"
	case "linux/amd64":
		return "linux-x64"
	case "windows/arm64":
		return "win32-arm64"
	case "windows/amd64":
		return "win32-x64"
	default:
		return ""
	}
}

func ensurePathWithin(root, path string) error {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("resource output escapes resource root")
	}
	return nil
}

func resolveHostCGO(deps nativeBuildDeps) (string, error) {
	if deps.hostCGO == "0" || deps.hostCGO == "1" {
		return deps.hostCGO, nil
	}
	output, err := deps.executor.Output(nativeCommand{
		name:        "go",
		args:        []string{"env", "CGO_ENABLED"},
		environment: deps.environment,
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

func resolveVersion(deps nativeBuildDeps) (string, error) {
	if version := environmentValue(deps.environment, nativeVersionEnvironment); version != "" {
		if err := validateBuildVersion(version); err != nil {
			return "", err
		}
		return version, nil
	}
	output, err := deps.executor.Output(nativeCommand{
		name:        "git",
		args:        []string{"describe", "--tags", "--exact-match"},
		environment: deps.environment,
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

// Build versions are either dev or SemVer 2.0 with an optional leading v.
// This grammar excludes whitespace, quotes, controls, and option-like values,
// so the same value is one safe Go linker token and one npm positional value.
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

func desktopBundleEnvironment(goos string) string {
	switch goos {
	case "darwin":
		return nativeMacBundlesEnvironment
	case "linux":
		return nativeLinuxBundlesEnvironment
	case "windows":
		return nativeWindowsBundlesEnvironment
	default:
		return ""
	}
}

func desktopDistScript(goos string) string {
	switch goos {
	case "darwin":
		return "dist:mac"
	case "linux":
		return "dist:linux"
	case "windows":
		return "dist:win"
	default:
		return ""
	}
}

func withTargetEnvironment(environment []string, request nativeBuildRequest) []string {
	result := append([]string(nil), environment...)
	// GOENV をここでも畳み込む。Makefile も override GOENV = off を輸出している
	// が、GNU Make の Windows 移植は変数名を大文字小文字で区別するので、呼び出し元
	// が持っていた別綴りが同じ環境に生き残り、大文字小文字を区別しない Windows の
	// プロセス環境ではそちらが勝つ。setEnvironmentValue は綴り違いをまとめて畳む
	// ので、子の go build が見る GOENV はどの綴りでも off ひとつになる。
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

// canonicalizeNativeEnvironment makes the exact Make-exported spelling the
// only spelling commands can observe. An exact canonical value is authoritative
// over inherited case aliases. Alias-only and duplicate exact entries fail
// before validation can execute a command or create an output directory.
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
