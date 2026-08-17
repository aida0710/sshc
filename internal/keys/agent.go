package keys

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"sshc/internal/platform"
)

// defaultAgentTimeout は、agent との往復ひとつに掛ける上限である。
//
// ソケットの向こうは同じマシンの別プロセスであり、答えないなら壊れている。
const defaultAgentTimeout = 10 * time.Second

// Agent は、ユーザーの ssh-agent とプロトコルで直接話す。
//
// **ssh-add は起こさない。** あれを通していたのは、agent のプロトコルを自分で
// 話す手段が無かったからである。接続の公開鍵認証が既に同じソケットから鍵を
// 読んでいるので、**同じソケットに二つの話し方を持たない。**
//
// パスフレーズは、このプロセスの中で鍵を復号してから登録する。子プロセスの
// 標準入力を通らない——**外に出る秘密が一つ減る。**
type Agent struct {
	// Socket は SSH_AUTH_SOCK を読む。テストが差し替えるためにある。
	Socket func() string
	// Dial はソケットを開く。nil なら unix ソケットとして開く。
	Dial    func(ctx context.Context, address string) (net.Conn, error)
	Timeout time.Duration
}

// NewAgent は、この環境の ssh-agent に話しかけるアダプタを返す。
//
// **どこへ繋ぐかは OS ごとに違う。** Unix は SSH_AUTH_SOCK の指す unix ソケット
// で、Windows は固定の named pipe である。その差は agent_unix.go と
// agent_windows.go が持ち、ここから先のプロトコルは同じひとつである。
func NewAgent(lookup func(string) (string, bool)) platform.KeyAgent {
	return newPlatformAgent(lookup)
}

// Available は、このプロセスが agent に到達できるかを報告する。
//
// **開けるかどうかで答える。** 変数があることは、その先に誰かがいることを
// 意味しない——死んだ端末が残した SSH_AUTH_SOCK は、いつまでも残る。
func (a Agent) Available(ctx context.Context) bool {
	conn, err := a.open(ctx)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (a Agent) List(ctx context.Context) ([]platform.AgentIdentity, error) {
	conn, err := a.open(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	loaded, err := agent.NewClient(conn).List()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", platform.ErrAgentRejected, err)
	}
	identities := make([]platform.AgentIdentity, 0, len(loaded))
	for _, key := range loaded {
		public, err := ssh.ParsePublicKey(key.Blob)
		if err != nil {
			continue
		}
		identities = append(identities, platform.AgentIdentity{
			Bits:        publicKeyBits(public),
			Fingerprint: ssh.FingerprintSHA256(public),
			Comment:     key.Comment,
			Algorithm:   public.Type(),
		})
	}
	return identities, nil
}

// Add は秘密鍵を 1 つ読み込ませる。
//
// 鍵の復号はここで行う。agent が受け取るのは復号済みの鍵であり、パスフレーズ
// そのものは agent へ渡らない——それが渡るのは ssh-add に読ませていたときの
// 都合であって、プロトコルの要求ではない。
func (a Agent) Add(ctx context.Context, request platform.AgentAddRequest) error {
	contents, err := os.ReadFile(request.PrivateKeyPath)
	if err != nil {
		return err
	}
	private, err := DecodePrivateKey(contents, request.Passphrase)
	if err != nil {
		return err
	}

	conn, err := a.open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	added := agent.AddedKey{PrivateKey: private, Comment: request.PrivateKeyPath}
	if request.LifetimeSeconds > 0 {
		added.LifetimeSecs = uint32(request.LifetimeSeconds)
	}
	if err := agent.NewClient(conn).Add(added); err != nil {
		return fmt.Errorf("%w: %s", platform.ErrAgentRejected, err)
	}
	return nil
}

// Remove は、その公開鍵ファイルが指す鍵を agent から外す。
func (a Agent) Remove(ctx context.Context, publicKeyPath string) error {
	line, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return err
	}
	public, _, _, _, err := ssh.ParseAuthorizedKey(line)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotPublicKey, err)
	}

	conn, err := a.open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if err := agent.NewClient(conn).Remove(public); err != nil {
		return fmt.Errorf("%w: %s", platform.ErrAgentRejected, err)
	}
	return nil
}

func (a Agent) open(ctx context.Context) (net.Conn, error) {
	socket := ""
	if a.Socket != nil {
		socket = a.Socket()
	}
	if socket == "" {
		return nil, platform.ErrAgentUnavailable
	}

	timeout := a.Timeout
	if timeout <= 0 {
		timeout = defaultAgentTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dial := a.Dial
	if dial == nil {
		// **既定を持たない。** ここで unix ソケットへ落とすと、Windows で
		// Dial を配線し忘れた日に「unix ソケットが開けない」という、その OS に
		// 存在しない理由が返る。宛先を知っているのは組み立てた側だけである。
		return nil, platform.ErrAgentUnavailable
	}
	conn, err := dial(ctx, socket)
	if err != nil {
		return nil, errors.Join(platform.ErrAgentUnavailable, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	return conn, nil
}
