package remotesync_test

import (
	"os"
	"testing"

	"sshc/internal/envelope"
)

// cheapDerivationCost は、Argon2id の最小（threads あたり 8 KiB）に近いコスト。
// race detector の下でも 1 回の導出が 1 ms 前後で終わる。
var cheapDerivationCost = envelope.Limits{Time: 1, MemoryKiB: 64, Threads: 1}

// TestMain は、このパッケージのテストが新しい鍵に使う Argon2id のコストを下げる。
//
// ここで確かめるのは同期の手順であり、鍵導出の強さではない。強さは internal/envelope
// のテストが製品のコストで確かめる。製品のコスト（64 MiB を 3 回）のままでは、
// macOS の race detector の下で、このパッケージだけで 440〜654 秒かかる。
func TestMain(m *testing.M) {
	envelope.DerivationCost = cheapDerivationCost
	os.Exit(m.Run())
}
