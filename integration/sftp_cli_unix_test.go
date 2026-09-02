//go:build unix

package integration

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

const sftpCLIVaultPassword = "sftp-cli-integration-vault-password"

// TestSFTPCLIRoundTripsAgainstRealOpenSSH crosses the complete product path:
// CLI session -> engine transfer queue -> Vault-backed SSH -> OpenSSH SFTP.
// A payload larger than one client chunk proves that append offsets work over
// the real protocol instead of only against the HTTP test double.
func TestSFTPCLIRoundTripsAgainstRealOpenSSH(t *testing.T) {
	address := requiredProxyEnvironment(t, proxyJumpAddressVariable)
	user := requiredProxyEnvironment(t, proxyJumpUserVariable)
	password := requiredProxyEnvironment(t, proxyJumpPasswordVariable)
	knownHostsPath := requiredProxyEnvironment(t, proxyKnownHostsVariable)
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("%s=%q: %v", proxyJumpAddressVariable, address, err)
	}
	knownHosts, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatal(err)
	}

	home := isolatedHome(t)
	sshDirectory := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	const alias = "sftp-cli-integration"
	configuration := fmt.Sprintf(`Host %s
	HostName %s
	User %s
	Port %s
	PreferredAuthentications password
	PubkeyAuthentication no
	KbdInteractiveAuthentication no
	PasswordAuthentication yes
	StrictHostKeyChecking yes
`, alias, host, user, port)
	if err := os.WriteFile(filepath.Join(sshDirectory, "config"), []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDirectory, "known_hosts"), knownHosts, 0o600); err != nil {
		t.Fatal(err)
	}

	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	transactions := storage.NewManager(workspace, time.Now, rand.Reader)
	configurationService := application.NewService(workspace, transactions)
	vault := secret.NewService(workspace, transactions, time.Now)
	if err := vault.Initialise(sftpCLIVaultPassword); err != nil {
		t.Fatal(err)
	}
	binding, err := configurationService.PasswordBinding(alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.SetBound(alias, password, binding); err != nil {
		t.Fatal(err)
	}

	engine := start(t, home, "engine")
	waitForFile(t, handoffPath(home), 30*time.Second, engine)
	unlock := startOnTerminal(t, home, "vault", "unlock")
	unlock.expect(t, "Master password: ", 20*time.Second)
	unlock.typeLine(t, sftpCLIVaultPassword)
	if code := unlock.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("vault unlock exit = %d\n%s", code, unlock.output.String())
	}

	source := filepath.Join(home, "upload")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	pattern := []byte("sshc-sftp-cli-e2e\x00")
	large := bytes.Repeat(pattern, (16<<20)/len(pattern)+2)
	large = large[:16<<20]
	want := map[string][]byte{
		"small.txt":          []byte("put and get through OpenSSH\n"),
		"nested/payload.bin": large,
		"nested/empty.bin":   {},
	}
	for relative, contents := range want {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(relative)), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	remoteRoot := fmt.Sprintf("/tmp/sshc-sftp-cli-%d", time.Now().UnixNano())
	put := start(t, home, "sftp", "put", alias, source, remoteRoot, "--recursive", "--jobs", "3", "--json")
	if code := put.wait(t, 90*time.Second); code != 0 {
		t.Fatalf("sftp put exit = %d\nstdout: %s\nstderr: %s", code, put.Stdout.String(), put.Stderr.String())
	}
	if !strings.Contains(put.Stdout.String(), `"success":true`) {
		t.Fatalf("sftp put did not return a success envelope: %s", put.Stdout.String())
	}

	destination := filepath.Join(home, "download")
	get := start(t, home, "sftp", "get", alias, remoteRoot, destination, "--recursive", "--jobs", "3", "--split-size", "16", "--split-jobs", "4", "--chunk-size", "8", "--json")
	if code := get.wait(t, 90*time.Second); code != 0 {
		t.Fatalf("sftp get exit = %d\nstdout: %s\nstderr: %s", code, get.Stdout.String(), get.Stderr.String())
	}
	if !strings.Contains(get.Stdout.String(), `"success":true`) {
		t.Fatalf("sftp get did not return a success envelope: %s", get.Stdout.String())
	}
	for relative, expected := range want {
		actual, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read downloaded %s: %v", relative, err)
		}
		if !bytes.Equal(actual, expected) {
			t.Fatalf("downloaded %s differs: got %d bytes, want %d", relative, len(actual), len(expected))
		}
	}
}
