package envelope

import (
	"os"
	"testing"
)

// このパッケージのテストは、鍵の導出そのもの（コスト、上限、直列化）を確かめるので、
// ビルドタグ sshc_test_kdf で流しても製品のコストで導出する。
func TestMain(m *testing.M) {
	DerivationCost = productionDerivationCost
	os.Exit(m.Run())
}

// 製品が新しい鍵に使うコストは、remote から受け取ってよい上限と同じである。下回れば、
// このインストールが書くすべての envelope が弱くなる。
func TestTheProductionDerivationCostIsTheRemoteCeiling(t *testing.T) {
	if productionDerivationCost != AcceptedFromRemote {
		t.Errorf("productionDerivationCost = %+v, want the remote ceiling %+v", productionDerivationCost, AcceptedFromRemote)
	}
}
