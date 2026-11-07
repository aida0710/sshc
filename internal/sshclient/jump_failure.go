package sshclient

import "fmt"

// JumpHostError は、ProxyJump の経路の途中のホップ（踏み台）で接続が失敗したことを表す。
// 失敗の理由だけを出すと、利用者は最後の接続先の問題だと読み、そこを確かめに行く。どの踏み台の
// 何段目で止まったかを、理由の前に添えるために包む。理由の種類（ClassifyConnectFailure、errors.Is）は
// Unwrap で元の失敗のまま読める。
type JumpHostError struct {
	// Name は踏み台の接続名で、接続名の無い踏み台（user@host の形）は宛先である。
	Name string
	// Hop は経路の中で何番目か（1 始まり）、Hops は最後の接続先を含めた経路のホップ数である。
	Hop, Hops int
	Err       error
}

func (failure *JumpHostError) Error() string {
	return fmt.Sprintf("jump host %s (%d/%d): %v", failure.Name, failure.Hop, failure.Hops, failure.Err)
}

func (failure *JumpHostError) Unwrap() error { return failure.Err }

// sentence は、踏み台の名前と段を、元の失敗の文の前に添える。
func (failure *JumpHostError) sentence() string {
	return fmt.Sprintf("踏み台の%s（%d/%d）に接続できませんでした。", failure.Name, failure.Hop, failure.Hops) + FailureSentence(failure.Err)
}
