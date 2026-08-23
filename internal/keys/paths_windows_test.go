//go:build windows

package keys

// テストのフィクスチャが使う、このファイルシステムの絶対パス。
//
// Windows で「外」と言えるのは別のボリュームである。同じドライブの
// `\etc\ssh` は表記としては絶対ではなく、ワークスペースの下に落ちてしまう。
const (
	// testSSHDirectory は、ワークスペースの外に置いた `~/.ssh` の表記である。
	testSSHDirectory = `C:\Users\Example\.ssh`
	// testOutsideKey は、どのワークスペースにも属さない鍵の表記である。
	testOutsideKey = `D:\shared\ssh\shared`
)
