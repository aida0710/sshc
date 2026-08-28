package integration

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// TestBuiltBinaryRunsAgainstVirtualTelnetServer proves the installed command
// shape through main, real TCP, Telnet negotiation, stdout JSON, and process
// exit status. Protocol edge cases remain in the faster cmd/sshc tests.
func TestBuiltBinaryRunsAgainstVirtualTelnetServer(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	serverDone := make(chan error, 1)
	go func() { serverDone <- serveIntegrationTelnet(listener) }()

	process := start(t, isolatedHome(t),
		"telnet", listener.Addr().String(), "--non-interactive",
		"--connect-timeout", "2s", "--timeout", "3s", "--settle", "0",
		"--expect", `virtual# $`, "--json", "--", "show", "version",
	)
	if code := process.wait(t, 5*time.Second); code != 0 {
		t.Fatalf("sshc telnet --non-interactive exit = %d\nstdout: %s\nstderr: %s", code, process.Stdout.String(), process.Stderr.String())
	}
	if process.Stderr.String() != "" {
		t.Fatalf("sshc telnet --non-interactive stderr = %q", process.Stderr.String())
	}
	var report struct {
		Transport  string   `json:"transport"`
		Target     string   `json:"target"`
		Success    bool     `json:"success"`
		Matched    bool     `json:"matched"`
		Transcript string   `json:"transcript"`
		Warnings   []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(process.Stdout.String()), &report); err != nil {
		t.Fatalf("decode sshc telnet --non-interactive JSON: %v\n%s", err, process.Stdout.String())
	}
	if report.Transport != "telnet" || report.Target != listener.Addr().String() || !report.Success || !report.Matched {
		t.Fatalf("sshc telnet --non-interactive report = %#v", report)
	}
	if report.Transcript != "virtual telnet version 1\r\nvirtual# " || len(report.Warnings) != 1 {
		t.Fatalf("sshc telnet --non-interactive output = %#v", report)
	}
	select {
	case err := <-serverDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("virtual Telnet server did not stop")
	}
}

func serveIntegrationTelnet(listener net.Listener) error {
	connection, err := listener.Accept()
	if err != nil {
		return err
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(4 * time.Second)); err != nil {
		return err
	}
	const iac, will, do, echo = byte(255), byte(251), byte(253), byte(1)
	if _, err := connection.Write([]byte{iac, will, echo}); err != nil {
		return err
	}
	reply := make([]byte, 3)
	if _, err := io.ReadFull(connection, reply); err != nil {
		return err
	}
	if !bytes.Equal(reply, []byte{iac, do, echo}) {
		return fmt.Errorf("Telnet negotiation reply = %v", reply)
	}
	command, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil {
		return err
	}
	if command != "show version\r\n" {
		return fmt.Errorf("Telnet command = %q", command)
	}
	_, err = io.WriteString(connection, "virtual telnet version 1\r\nvirtual# ")
	return err
}
