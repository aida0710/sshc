package buildcontract

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

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
	process := exec.Command(command.name, command.args...)
	process.Env = command.environment
	process.Dir = command.directory
	process.Stdout = executor.stdout
	process.Stderr = executor.stderr
	return process.Run()
}

func (executor osNativeExecutor) Output(command nativeCommand) ([]byte, error) {
	process := exec.Command(command.name, command.args...)
	process.Env = command.environment
	process.Dir = command.directory
	return process.Output()
}

type nativeBuildDeps struct {
	hostOS      string
	hostArch    string
	hostCGO     string
	environment []string
	executor    nativeCommandExecutor
	mkdirAll    func(string, os.FileMode) error
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
	return runNativeBuild(args, nativeBuildDeps{
		hostOS:      runtime.GOOS,
		hostArch:    runtime.GOARCH,
		hostCGO:     environmentValue(os.Environ(), "CGO_ENABLED"),
		environment: os.Environ(),
		executor:    osNativeExecutor{stdout: stdout, stderr: stderr},
		mkdirAll:    os.MkdirAll,
	}, stdout, stderr)
}

func runNativeBuild(args []string, deps nativeBuildDeps, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("command is required")
	}
	if deps.executor == nil || deps.mkdirAll == nil {
		return errors.New("native build dependencies are incomplete")
	}

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
	goos := flags.String("goos", "", "target operating system")
	goarch := flags.String("goarch", "", "target architecture")
	output := flags.String("output", "", "output file")
	cgo := flags.String("cgo", "", "CGO_ENABLED value")
	if err := parseNativeFlags(flags, args); err != nil {
		return err
	}
	request := nativeBuildRequest{goos: *goos, goarch: *goarch, output: *output, cgo: *cgo}
	if err := validateBuildRequest(request); err != nil {
		return err
	}
	version := resolveVersion(deps)
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
	return buildNativeCLI(request, resolveVersion(deps), deps, stdout)
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
	requests, err := parseDesktopBundles(*bundles, *expectedHost, *resourceRoot)
	if err != nil {
		return err
	}
	version := resolveVersion(deps)
	for _, request := range requests {
		if err := buildNativeCLI(request, version, deps, stdout); err != nil {
			return err
		}
	}
	return nil
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
	version := resolveVersion(deps)
	if version == "dev" {
		return nil
	}
	return deps.executor.Run(nativeCommand{
		name: "npm",
		args: []string{
			"version",
			"--prefix", *directory,
			"--allow-same-version",
			"--no-git-tag-version",
			strings.TrimPrefix(version, "v"),
		},
		environment: deps.environment,
	})
}

func runBuildMatrix(args []string, deps nativeBuildDeps, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("matrix", stderr)
	targets := flags.String("targets", "", "space separated GOOS/GOARCH:CGO records")
	outputDir := flags.String("output-dir", "", "release output directory")
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
	version := resolveVersion(deps)
	for _, request := range requests {
		if err := buildNativeCLI(request, version, deps, stdout); err != nil {
			return err
		}
	}
	return nil
}

func runCurrentRelease(args []string, deps nativeBuildDeps, stdout, stderr io.Writer) error {
	flags := newNativeFlagSet("release-current", stderr)
	arches := flags.String("arches", "", "space separated release architectures")
	outputDir := flags.String("output-dir", "", "release output directory")
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
	version := resolveVersion(deps)
	for _, request := range requests {
		if err := buildNativeCLI(request, version, deps, stdout); err != nil {
			return err
		}
	}
	return nil
}

func buildNativeCLI(request nativeBuildRequest, version string, deps nativeBuildDeps, stdout io.Writer) error {
	parent := filepath.Dir(filepath.Clean(request.output))
	if err := deps.mkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	fmt.Fprintf(stdout, "==> %s/%s (CGO_ENABLED=%s)\n", request.goos, request.goarch, request.cgo)
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
	if strings.ContainsRune(output, '\x00') {
		return errors.New("OUTPUT contains an invalid character")
	}
	if strings.ContainsAny(output, "*?[") {
		return errors.New("OUTPUT must not contain glob characters")
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
	if strings.ContainsRune(directory, '\x00') || strings.ContainsAny(directory, "*?[") {
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

func resolveVersion(deps nativeBuildDeps) string {
	if version := strings.TrimSpace(environmentValue(deps.environment, "VERSION")); version != "" {
		return version
	}
	output, err := deps.executor.Output(nativeCommand{
		name:        "git",
		args:        []string{"describe", "--tags", "--exact-match"},
		environment: deps.environment,
	})
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return "dev"
	}
	return strings.TrimSpace(string(output))
}

func withTargetEnvironment(environment []string, request nativeBuildRequest) []string {
	result := append([]string(nil), environment...)
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
		if ok && strings.EqualFold(name, key) {
			return value
		}
	}
	return ""
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
