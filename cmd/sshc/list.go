package main

import (
	"fmt"
	"io"
)

// runList は、接続できる alias を 1 行に 1 つずつ出す。この一覧は shell 補完の
// 候補にもなる。readConnectableConnections で落とすのは、起動も評価もしないと
// 決めた値を shell へ渡さないための多層防御でもある。
func runList(home string, stdout, stderr io.Writer) int {
	connections, err := readConnectableConnections(home, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return exitFailure
	}
	for _, connection := range connections {
		if _, err := fmt.Fprintln(stdout, connection.Alias); err != nil {
			fmt.Fprintf(stderr, "sshc: write host list: %v\n", err)
			return exitFailure
		}
	}
	return 0
}
