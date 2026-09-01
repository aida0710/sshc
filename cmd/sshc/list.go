package main

import (
	"fmt"
	"io"

	"sshc/internal/app"
	"sshc/internal/validate"
)

// この一覧は shell 補完の候補にもなる。OpenSSH は `Host $(id)` のような alias も
// 読むため、validate.Alias の外にある alias はここで落とす。起動も評価もされない
// と決めた値を shell へ渡さないための多層防御である。黙って消すと接続先が消えた
// ように見えるので、落とした alias は stderr に理由付きで出す。
func runList(home string, stdout, stderr io.Writer) int {
	connections, err := app.ReadConnections(home)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	for _, connection := range connections {
		if err := validate.Alias(connection.Alias); err != nil {
			// alias は端末制御文字を含みうる。そのまま書くと表示を細工されるため引用する。
			fmt.Fprintf(stderr, "sshc: skipping alias %q: %v\n", connection.Alias, err)
			continue
		}
		if _, err := fmt.Fprintln(stdout, connection.Alias); err != nil {
			fmt.Fprintf(stderr, "sshc: write host list: %v\n", err)
			return 1
		}
	}
	return 0
}
