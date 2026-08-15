package buildcontract

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type recordingNativeExecutor struct {
	commands  []nativeCommand
	output    []byte
	outputErr error
}

func (executor *recordingNativeExecutor) Run(command nativeCommand) error {
	executor.commands = append(executor.commands, command)
	return nil
}

func (executor *recordingNativeExecutor) Output(command nativeCommand) ([]byte, error) {
	executor.commands = append(executor.commands, command)
	return executor.output, executor.outputErr
}

func TestNativeBuildRejectsInvalidExplicitInputs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "empty OS", args: []string{"build", "--goos", "", "--goarch", "amd64", "--output", "dist/sshc", "--cgo", "0"}, wantErr: "GOOS is required"},
		{name: "empty architecture", args: []string{"build", "--goos", "linux", "--goarch", "", "--output", "dist/sshc", "--cgo", "0"}, wantErr: "GOARCH is required"},
		{name: "empty output", args: []string{"build", "--goos", "linux", "--goarch", "amd64", "--output", "", "--cgo", "0"}, wantErr: "OUTPUT is required"},
		{name: "empty cgo", args: []string{"build", "--goos", "linux", "--goarch", "amd64", "--output", "dist/sshc", "--cgo", ""}, wantErr: "CGO is required"},
		{name: "unsupported OS", args: []string{"build", "--goos", "freebsd", "--goarch", "amd64", "--output", "dist/sshc", "--cgo", "0"}, wantErr: "unsupported GOOS"},
		{name: "unsupported architecture", args: []string{"build", "--goos", "linux", "--goarch", "386", "--output", "dist/sshc", "--cgo", "0"}, wantErr: "unsupported GOARCH"},
		{name: "invalid cgo", args: []string{"build", "--goos", "linux", "--goarch", "amd64", "--output", "dist/sshc", "--cgo", "2"}, wantErr: "CGO must be 0 or 1"},
		{name: "directory output", args: []string{"build", "--goos", "linux", "--goarch", "amd64", "--output", ".", "--cgo", "0"}, wantErr: "OUTPUT must name a file"},
		{name: "control in output", args: []string{"build", "--goos", "linux", "--goarch", "amd64", "--output", "dist/sshc\nother", "--cgo", "0"}, wantErr: "OUTPUT contains a control character"},
		{name: "Windows suffix missing", args: []string{"build", "--goos", "windows", "--goarch", "amd64", "--output", "dist/sshc-windows-amd64", "--cgo", "0"}, wantErr: "Windows OUTPUT must end in .exe"},
		{name: "Unix suffix present", args: []string{"build", "--goos", "linux", "--goarch", "amd64", "--output", "dist/sshc-linux-amd64.exe", "--cgo", "0"}, wantErr: "non-Windows OUTPUT must not end in .exe"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := &recordingNativeExecutor{}
			err := runNativeBuild(test.args, nativeBuildDeps{
				hostOS:      "linux",
				hostArch:    "amd64",
				hostCGO:     "0",
				environment: []string{nativeVersionEnvironment + "=v1.2.3"},
				executor:    executor,
				mkdirAll:    os.MkdirAll,
			}, io.Discard, io.Discard)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("runNativeBuild() error = %v, want containing %q", err, test.wantErr)
			}
			if len(executor.commands) != 0 {
				t.Fatalf("invalid input executed %d commands", len(executor.commands))
			}
		})
	}
}

func TestNativeBuildPreservesOutputPathWithSpacesInArgv(t *testing.T) {
	executor := &recordingNativeExecutor{}
	output := filepath.Join(t.TempDir(), "artifact output", "sshc-windows-amd64.exe")
	err := runNativeBuild([]string{
		"build",
		"--goos", "windows",
		"--goarch", "amd64",
		"--output", output,
		"--cgo", "0",
	}, nativeBuildDeps{
		hostOS:      "darwin",
		hostArch:    "arm64",
		hostCGO:     "1",
		environment: []string{"PATH=/usr/bin", nativeVersionEnvironment + "=v1.2.3", "GOOS=linux"},
		executor:    executor,
		mkdirAll:    os.MkdirAll,
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("runNativeBuild() error: %v", err)
	}
	if len(executor.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(executor.commands))
	}
	command := executor.commands[0]
	wantArgs := []string{"build", "-trimpath", "-ldflags", "-X main.version=v1.2.3", "-o", output, "./cmd/sshc"}
	if command.name != "go" || !reflect.DeepEqual(command.args, wantArgs) {
		t.Fatalf("command = %s %q, want go %q", command.name, command.args, wantArgs)
	}
	for key, want := range map[string]string{"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0"} {
		if got := recordedEnvValue(command.environment, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if info, err := os.Stat(filepath.Dir(output)); err != nil || !info.IsDir() {
		t.Fatalf("output parent was not created: info=%v err=%v", info, err)
	}
}

func TestNativeBuildRejectsMalformedExplicitVersionBeforeSideEffects(t *testing.T) {
	executor := &recordingNativeExecutor{}
	mkdirCalls := 0
	err := runNativeBuild([]string{
		"build",
		"--goos", "linux",
		"--goarch", "amd64",
		"--output", "dist/sshc-linux-amd64",
		"--cgo", "0",
	}, nativeBuildDeps{
		hostOS:      "linux",
		hostArch:    "amd64",
		hostCGO:     "0",
		environment: []string{nativeVersionEnvironment + "=v1.2.3 -s"},
		executor:    executor,
		mkdirAll: func(string, os.FileMode) error {
			mkdirCalls++
			return nil
		},
	}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invalid build version") {
		t.Fatalf("runNativeBuild() error = %v, want invalid build version", err)
	}
	if len(executor.commands) != 0 || mkdirCalls != 0 {
		t.Fatalf("malformed version executed commands=%d mkdir=%d", len(executor.commands), mkdirCalls)
	}
}

func TestNativeBuildRejectsMalformedGitVersionBeforeBuildOrMkdir(t *testing.T) {
	executor := &recordingNativeExecutor{output: []byte("v1.2.3 -w\n")}
	mkdirCalls := 0
	err := runNativeBuild([]string{
		"build",
		"--goos", "linux",
		"--goarch", "amd64",
		"--output", "dist/sshc-linux-amd64",
		"--cgo", "0",
	}, nativeBuildDeps{
		hostOS:   "linux",
		hostArch: "amd64",
		hostCGO:  "0",
		executor: executor,
		mkdirAll: func(string, os.FileMode) error {
			mkdirCalls++
			return nil
		},
	}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invalid build version") {
		t.Fatalf("runNativeBuild() error = %v, want invalid build version", err)
	}
	if len(executor.commands) != 1 || executor.commands[0].name != "git" || mkdirCalls != 0 {
		t.Fatalf("malformed git version commands=%#v mkdir=%d, want only git describe", executor.commands, mkdirCalls)
	}
}

func TestHostBuildRunsWebBuildOnlyAfterValidation(t *testing.T) {
	executor := &recordingNativeExecutor{}
	err := runNativeBuild([]string{"host-build", "--output-dir", "bin"}, nativeBuildDeps{
		hostOS:      "linux",
		hostArch:    "amd64",
		hostCGO:     "0",
		environment: []string{nativeVersionEnvironment + "=v1.2.3"},
		executor:    executor,
		mkdirAll:    func(string, os.FileMode) error { return nil },
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("runNativeBuild() error: %v", err)
	}
	if len(executor.commands) != 2 {
		t.Fatalf("commands = %#v, want npm then go build", executor.commands)
	}
	if command := executor.commands[0]; command.name != "npm" || !reflect.DeepEqual(command.args, []string{"run", "build", "--prefix", "web"}) {
		t.Fatalf("first command = %s %q, want npm web build", command.name, command.args)
	}
	if executor.commands[1].name != "go" {
		t.Fatalf("second command = %s, want go", executor.commands[1].name)
	}
}

func TestHostBuildRejectsMalformedVersionBeforeReadingCGO(t *testing.T) {
	executor := &recordingNativeExecutor{output: []byte("1\n")}
	mkdirCalls := 0
	err := runNativeBuild([]string{"host-build", "--output-dir", "bin"}, nativeBuildDeps{
		hostOS:      "linux",
		hostArch:    "amd64",
		environment: []string{nativeVersionEnvironment + "=v1.2.3 -w"},
		executor:    executor,
		mkdirAll: func(string, os.FileMode) error {
			mkdirCalls++
			return nil
		},
	}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invalid build version") {
		t.Fatalf("runNativeBuild() error = %v, want invalid build version", err)
	}
	if len(executor.commands) != 0 || mkdirCalls != 0 {
		t.Fatalf("invalid host version commands=%#v mkdir=%d, want zero actions", executor.commands, mkdirCalls)
	}
}

func TestReleaseCurrentRejectsUnsupportedHostBeforeAllActions(t *testing.T) {
	executor := &recordingNativeExecutor{}
	mkdirCalls := 0
	err := runNativeBuild([]string{
		"release-current",
		"--arches", "amd64 arm64",
		"--output-dir", "dist",
	}, nativeBuildDeps{
		hostOS:      "freebsd",
		hostArch:    "amd64",
		hostCGO:     "0",
		environment: []string{nativeVersionEnvironment + "=v1.2.3"},
		executor:    executor,
		mkdirAll: func(string, os.FileMode) error {
			mkdirCalls++
			return nil
		},
	}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unsupported host OS") {
		t.Fatalf("runNativeBuild() error = %v, want unsupported host", err)
	}
	if len(executor.commands) != 0 || mkdirCalls != 0 {
		t.Fatalf("unsupported release host executed commands=%d mkdir=%d", len(executor.commands), mkdirCalls)
	}
}

func TestNativeDesktopGuardUsesActualHostBeforeBuilding(t *testing.T) {
	executor := &recordingNativeExecutor{}
	err := runNativeBuild([]string{
		"desktop",
		"--host", "windows",
		"--resource-root", filepath.Join(t.TempDir(), "resources"),
		"--bundles", "win32-arm64:windows:arm64:0:sshc.exe win32-x64:windows:amd64:0:sshc.exe",
	}, nativeBuildDeps{
		hostOS:      "linux",
		hostArch:    "amd64",
		hostCGO:     "0",
		environment: []string{nativeVersionEnvironment + "=dev", "GOOS=windows"},
		executor:    executor,
		mkdirAll:    os.MkdirAll,
	}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "requires windows host; actual host is linux") {
		t.Fatalf("runNativeBuild() error = %v, want wrong-host rejection", err)
	}
	if len(executor.commands) != 0 {
		t.Fatalf("wrong-host desktop target executed %d commands", len(executor.commands))
	}
}

func TestNativeHostGuardRejectsOverrideableTargetOS(t *testing.T) {
	executor := &recordingNativeExecutor{}
	err := runNativeBuild([]string{"guard-host", "--host", "windows"}, nativeBuildDeps{
		hostOS:      "linux",
		hostArch:    "amd64",
		hostCGO:     "0",
		environment: []string{"GOOS=windows"},
		executor:    executor,
		mkdirAll:    os.MkdirAll,
	}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "requires windows host; actual host is linux") {
		t.Fatalf("runNativeBuild() error = %v, want actual-host rejection", err)
	}
	if len(executor.commands) != 0 {
		t.Fatalf("host guard executed %d commands", len(executor.commands))
	}
}

func TestNativeDesktopBuildsExactWindowsResourceLayout(t *testing.T) {
	executor := &recordingNativeExecutor{}
	root := filepath.Join(t.TempDir(), "desktop resources")
	err := runNativeBuild([]string{
		"desktop",
		"--host", "windows",
		"--resource-root", root,
	}, nativeBuildDeps{
		hostOS:   "windows",
		hostArch: "amd64",
		hostCGO:  "0",
		environment: []string{
			nativeVersionEnvironment + "=dev",
			nativeWindowsBundlesEnvironment + "=win32-arm64:windows:arm64:0:sshc.exe win32-x64:windows:amd64:0:sshc.exe",
		},
		executor: executor,
		mkdirAll: os.MkdirAll,
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("runNativeBuild() error: %v", err)
	}
	wantOutputs := []string{
		filepath.Join(root, "win32-arm64", "sshc.exe"),
		filepath.Join(root, "win32-x64", "sshc.exe"),
	}
	if got := commandOutputs(executor.commands); !reflect.DeepEqual(got, wantOutputs) {
		t.Fatalf("outputs = %q, want %q", got, wantOutputs)
	}
	if len(executor.commands) != 5 {
		t.Fatalf("commands = %#v, want install, web, two CLI builds, and dist", executor.commands)
	}
	wantPrefix := []nativeCommand{
		{name: "npm", args: []string{"install", "--prefix", "desktop"}},
		{name: "npm", args: []string{"run", "build", "--prefix", "web"}},
	}
	for index, want := range wantPrefix {
		got := executor.commands[index]
		if got.name != want.name || !reflect.DeepEqual(got.args, want.args) {
			t.Errorf("command %d = %s %q, want %s %q", index, got.name, got.args, want.name, want.args)
		}
	}
	if dist := executor.commands[4]; dist.name != "npm" || !reflect.DeepEqual(dist.args, []string{"run", "dist:win", "--prefix", "desktop"}) {
		t.Errorf("dist command = %s %q", dist.name, dist.args)
	}
	for _, command := range executor.commands {
		if command.name != "go" {
			continue
		}
		if got := recordedEnvValue(command.environment, "CGO_ENABLED"); got != "0" {
			t.Errorf("Windows CGO_ENABLED = %q, want 0", got)
		}
	}
}

func TestNativeDesktopUsesHostSpecificBundleChannelAndDistScript(t *testing.T) {
	tests := []struct {
		host      string
		bundles   string
		bundleEnv string
		dist      string
	}{
		{host: "darwin", bundles: "mac-arm64:darwin:arm64:1:sshc mac-x64:darwin:amd64:1:sshc", bundleEnv: nativeMacBundlesEnvironment, dist: "dist:mac"},
		{host: "linux", bundles: "linux-arm64:linux:arm64:0:sshc linux-x64:linux:amd64:0:sshc", bundleEnv: nativeLinuxBundlesEnvironment, dist: "dist:linux"},
		{host: "windows", bundles: "win32-arm64:windows:arm64:0:sshc.exe win32-x64:windows:amd64:0:sshc.exe", bundleEnv: nativeWindowsBundlesEnvironment, dist: "dist:win"},
	}
	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			executor := &recordingNativeExecutor{}
			err := runNativeBuild([]string{
				"desktop",
				"--host", test.host,
				"--resource-root", filepath.Join(t.TempDir(), "resources"),
			}, nativeBuildDeps{
				hostOS:      test.host,
				hostArch:    "amd64",
				hostCGO:     "0",
				environment: []string{nativeVersionEnvironment + "=dev", test.bundleEnv + "=" + test.bundles},
				executor:    executor,
				mkdirAll:    func(string, os.FileMode) error { return nil },
			}, io.Discard, io.Discard)
			if err != nil {
				t.Fatalf("runNativeBuild() error: %v", err)
			}
			dist := executor.commands[len(executor.commands)-1]
			want := []string{"run", test.dist, "--prefix", "desktop"}
			if dist.name != "npm" || !reflect.DeepEqual(dist.args, want) {
				t.Errorf("dist command = %s %q, want npm %q", dist.name, dist.args, want)
			}
		})
	}
}

func TestBuildMatrixRunsWebThenVerifiesEveryArtifact(t *testing.T) {
	executor := &recordingNativeExecutor{}
	err := runNativeBuild([]string{
		"matrix",
		"--targets", "linux/amd64:0 linux/arm64:0",
		"--output-dir", "release output",
	}, nativeBuildDeps{
		hostOS:      "linux",
		hostArch:    "amd64",
		hostCGO:     "0",
		environment: []string{nativeVersionEnvironment + "=v1.2.3"},
		executor:    executor,
		mkdirAll:    func(string, os.FileMode) error { return nil },
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("runNativeBuild() error: %v", err)
	}
	if len(executor.commands) != 5 {
		t.Fatalf("commands = %#v, want web plus two build/verifier pairs", executor.commands)
	}
	if executor.commands[0].name != "npm" {
		t.Fatalf("first command = %s, want npm", executor.commands[0].name)
	}
	for _, index := range []int{2, 4} {
		if executor.commands[index].name != "sh" {
			t.Errorf("command %d = %s, want shell verifier", index, executor.commands[index].name)
		}
	}
}

func TestNativeExecutorProgramAllowlistIsExact(t *testing.T) {
	want := map[string]bool{"go": true, "git": true, "npm": true, "sh": true, "pwsh": true}
	for _, program := range []string{"go", "git", "npm", "sh", "pwsh", "ssh", "bash", "cmd", "powershell", "curl"} {
		if got := allowedNativeProgram(program); got != want[program] {
			t.Errorf("allowedNativeProgram(%q) = %v, want %v", program, got, want[program])
		}
	}
}

func TestReleaseCurrentUsesActualHostForBothArchitectures(t *testing.T) {
	tests := []struct {
		hostOS       string
		hostCGO      string
		wantSuffix   string
		wantBuildCGO string
	}{
		{hostOS: "darwin", hostCGO: "1", wantBuildCGO: "1"},
		{hostOS: "linux", hostCGO: "1", wantBuildCGO: "0"},
		{hostOS: "windows", hostCGO: "1", wantSuffix: ".exe", wantBuildCGO: "0"},
	}
	for _, test := range tests {
		t.Run(test.hostOS, func(t *testing.T) {
			executor := &recordingNativeExecutor{}
			root := filepath.Join(t.TempDir(), "release output")
			err := runNativeBuild([]string{
				"release-current",
				"--arches", "amd64 arm64",
				"--output-dir", root,
			}, nativeBuildDeps{
				hostOS:      test.hostOS,
				hostArch:    "amd64",
				hostCGO:     test.hostCGO,
				environment: []string{nativeVersionEnvironment + "=v2.0.0", "GOOS=freebsd"},
				executor:    executor,
				mkdirAll:    os.MkdirAll,
			}, io.Discard, io.Discard)
			if err != nil {
				t.Fatalf("runNativeBuild() error: %v", err)
			}
			wantOutputs := []string{
				filepath.Join(root, "sshc-"+test.hostOS+"-amd64"+test.wantSuffix),
				filepath.Join(root, "sshc-"+test.hostOS+"-arm64"+test.wantSuffix),
			}
			if got := commandOutputs(executor.commands); !reflect.DeepEqual(got, wantOutputs) {
				t.Fatalf("outputs = %q, want %q", got, wantOutputs)
			}
			if len(executor.commands) != 5 {
				t.Fatalf("commands = %d, want web build plus two build/verifier pairs: %#v", len(executor.commands), executor.commands)
			}
			if command := executor.commands[0]; command.name != "npm" || !reflect.DeepEqual(command.args, []string{"run", "build", "--prefix", "web"}) {
				t.Fatalf("first command = %s %q, want npm web build", command.name, command.args)
			}
			for index, command := range executor.commands {
				if index == 0 || index%2 == 0 {
					continue
				}
				if got := recordedEnvValue(command.environment, "GOOS"); got != test.hostOS {
					t.Errorf("build GOOS = %q, want actual host %q", got, test.hostOS)
				}
				if got := recordedEnvValue(command.environment, "CGO_ENABLED"); got != test.wantBuildCGO {
					t.Errorf("build CGO_ENABLED = %q, want %q", got, test.wantBuildCGO)
				}
			}
			for index, architecture := range []string{"amd64", "arm64"} {
				verify := executor.commands[2+index*2]
				wantArtifact := wantOutputs[index]
				if test.hostOS == "windows" {
					want := []string{"-NoProfile", "-File", "scripts/verify-artifact-name.ps1", "-Artifact", wantArtifact, "-OS", "windows", "-Architecture", architecture}
					if verify.name != "pwsh" || !reflect.DeepEqual(verify.args, want) {
						t.Errorf("Windows verifier = %s %q, want pwsh %q", verify.name, verify.args, want)
					}
				} else {
					want := []string{"scripts/verify-artifact-name.sh", wantArtifact, test.hostOS, architecture}
					if verify.name != "sh" || !reflect.DeepEqual(verify.args, want) {
						t.Errorf("Unix verifier = %s %q, want sh %q", verify.name, verify.args, want)
					}
				}
			}
		})
	}
}

func TestDesktopVersionUsesNPMArgvWithoutShellInterpolation(t *testing.T) {
	executor := &recordingNativeExecutor{}
	directory := filepath.Join("desktop package", "app")
	err := runNativeBuild([]string{"desktop-version", "--directory", directory}, nativeBuildDeps{
		hostOS:      "windows",
		hostArch:    "amd64",
		hostCGO:     "0",
		environment: []string{nativeVersionEnvironment + "=v3.4.5"},
		executor:    executor,
		mkdirAll:    os.MkdirAll,
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("runNativeBuild() error: %v", err)
	}
	if len(executor.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(executor.commands))
	}
	wantArgs := []string{"version", "--prefix", directory, "--allow-same-version", "--no-git-tag-version", "--", "3.4.5"}
	if command := executor.commands[0]; command.name != "npm" || !reflect.DeepEqual(command.args, wantArgs) {
		t.Fatalf("command = %s %q, want npm %q", command.name, command.args, wantArgs)
	}
}

func TestBuildVersionGrammar(t *testing.T) {
	tests := []struct {
		version string
		valid   bool
	}{
		{version: "dev", valid: true},
		{version: "0.0.0", valid: true},
		{version: "v1.2.3", valid: true},
		{version: "v1.2.3-alpha.1+build.5", valid: true},
		{version: "v1.2", valid: false},
		{version: "v01.2.3", valid: false},
		{version: "v1.2.3-01", valid: false},
		{version: "v1.2.3 -s", valid: false},
		{version: `v1.2.3" -w`, valid: false},
		{version: "--help", valid: false},
		{version: "v1.2.3\n-w", valid: false},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			err := validateBuildVersion(test.version)
			if (err == nil) != test.valid {
				t.Errorf("validateBuildVersion(%q) error=%v, valid=%v", test.version, err, test.valid)
			}
		})
	}
}

func TestTargetEnvironmentReplacementIsCaseInsensitiveAndUnique(t *testing.T) {
	environment := []string{
		"PATH=/usr/bin",
		"GoOs=freebsd",
		"GOOS=plan9",
		"goarch=386",
		"Cgo_Enabled=1",
	}
	request := nativeBuildRequest{goos: "windows", goarch: "arm64", cgo: "0"}
	got := withTargetEnvironment(environment, request)
	for key, want := range map[string]string{"GOOS": "windows", "GOARCH": "arm64", "CGO_ENABLED": "0"} {
		matches := 0
		for _, entry := range got {
			name, value, present := strings.Cut(entry, "=")
			if present && strings.EqualFold(name, key) {
				matches++
				if name != key || value != want {
					t.Errorf("%s entry = %q, want %s=%s", key, entry, key, want)
				}
			}
		}
		if matches != 1 {
			t.Errorf("%s entries = %d, want exactly one", key, matches)
		}
	}
}

func recordedEnvValue(environment []string, key string) string {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, key) {
			return value
		}
	}
	return ""
}

func commandOutputs(commands []nativeCommand) []string {
	outputs := make([]string, 0, len(commands))
	for _, command := range commands {
		for index, argument := range command.args {
			if argument == "-o" && index+1 < len(command.args) {
				outputs = append(outputs, command.args[index+1])
				break
			}
		}
	}
	return outputs
}
