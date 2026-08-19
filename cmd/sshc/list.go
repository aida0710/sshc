package main

import (
	"fmt"
	"io"

	"sshc/internal/app"
)

// ListSubcommand は、OpenSSH が config と Include から読み取る具体的な接続名を列挙する。
const ListSubcommand = "list"

func runList(home string, stdout, stderr io.Writer) int {
	connections, err := app.ReadConnections(home)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	for _, connection := range connections {
		if _, err := fmt.Fprintln(stdout, connection.Alias); err != nil {
			fmt.Fprintf(stderr, "sshc: write host list: %v\n", err)
			return 1
		}
	}
	return 0
}
