package sshclient

import (
	"context"
	"errors"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// ScanAlgorithms は、ホスト鍵を尋ねる種別の並びである。
//
// `ssh-keyscan` の既定と同じ顔ぶれにしてある。サーバーが持っていない種別は
// 暗黙に飛ばす。持っていないことは失敗ではない。
var ScanAlgorithms = []string{
	ssh.KeyAlgoED25519,
	ssh.KeyAlgoECDSA256,
	ssh.KeyAlgoECDSA384,
	ssh.KeyAlgoECDSA521,
	ssh.KeyAlgoRSASHA512,
	ssh.KeyAlgoRSASHA256,
	ssh.KeyAlgoRSA,
}

// DefaultScanTimeout は、ひとつのアドレスを尋ねるのに掛ける上限である。
//
// 種別ごとではなく全体の予算である。種別ごとにすると、届かないアドレスに
// 対して上限が種別の数だけ掛かる。7 倍待たされる。
const DefaultScanTimeout = 15 * time.Second

// errKeyCollected は、鍵を受け取ったので握手を止めるという合図である。
//
// 失敗ではない。鍵を集めるのに資格情報は要らないので、鍵が手に入った時点で
// 用は済んでいる。そこから認証へ進むと、集めるだけのはずの操作が資格情報を
// 差し出すことになる。
var errKeyCollected = errors.New("sshclient: the host key was collected")

// ScanHostKeys は、そのアドレスが提示するホスト鍵を集める。
//
// 認証しない。種別ごとに握手を始め、鍵を受け取ったところで断る。これは
// ssh-keyscan がしていることと同じである。
func ScanHostKeys(
	ctx context.Context,
	dial func(ctx context.Context, network, address string) (net.Conn, error),
	address string,
	timeout time.Duration,
) ([]ssh.PublicKey, error) {
	if timeout <= 0 {
		timeout = DefaultScanTimeout
	}
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var collected []ssh.PublicKey
	seen := map[string]bool{}
	var lastErr error
	for _, algorithm := range ScanAlgorithms {
		key, reached, err := scanOne(ctx, dial, address, algorithm)
		if err != nil {
			lastErr = err
			if !reached {
				// 繋がらなかった。種別の問題ではない。残りを試しても
				// 同じ理由で同じだけ待つだけである。
				break
			}
			continue
		}
		// 同じ鍵を二度並べない。RSA は三つの署名アルゴリズムで同じ鍵を出す。
		fingerprint := ssh.FingerprintSHA256(key)
		if seen[fingerprint] {
			continue
		}
		seen[fingerprint] = true
		collected = append(collected, key)
	}
	if len(collected) == 0 && lastErr != nil {
		// 一つも取れなかったときだけ理由を返す。取れた種別があるなら、
		// 取れなかった種別はサーバーが持っていないだけである。
		return nil, lastErr
	}
	return collected, nil
}

// scanOne は、ひとつの種別について鍵を尋ねる。
//
// reached は、そのアドレスに繋がったかを報告する。繋がらなかったことと、
// その種別を持っていなかったことは別の結果である。
func scanOne(
	ctx context.Context,
	dial func(ctx context.Context, network, address string) (net.Conn, error),
	address, algorithm string,
) (key ssh.PublicKey, reached bool, err error) {
	conn, err := dial(ctx, "tcp", address)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	var found ssh.PublicKey
	_, _, _, err = ssh.NewClientConn(conn, address, &ssh.ClientConfig{
		// 認証方式をひとつも渡さない。ここまで進む前に断つので使われないが、
		// 渡さないこと自体が「資格情報を持ち込まない」という宣言である。
		User:              "sshc-keyscan",
		HostKeyAlgorithms: []string{algorithm},
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			found = key
			return errKeyCollected
		},
	})
	if found != nil {
		return found, true, nil
	}
	if err == nil {
		return nil, true, errors.New("sshclient: the handshake completed without a host key")
	}
	return nil, true, err
}
