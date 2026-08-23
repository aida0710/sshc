//go:build unix

// ここが検査するのは `make install` が置く `~/.local/bin/sshc` である。
//
// Windows の導入は同じ製品ではない。そちらは NSIS の per-user インストーラ、
// HKCU、ユーザー PATH、`sshc.exe` であり、その実証は Windows Task 8/9 の
// package smoke が持っている。Windows に GNU Make を用意してこの recipe を
// 走らせても、確かめられるのは向こうで出荷しないものである。
package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const installFixture = "#!/bin/sh\ntrue\n"

func writeInstallFixture(t *testing.T, directory string) string {
	t.Helper()
	path := filepath.Join(directory, "fixture-sshc")
	if err := os.WriteFile(path, []byte(installFixture), 0o755); err != nil {
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

// install-binaryは、古い宛先を新しい実行可能ファイルへ置き換える。ログイン時起動を
// OSへ任せるようになってからは、置いたファイル自身に何かを依頼することはない。
func TestInstallBinaryAtomicallyReplacesTheCLI(t *testing.T) {
	root := t.TempDir()
	source := writeInstallFixture(t, root)
	installDirectory := filepath.Join(root, "installed")
	if err := os.MkdirAll(installDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(installDirectory, "sshc")
	if err := os.WriteFile(destination, []byte("old executable\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	output, err := runInstallMake(t, nil, "install-binary",
		"INSTALL_SOURCE="+source, "INSTALL_DIR="+installDirectory)
	if err != nil {
		t.Fatalf("make install-binary: %v\n%s", err, output)
	}

	installed, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != installFixture {
		t.Fatalf("installed bytes = %q, want fixture bytes", installed)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("installed mode = %o, want 755", info.Mode().Perm())
	}
	matches, err := filepath.Glob(filepath.Join(installDirectory, ".sshc.install.*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("staging residue = %v, %v", matches, err)
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

// stage先は推測した名前へ直接copyせず、mktempが排他的に作ったregular fileでなければ
// ならない。このshimはproductionのinstallを置き換え、呼ばれた時点の宛先を観察する。
// PID名へ事前配置されたsymlinkを追う実装では、この検査に失敗する。
func TestInstallBinaryStagesIntoAnExclusivelyCreatedRegularFile(t *testing.T) {
	root := t.TempDir()
	source := writeInstallFixture(t, root)
	installDirectory := filepath.Join(root, "installed")
	shimDirectory := filepath.Join(root, "shim")
	if err := os.MkdirAll(shimDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	installShim := `#!/bin/sh
if [ "$1" != "-m" ] || [ "$2" != "0755" ] || [ ! -f "$4" ] || [ -L "$4" ]; then
	exit 91
fi
/bin/cp "$3" "$4"
/bin/chmod 0755 "$4"
`
	if err := os.WriteFile(filepath.Join(shimDirectory, "install"), []byte(installShim), 0o755); err != nil {
		t.Fatal(err)
	}

	output, err := runInstallMake(t, []string{
		"PATH=" + shimDirectory + ":" + os.Getenv("PATH"),
	}, "install-binary", "INSTALL_SOURCE="+source, "INSTALL_DIR="+installDirectory)
	if err != nil {
		t.Fatalf("make install-binary: %v\n%s", err, output)
	}
}

// uninstall-binaryはインストール済みのCLIを取り除く。ログイン時起動をOSへ任せる
// ようになってからは、削除の前に何かを止める必要がない。
func TestUninstallBinaryRemovesTheInstalledCLI(t *testing.T) {
	root := t.TempDir()
	installDirectory := filepath.Join(root, "installed")
	if err := os.MkdirAll(installDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(installDirectory, "sshc")
	if err := os.WriteFile(destination, []byte("installed executable\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	output, err := runInstallMake(t, nil, "uninstall-binary", "INSTALL_DIR="+installDirectory)
	if err != nil {
		t.Fatalf("make uninstall-binary: %v\n%s", err, output)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("installed CLI still exists: %v", err)
	}
}
