//go:build sshc_test_kdf

package envelope

// CI と make test は、テストをビルドタグ sshc_test_kdf を付けて流す。このとき Derive は、
// Argon2id が受け付ける最小に近いコストで導出する。導出・封・開く手順は製品と同じに通る。
//
// 鍵の導出の強さを確かめるのはこのパッケージのテストだけで（TestMain が製品のコストに
// 戻す）、ほかのパッケージのテストは Vault や同期の手順を確かめるために鍵を導出する。
// 製品のコスト（64 MiB を 3 回）のままだと、race detector の下、とくに Argon2id の
// アセンブリが無い arm64（macOS の runner）では 1 回の導出に数秒かかり、テストの時間の
// 大半を占めていた（2026-10-27 の macOS の race で、httpserver が 435 秒から 21 秒に、
// secret が 530 秒から 20 秒未満になった）。
//
// 製品のバイナリをこのタグでビルドしてはならない。リリースの経路がこのタグを使わない
// ことは internal/buildcontract が確かめる。
func init() {
	DerivationCost = Limits{Time: 1, MemoryKiB: 64, Threads: 1}
}
