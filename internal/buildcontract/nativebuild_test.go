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
		{name: "glob output", args: []string{"build", "--goos", "linux", "--goarch", "amd64", "--output", "dist/sshc*", "--cgo", "0"}, wantErr: "OUTPUT must not contain glob characters"},
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
				environment: []string{"VERSION=v1.2.3"},
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
		environment: []string{"PATH=/usr/bin", "VERSION=v1.2.3", "GOOS=linux"},
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
		environment: []string{"VERSION=dev", "GOOS=windows"},
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
		"--bundles", "win32-arm64:windows:arm64:0:sshc.exe win32-x64:windows:amd64:0:sshc.exe",
	}, nativeBuildDeps{
		hostOS:      "windows",
		hostArch:    "amd64",
		hostCGO:     "0",
		environment: []string{"VERSION=dev"},
		executor:    executor,
		mkdirAll:    os.MkdirAll,
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
	for _, command := range executor.commands {
		if got := recordedEnvValue(command.environment, "CGO_ENABLED"); got != "0" {
			t.Errorf("Windows CGO_ENABLED = %q, want 0", got)
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
				environment: []string{"VERSION=v2.0.0", "GOOS=freebsd"},
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
			for _, command := range executor.commands {
				if got := recordedEnvValue(command.environment, "GOOS"); got != test.hostOS {
					t.Errorf("build GOOS = %q, want actual host %q", got, test.hostOS)
				}
				if got := recordedEnvValue(command.environment, "CGO_ENABLED"); got != test.wantBuildCGO {
					t.Errorf("build CGO_ENABLED = %q, want %q", got, test.wantBuildCGO)
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
		environment: []string{"VERSION=v3.4.5"},
		executor:    executor,
		mkdirAll:    os.MkdirAll,
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("runNativeBuild() error: %v", err)
	}
	if len(executor.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(executor.commands))
	}
	wantArgs := []string{"version", "--prefix", directory, "--allow-same-version", "--no-git-tag-version", "3.4.5"}
	if command := executor.commands[0]; command.name != "npm" || !reflect.DeepEqual(command.args, wantArgs) {
		t.Fatalf("command = %s %q, want npm %q", command.name, command.args, wantArgs)
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
