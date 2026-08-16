//go:build unix

package keys

// テストのフィクスチャが使う、このファイルシステムの絶対パス。
const (
	// testSSHDirectory は、ワークスペースの外に置いた `~/.ssh` の綴りである。
	testSSHDirectory = "/Users/example/.ssh"
	// testOutsideKey は、どのワークスペースにも属さない鍵の綴りである。
	testOutsideKey = "/etc/ssh/shared"
)
