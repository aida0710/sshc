package sftp

import (
	"os"
	"os/exec"
	"testing"
)

func openOpenSSHTestClient(t *testing.T) *Client {
	t.Helper()
	serverPath := ""
	for _, candidate := range []string{"/usr/lib/openssh/sftp-server", "/usr/libexec/sftp-server"} {
		if _, err := os.Stat(candidate); err == nil {
			serverPath = candidate
			break
		}
	}
	if serverPath == "" {
		t.Skip("OpenSSH sftp-server is not installed")
	}
	// The client closes stdin during cleanup; t.Context is already cancelled
	// before cleanup runs and would kill the server before it reads EOF.
	server := exec.Command(serverPath)
	reader, err := server.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	writer, err := server.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	client, err := newClientPipe(reader, writer)
	if err != nil {
		_ = writer.Close()
		_ = server.Wait()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		if err := server.Wait(); err != nil {
			t.Errorf("OpenSSH fixture exited: %v", err)
		}
	})
	return client
}
