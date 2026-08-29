package app

import (
	"testing"

	"sshc/internal/sshclient"
)

func TestCLIConnectionAlwaysShowsBasicConnectionProgress(t *testing.T) {
	connection, err := NewCLIConnection(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := connection.parts.dialer.Verbosity(); got != sshclient.Brief {
		t.Fatalf("CLI verbosity = %d, want basic progress", got)
	}
}
