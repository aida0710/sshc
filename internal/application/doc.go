// Package application は、~/.ssh の設定を読み書きするユースケースを保持する。
//
// 設定（config と Include 先、metadata.json、鍵ファイルのパス）を変えるユースケースは、
// Vault の変更を伴うものもここに置く。alias の改名、接続の作成・更新、鍵のパスが
// 変わる操作（鍵の改名・移動、グループの改名・削除）は、設定と Vault を 1 つの
// storage トランザクションで確定する。Vault は SetVault で受ける。HTTP と CLI の
// handler は、入力の検証、ここの関数の 1 回の呼び出し、応答への変換だけを行う。
//
// アプリケーション全体のユースケース層ではない。次のものはそれぞれのパッケージが持つ。
//   - Vault だけの状態遷移（解錠、ロック、マスターパスワードの変更、中断した
//     再封印の復旧）: secret.Service。判定と遷移を同じ mutationMu の中で行う。
//   - VPN プロファイルの保存: vpnprofile。設定と Vault を同じ形で 1 回に書く。
//   - 同期（オブジェクトストアとの取り込み・書き出し）: remotesync。
package application
