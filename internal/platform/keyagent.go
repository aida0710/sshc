package platform

import (
	"context"
	"errors"
)

var (
	ErrAgentUnavailable = errors.New("no ssh-agent is reachable from this process")
	ErrAgentRejected    = errors.New("the ssh-agent rejected the request")
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
// 鍵はファイルのパスではなく中身で渡す。ファイルを読むのは呼び手（keys.Service）で、
// ほかの鍵の読み込みと同じくシンボリックリンクをたどらず、大きさに上限を設けて読む。
// KeyAgent はファイルに触れず、agent との通信だけを受け持つ。
//
// Passphrase は、このプロセスの中で鍵を復号するためだけに使い、agent へは渡さない。
// agent が受け取るのは復号済みの鍵である。
type AgentAddRequest struct {
	// PrivateKey は秘密鍵ファイルの中身である。
	PrivateKey []byte
	// Comment は、agent の一覧で鍵に付く名前である。
	Comment         string
	Passphrase      []byte
	LifetimeSeconds int
}

// KeyAgent は、ユーザーの ssh-agent に秘密鍵を登録する。自動テストは常に偽物で
// 差し替える。このリポジトリのどのテストも本物のエージェントとは話さない。
type KeyAgent interface {
	Available(ctx context.Context) bool
	List(ctx context.Context) ([]AgentIdentity, error)
	Add(ctx context.Context, request AgentAddRequest) error
	// Remove は、publicKey（公開鍵ファイルの中身。authorized_keys と同じ 1 行の形式）
	// が指す鍵を agent から外す。
	Remove(ctx context.Context, publicKey []byte) error
}
