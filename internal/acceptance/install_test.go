package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const maintenanceFixture = `#!/bin/sh
printf '%s|%s\n' "$0" "$*" >> "$SSHC_TEST_SERVICE_LOG"
if [ "$*" = "service refresh" ] && [ "${SSHC_TEST_FAIL_REFRESH:-}" = "1" ]; then
	exit 17
fi
if [ "$*" = "service disable" ] && [ "${SSHC_TEST_FAIL_DISABLE:-}" = "1" ]; then
	exit 18
fi
`

func writeMaintenanceFixture(t *testing.T, directory string) string {
	t.Helper()
	path := filepath.Join(directory, "fixture-sshc")
	if err := os.WriteFile(path, []byte(maintenanceFixture), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func runInstallMake(t *testing.T, environment []string, target string, assignments ...string) (string, error) {
	t.Helper()
	arguments := append([]string{"--no-print-directory", target}, assignments...)
	command := exec.Command("make", arguments...)
	command.Dir = filepath.Join("..", "..")
	command.Env = append(os.Environ(), environment...)
	output, err := command.CombinedOutput()
	return string(output), err
}

// install-binaryは、古い宛先を新しい実行可能ファイルへ置き換えてから、その置かれた
// ファイル自身にrefreshを依頼する。sourceを呼ぶ実装では、登録される絶対パスが
// リポジトリ側へ戻ってしまうため、このログがそれを区別する。
func TestInstallBinaryAtomicallyReplacesTheCLIAndRefreshesTheService(t *testing.T) {
	root := t.TempDir()
	source := writeMaintenanceFixture(t, root)
	installDirectory := filepath.Join(root, "installed")
	if err := os.MkdirAll(installDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(installDirectory, "sshc")
	if err := os.WriteFile(destination, []byte("old executable\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "service.log")

	output, err := runInstallMake(t, []string{"SSHC_TEST_SERVICE_LOG=" + logPath}, "install-binary",
		"INSTALL_SOURCE="+source, "INSTALL_DIR="+installDirectory)
	if err != nil {
		t.Fatalf("make install-binary: %v\n%s", err, output)
	}

	installed, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != maintenanceFixture {
		t.Fatalf("installed bytes = %q, want fixture bytes", installed)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("installed mode = %o, want 755", info.Mode().Perm())
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(logged) != destination+"|service refresh\n" {
		t.Fatalf("service log = %q", logged)
	}
	matches, err := filepath.Glob(filepath.Join(installDirectory, ".sshc.install.*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("staging residue = %v, %v", matches, err)
	}
}

// refreshはrename後にしか実行できない。したがって失敗時は新しいCLIを残すが、全体を
// 成功とは報告せず、どこまで完了したかを明示する。
func TestInstallBinaryKeepsTheNewCLIButReportsARefreshFailure(t *testing.T) {
	root := t.TempDir()
	source := writeMaintenanceFixture(t, root)
	installDirectory := filepath.Join(root, "installed")
	logPath := filepath.Join(root, "service.log")

	output, err := runInstallMake(t, []string{
		"SSHC_TEST_SERVICE_LOG=" + logPath,
		"SSHC_TEST_FAIL_REFRESH=1",
	}, "install-binary", "INSTALL_SOURCE="+source, "INSTALL_DIR="+installDirectory)
	if err == nil {
		t.Fatalf("refresh failure was reported as success:\n%s", output)
	}
	installed, readErr := os.ReadFile(filepath.Join(installDirectory, "sshc"))
	if readErr != nil || string(installed) != maintenanceFixture {
		t.Fatalf("installed after refresh failure = %q, %v", installed, readErr)
	}
	if !strings.Contains(output, "CLI was installed") || !strings.Contains(output, "login service") {
		t.Fatalf("partial success is not explained:\n%s", output)
	}
}

// stageに失敗した時点ではrenameしていないので、既存のCLIは1 byteも変わらない。
func TestInstallBinaryKeepsTheOldCLIWhenStagingFails(t *testing.T) {
	root := t.TempDir()
	installDirectory := filepath.Join(root, "installed")
	if err := os.MkdirAll(installDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(installDirectory, "sshc")
	if err := os.WriteFile(destination, []byte("old executable\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	output, err := runInstallMake(t, nil, "install-binary",
		"INSTALL_SOURCE="+filepath.Join(root, "missing"), "INSTALL_DIR="+installDirectory)
	if err == nil {
		t.Fatalf("missing source was reported as success:\n%s", output)
	}
	current, readErr := os.ReadFile(destination)
	if readErr != nil || string(current) != "old executable\n" {
		t.Fatalf("old destination changed to %q, %v", current, readErr)
	}
}

// disableが失敗したのに実行ファイルだけ消すと、KeepAliveが存在しないパスを起動し
// 続ける。削除よりdisableが先であることを、失敗側の状態で証明する。
func TestUninstallBinaryKeepsTheCLIWhenServiceDisableFails(t *testing.T) {
	root := t.TempDir()
	maintenance := writeMaintenanceFixture(t, root)
	installDirectory := filepath.Join(root, "installed")
	if err := os.MkdirAll(installDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(installDirectory, "sshc")
	if err := os.WriteFile(destination, []byte("installed executable\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "service.log")

	output, err := runInstallMake(t, []string{
		"SSHC_TEST_SERVICE_LOG=" + logPath,
		"SSHC_TEST_FAIL_DISABLE=1",
	}, "uninstall-binary", "MAINTENANCE_BINARY="+maintenance, "INSTALL_DIR="+installDirectory)
	if err == nil {
		t.Fatalf("disable failure was reported as success:\n%s", output)
	}
	current, readErr := os.ReadFile(destination)
	if readErr != nil || string(current) != "installed executable\n" {
		t.Fatalf("installed CLI was removed or changed: %q, %v", current, readErr)
	}
}

func TestUninstallBinaryDisablesTheServiceBeforeRemovingTheCLI(t *testing.T) {
	root := t.TempDir()
	maintenance := writeMaintenanceFixture(t, root)
	installDirectory := filepath.Join(root, "installed")
	if err := os.MkdirAll(installDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(installDirectory, "sshc")
	if err := os.WriteFile(destination, []byte("installed executable\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "service.log")

	output, err := runInstallMake(t, []string{"SSHC_TEST_SERVICE_LOG=" + logPath}, "uninstall-binary",
		"MAINTENANCE_BINARY="+maintenance, "INSTALL_DIR="+installDirectory)
	if err != nil {
		t.Fatalf("make uninstall-binary: %v\n%s", err, output)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil || string(logged) != maintenance+"|service disable\n" {
		t.Fatalf("service log = %q, %v", logged, err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("installed CLI still exists: %v", err)
	}
}
