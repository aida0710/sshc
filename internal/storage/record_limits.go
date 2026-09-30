package storage

import "errors"

const (
	// maxTransactionEntries は、1 トランザクションが持てるエントリ数の上限。いちばん
	// 大きいトランザクションは、マスターパスワード変更と、未対応の Vault の復旧・リセット
	// である。どれも世代バックアップを暗号化し直すので、バックアップの数だけエントリを
	// 持つ。バックアップは保持の上限（RetainedBackupFiles）までしか残らないので、
	// ふだんは届かない。届くのは、保持の上限を超えても残す記録（いちばん新しい記録など。
	// pruneHistory）が、それだけでこの上限に近い数のバックアップを持つときである。
	// 届いたときは何も書かずに断る。
	//
	// 所要時間はエントリ数に比例する（1 件ごとに数回の fsync を伴う）。8192 件の実測は
	// tmpfs で約 2 秒、SATA SSD の ext4 で約 7 分で、その間 workspace のロックを持ち
	// 続ける。
	maxTransactionEntries = 8192
	// maxRecordBytesPerEntry は、journal の記録で 1 エントリに見込む大きさ。一時ファイル
	// とバックアップのパスを含めた実測は 650 バイト前後で、深いパスにも余裕を持たせる。
	maxRecordBytesPerEntry = 4 << 10
	// maxRecordBytes は、journal と history の記録を読み書きする上限。書く側と読む側が
	// 同じ値を使い、「書けたが読めない」記録を作らない。
	maxRecordBytes = maxTransactionEntries * maxRecordBytesPerEntry
)

// ErrTransactionTooLarge は、journal に記録しきれないトランザクションを断る。エントリ数の
// 超過は何も書く前に、記録の大きさの超過は記録を書く時点で断る。
var ErrTransactionTooLarge = errors.New("transaction is too large for its journal record")
