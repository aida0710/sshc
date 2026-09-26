package sshclient

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"sshc/internal/commandconn"
)

func TestExpiredProxySSOTokensRequestLoginOnTheEngineMachine(t *testing.T) {
	err := proxyFailure(io.EOF, "aws: [ERROR]: Error when retrieving token from sso: Token has expired and refresh failed")
	if !errors.Is(err, ErrProxyAuthenticationRequired) || !errors.Is(err, io.EOF) {
		t.Fatalf("authentication classification lost the cause: %v", err)
	}
	for _, instruction := range []string{"sshcエンジンが動いているマシン", "同じOSユーザー", "aws sso login --profile", "--use-device-code"} {
		if !strings.Contains(err.Error(), instruction) {
			t.Errorf("login instructions omit %q: %v", instruction, err)
		}
	}
	var explained *ExplainedError
	if !errors.As(err, &explained) || !strings.Contains(explained.Err.Error(), "Token has expired") {
		t.Fatal("the original AWS error is not available for detailed logs")
	}
}

func TestAnExpiredSSOProxyStopsTheActualHandshakeWithLoginInstructions(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	process := exec.Command(self, "-test.run=^TestExpiredSSOProxyProcess$")
	process.Env = append(os.Environ(), "SSHC_TEST_PROXY_SSO=1")
	connection, err := commandconn.Start(process, "fixture-aws")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	_, _, _, err = newClientConn(t.Context(), connection, "fixture:22", &ssh.ClientConfig{
		User: "fixture", HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if !errors.Is(err, ErrProxyAuthenticationRequired) {
		t.Fatalf("SSO failure did not survive the subprocess and SSH handshake: %v", err)
	}
	var output strings.Builder
	describeProxyExit(newTracer(Detailed, &output), connection)
	if !strings.Contains(output.String(), "ProxyCommandの終了：コード 1") {
		t.Fatalf("missing proxy exit status: %s", output.String())
	}
}

func TestExpiredSSOProxyProcess(t *testing.T) {
	if os.Getenv("SSHC_TEST_PROXY_SSO") != "1" {
		t.Skip("only runs as the proxy subprocess")
	}
	_, _ = io.WriteString(os.Stderr, "aws: [ERROR]: Error when retrieving token from sso: Token has expired and refresh failed\n")
	os.Exit(1)
}

func TestProxyNetworkErrorsDoNotRequestANewSSOLogin(t *testing.T) {
	for _, complaint := range []string{
		"aws: command not found",
		"Error when retrieving token from sso: Connection timed out",
		"SessionManagerPlugin is not found",
	} {
		err := proxyFailure(io.EOF, complaint)
		if errors.Is(err, ErrProxyAuthenticationRequired) || !strings.Contains(err.Error(), complaint) {
			t.Fatalf("network or setup failure misclassified: %v", err)
		}
	}
}
