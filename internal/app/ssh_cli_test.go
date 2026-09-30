package app

import (
	"testing"

	"sshc/internal/connectionlog"
)

func TestCLIConnectionAlwaysShowsBasicConnectionProgress(t *testing.T) {
	connection, err := NewCLIConnection(CLIConnectionOptions{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got := connection.parts.dialer.Verbosity(); got != connectionlog.Brief {
		t.Fatalf("CLI verbosity = %d, want basic progress", got)
	}
}
