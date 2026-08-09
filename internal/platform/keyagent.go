package platform

import (
	"context"
	"errors"
)

var (
	ErrAgentUnavailable = errors.New("no ssh-agent is reachable from this process")
	ErrAgentRejected    = errors.New("ssh-add rejected the request")
)

// AgentIdentity は、ユーザーの ssh-agent に現在読み込まれている鍵ひとつ。
type AgentIdentity struct {
	Bits        int
	Fingerprint string
	Comment     string
	Algorithm   string
}

// AgentAddRequest はエージェントに秘密鍵を 1 つ読み込ませる。
//
// Passphrase は子プロセスの標準入力を通る。引数になることも環境変数になることも
// 決してない。どちらも、同じユーザーで動くどのプロセスからも読めるもの
// だからである。
type AgentAddRequest struct {
	PrivateKeyPath  string
	Passphrase      []byte
	LifetimeSeconds int
}

// KeyAgent は、ユーザーの ssh-agent に秘密鍵を登録する。自動テストは常に偽物で
// 差し替える。このリポジトリのどのテストも本物のエージェントとは話さない。
type KeyAgent interface {
	Available(ctx context.Context) bool
	List(ctx context.Context) ([]AgentIdentity, error)
	Add(ctx context.Context, request AgentAddRequest) error
	Remove(ctx context.Context, publicKeyPath string) error
}
