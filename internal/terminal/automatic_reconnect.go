package terminal

import "context"

// 自動再接続の試みで開く接続に印を付ける。
//
// 接続を開く側は、これを見て、利用者が止めたものを自動再接続では起動し直さない
// ようにできる（VPN画面の「切断」で止めた経路など）。［再接続］の操作と、新しく
// 開いた接続には印を付けない。どちらも利用者がいま接続を求めている。

type automaticReconnectKey struct{}

// WithAutomaticReconnect は、自動再接続の試みであることを載せた ctx を返す。
func WithAutomaticReconnect(ctx context.Context) context.Context {
	return context.WithValue(ctx, automaticReconnectKey{}, true)
}

// IsAutomaticReconnect は、ctx が自動再接続の試みのものかを返す。
func IsAutomaticReconnect(ctx context.Context) bool {
	automatic, _ := ctx.Value(automaticReconnectKey{}).(bool)
	return automatic
}

type waitingForNetworkKey struct{}

// WithWaitingForNetwork は、回線が戻ったかを確かめる試みであることを載せた ctx を返す。回線が
// 無いままなら、その試みは何も書かずに失敗する。待つ間に試すたびに同じ理由がターミナルに並ばないようにする。
func WithWaitingForNetwork(ctx context.Context) context.Context {
	return context.WithValue(ctx, waitingForNetworkKey{}, true)
}

// IsWaitingForNetwork は、ctx が回線が戻ったかを確かめる試みのものかを返す。
func IsWaitingForNetwork(ctx context.Context) bool {
	waiting, _ := ctx.Value(waitingForNetworkKey{}).(bool)
	return waiting
}
