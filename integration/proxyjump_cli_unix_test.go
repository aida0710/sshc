//go:build unix

package integration

import (
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

const (
	proxyJumpAddressVariable     = "SSHC_TEST_PROXY_JUMP_ADDR"
	proxyJumpUserVariable        = "SSHC_TEST_PROXY_JUMP_USER"
	proxyJumpPasswordVariable    = "SSHC_TEST_PROXY_JUMP_PASSWORD"
	proxyDestinationAddrVariable = "SSHC_TEST_PROXY_DEST_ADDR"
	proxyDestinationUserVariable = "SSHC_TEST_PROXY_DEST_USER"
	proxyDestinationPassVariable = "SSHC_TEST_PROXY_DEST_PASSWORD"
	proxyKnownHostsVariable      = "SSHC_TEST_PROXY_KNOWN_HOSTS"
	proxyVaultPassword           = "proxyjump-integration-vault-password"
)

// TestCLIUsesVaultPasswordsAcrossARealProxyJump runs the complete product path:
// encrypted vault -> engine handoff -> `sshc ssh` -> OpenSSH jump host ->
// OpenSSH destination. The two servers use different passwords so a direct
// connection or accidental credential reuse cannot make the test pass.
func TestCLIUsesVaultPasswordsAcrossARealProxyJump(t *testing.T) {
	jumpAddress := requiredProxyEnvironment(t, proxyJumpAddressVariable)
	jumpUser := requiredProxyEnvironment(t, proxyJumpUserVariable)
	jumpPassword := requiredProxyEnvironment(t, proxyJumpPasswordVariable)
	destinationAddress := requiredProxyEnvironment(t, proxyDestinationAddrVariable)
	destinationUser := requiredProxyEnvironment(t, proxyDestinationUserVariable)
	destinationPassword := requiredProxyEnvironment(t, proxyDestinationPassVariable)
	knownHostsPath := requiredProxyEnvironment(t, proxyKnownHostsVariable)

	jumpHost, jumpPort, err := net.SplitHostPort(jumpAddress)
	if err != nil {
		t.Fatalf("%s=%q: %v", proxyJumpAddressVariable, jumpAddress, err)
	}
	destinationHost, destinationPort, err := net.SplitHostPort(destinationAddress)
	if err != nil {
		t.Fatalf("%s=%q: %v", proxyDestinationAddrVariable, destinationAddress, err)
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
	configuration := fmt.Sprintf(`Host proxyjump-integration-jump
	HostName %s
	User %s
	Port %s
	PreferredAuthentications password
	PubkeyAuthentication no
	KbdInteractiveAuthentication no
	PasswordAuthentication yes
	StrictHostKeyChecking yes

Host proxyjump-integration-destination
	HostName %s
	User %s
	Port %s
	ProxyJump proxyjump-integration-jump
	PreferredAuthentications password
	PubkeyAuthentication no
	KbdInteractiveAuthentication no
	PasswordAuthentication yes
	StrictHostKeyChecking yes
`, jumpHost, jumpUser, jumpPort, destinationHost, destinationUser, destinationPort)
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
	if err := vault.Initialise(proxyVaultPassword); err != nil {
		t.Fatal(err)
	}
	for alias, password := range map[string]string{
		"proxyjump-integration-jump":        jumpPassword,
		"proxyjump-integration-destination": destinationPassword,
	} {
		binding, err := configurationService.PasswordBinding(alias)
		if err != nil {
			t.Fatalf("binding %s: %v", alias, err)
		}
		if err := vault.SetBound(alias, password, binding); err != nil {
			t.Fatalf("password %s: %v", alias, err)
		}
	}

	engine := start(t, home, "engine")
	waitForFile(t, handoffPath(home), 30*time.Second, engine)
	unlock := startOnTerminal(t, home, "vault", "unlock")
	unlock.expect(t, "Master password: ", 20*time.Second)
	unlock.typeLine(t, proxyVaultPassword)
	if code := unlock.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("vault unlock exit = %d\n%s", code, unlock.output.String())
	}

	connection := startOnTerminal(t, home, "ssh", "proxyjump-integration-destination")
	connection.expect(t, "proxyjump-integration-jump に接続しました（1/2）", 30*time.Second)
	connection.expect(t, "proxyjump-integration-destination に接続しました（2/2）", 30*time.Second)
	// 2/2 は最終SSH handshakeの完了であり、remote shellの開始完了ではない。
	// ここより前の入力は認証回答への混入を防ぐため意図的に捨てられる。
	connection.expect(t, "[sshc] セッションを開始しました。", 20*time.Second)
	connection.typeLine(t, "echo proxyjump-cli-e2e-ok; exit")
	connection.expect(t, "proxyjump-cli-e2e-ok", 20*time.Second)
	if code := connection.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("ssh exit = %d\n%s", code, connection.output.String())
	}
	transcript := connection.output.String()
	if strings.Contains(transcript, "Password for ") || strings.Contains(transcript, "Saved password was rejected") {
		t.Fatalf("the real ProxyJump asked for a password instead of using the vault:\n%s", transcript)
	}
}

func requiredProxyEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Skipf("%s is not set; start the two-sshd ProxyJump fixture to run this", name)
	}
	return value
}
