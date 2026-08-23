//go:build unix

package keys

// テストのフィクスチャが使う、このファイルシステムの絶対パス。
const (
	// testSSHDirectory は、ワークスペースの外に置いた `~/.ssh` の表記である。
	testSSHDirectory = "/Users/example/.ssh"
	// testOutsideKey は、どのワークスペースにも属さない鍵の表記である。
	testOutsideKey = "/etc/ssh/shared"
)
