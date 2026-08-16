package main

import (
	"context"
	"errors"
	"io"
)

// ownership の理由は、終了コードを決めるために区別される。
var (
	// errOwnershipEnded は、持ち主が正常に手を離したことを言う。**失敗ではない。**
	errOwnershipEnded = errors.New("desktop ownership ended")
	// errOwnershipProtocol は、寿命だけを運ぶはずのチャンネルに中身が来たこと、
	// あるいは入力がそもそも寿命チャンネルではないことを言う。
	errOwnershipProtocol = errors.New("desktop ownership protocol violation")
	// errOwnershipRead は、チャンネルの監視そのものが失敗したことを言う。
	errOwnershipRead = errors.New("desktop ownership channel failed")
)

// ownershipMonitor は、デスクトップの外殻が生きているあいだだけ開いている
// 一方向のチャンネルを見張る。
//
// **PID は見ない。** プロセス表を覗く方式は、番号が使い回された瞬間に他人を
// 親だと思い込む。チャンネルが閉じたことは、OS が保証する事実である。
type ownershipMonitor interface {
	// Start は、入力が生きているチャンネルであることを同期的に確かめ、監視を
	// 張る。開始前に既に閉じていたかどうかを確かめ終えてから返る。
	Start(context.Context) (<-chan error, error)
	// Stop は監視を取り消して合流する。**呼び出し側のファイルは閉じない。**
	Stop() error
}

// newOwnershipMonitor は、OS ごとの実装を返す。テストは
// runEngineWithDependencies を通して自前の実装を注入する——任意の io.Reader が
// 本番の所有権の証拠になってはならない。
func newOwnershipMonitor(reader io.Reader) (ownershipMonitor, error) {
	return newOSOwnershipMonitor(reader)
}
