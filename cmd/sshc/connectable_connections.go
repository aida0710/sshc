package main

import (
	"fmt"
	"io"

	"sshc/internal/app"
	"sshc/internal/validate"
)

// readConnectableConnections は、ssh_config の接続のうち、sshc が接続先として
// 受け付ける alias のものだけを返す。
//
// OpenSSH は `Host $(id)` のような alias も読む。`sshc list` の一覧は shell 補完の
// 候補になり、`sshc ssh` の選択画面で選んだ alias は runConnect へ渡る。どちらも
// validate.Alias の外にある alias は起動も評価もしないと決めているので、ここで落とす。
// 黙って消すと接続先が消えたように見えるので、落とした alias は stderr に理由付きで出す。
func readConnectableConnections(home string, stderr io.Writer) ([]app.Connection, error) {
	connections, err := app.ReadConnections(home)
	if err != nil {
		return nil, err
	}
	connectable := make([]app.Connection, 0, len(connections))
	for _, connection := range connections {
		if err := validate.Alias(connection.Alias); err != nil {
			// alias はターミナルの制御文字を含みうる。そのまま書くと表示を細工されるため引用する。
			fmt.Fprintf(stderr, "sshc: skipping alias %q: %v\n", connection.Alias, err)
			continue
		}
		connectable = append(connectable, connection)
	}
	return connectable, nil
}
