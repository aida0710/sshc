package main

import (
	"fmt"
	"io"

	"sshc/internal/app"
)

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
