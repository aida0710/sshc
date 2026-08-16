//go:build windows

package keys

// テストのフィクスチャが使う、このファイルシステムの絶対パス。
//
// **Windows で「外」と言えるのは別のボリュームである。** 同じドライブの
// `\etc\ssh` は綴りとしては絶対ではなく、ワークスペースの下に落ちてしまう。
const (
	// testSSHDirectory は、ワークスペースの外に置いた `~/.ssh` の綴りである。
	testSSHDirectory = `C:\Users\Example\.ssh`
	// testOutsideKey は、どのワークスペースにも属さない鍵の綴りである。
	testOutsideKey = `D:\shared\ssh\shared`
)
