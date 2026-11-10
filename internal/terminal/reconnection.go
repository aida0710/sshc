package terminal

import "context"

// reconnection は、落ちた接続を自動でつなぎ直す処理の進み具合である。処理そのものは reconnect.go と
// reconnect_network.go にある。
type reconnection struct {
	// attempts は、今の切断から試した自動再接続の回数である。ReconnectSettled のあいだ安定してつながったら
	// 0 に戻す。
	attempts int
	// view は、次の試みを待っているあいだ一覧に出すものである。待っていなければ nil。
	view *ReconnectView
	// cancel は、開き直している最中の接続を取り消す。開き直していなければ nil。
	cancel context.CancelFunc
	// transportCheckPending は、回線が変わったあとの確認に輸送がまだ応答していない間だけ真である。
	// この間に輸送が落ちたら、回線の変化で切れたものとして扱う。
	transportCheckPending bool
	// noNetwork は、直前の接続の試みが回線の無さで失敗したことを表す。
	noNetwork bool
	// networkWaitAnnounced は、回線を待っていることをターミナルに書いたことを表す。
	networkWaitAnnounced bool
}
