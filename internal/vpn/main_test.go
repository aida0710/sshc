package vpn

import (
	"os"
	"testing"
)

// TestMain は、検査の実行ファイルが偽のプログラムとして起動されたときは、その役だけを
// して終わる。偽のプログラムは、起動するときに付けた環境変数で見分ける。
func TestMain(m *testing.M) {
	if encoded, found := os.LookupEnv(fakeDockerReplyVariable); found {
		os.Exit(writeFakeDockerReply(encoded))
	}
	if _, found := os.LookupEnv(fakeOpenConnectVariable); found {
		os.Exit(runFakeOpenConnect())
	}
	os.Exit(m.Run())
}
