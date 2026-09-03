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

// TestCLIAutomatesPasswordAndTOTPAgainstRealOpenSSH exercises the complete
// product path against an OpenSSH container backed by PAM:
// encrypted vault -> engine handoff -> sshc run -> keyboard-interactive
// password challenge -> time-based OTP challenge.
func TestCLIAutomatesPasswordAndTOTPAgainstRealOpenSSH(t *testing.T) {
	address := requiredTOTPEnvironment(t, "SSHC_TEST_TOTP_ADDR")
	user := requiredTOTPEnvironment(t, "SSHC_TEST_TOTP_USER")
	password := requiredTOTPEnvironment(t, "SSHC_TEST_TOTP_PASSWORD")
	setupKey := requiredTOTPEnvironment(t, "SSHC_TEST_TOTP_SECRET")
	knownHostsPath := requiredTOTPEnvironment(t, "SSHC_TEST_TOTP_KNOWN_HOSTS")
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
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
	configuration := fmt.Sprintf(`Host totp-integration
	HostName %s
	User %s
	Port %s
	PreferredAuthentications keyboard-interactive
	PubkeyAuthentication no
	PasswordAuthentication no
	KbdInteractiveAuthentication yes
	StrictHostKeyChecking yes
`, host, user, port)
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
	binding, err := configurationService.PasswordBinding("totp-integration")
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.SetBound("totp-integration", password, binding); err != nil {
		t.Fatal(err)
	}
	if err := vault.SetCredential(secret.KindTOTP, "integration-token", setupKey); err != nil {
		t.Fatal(err)
	}
	if err := vault.AssignTOTPCredential("totp-integration", "integration-token", binding); err != nil {
		t.Fatal(err)
	}

	engine := start(t, home, "engine")
	waitForFile(t, handoffPath(home), 30*time.Second, engine)
	unlock := startOnTerminal(t, home, "vault", "unlock")
	unlock.expect(t, "Master password: ", 20*time.Second)
	unlock.typeLine(t, proxyVaultPassword)
	if code := unlock.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("vault unlock exit = %d\n%s", code, unlock.output.String())
	}

	command := start(t, home, "ssh", "totp-integration", "--non-interactive", "--", "printf", "totp-container-ok")
	if code := command.wait(t, 30*time.Second); code != 0 {
		t.Fatalf("TOTP run exit = %d\nstdout: %s\nstderr: %s", code, command.Stdout.String(), command.Stderr.String())
	}
	if !strings.Contains(command.Stdout.String(), "totp-container-ok") {
		t.Fatalf("remote command output = %q", command.Stdout.String())
	}
	if strings.Contains(command.Stderr.String(), "requires interaction") {
		t.Fatalf("saved credentials did not answer the PAM challenge: %s", command.Stderr.String())
	}
}

func requiredTOTPEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Skipf("%s is not set; start the OpenSSH PAM TOTP fixture to run this", name)
	}
	return value
}
