// Package diagnostics は、設計上、個別に起動される各種チェックを実行する。
// 設定のチェック、直接 TCP による到達性のチェック、そして SSH 認証テストである。
// いずれもユーザーが意図して開始する独立した操作であり、画面を開いた副作用として
// 走るものはここに何もない。
package diagnostics

import (
	"context"
	"errors"
	"net"
	"os"
	"time"
)

// DirectDialNotice は、ダイヤルした到達性の結果に付随する。このチェックは接続先へ
// 自分でダイヤルするので、踏み台ホストや VPN 経由でしか到達できないホストはここで
// 失敗するのが当然である。UI は、そのホストが落ちていると受け取られないよう、その旨を
// 述べなければならない。
const DirectDialNotice = "This check dialled the destination directly. ProxyJump, ProxyCommand, VPN profiles and any jump-host firewall were not used."

// VPNRouteNotice は、VPN プロファイルを付けた接続の結果に付随する。このマシンから
// 直接ダイヤルしても VPN の経路を通らないので、結果は実際の接続と関係が無い。VPN の
// 経路を起動すると、ボタン1回でコンテナの起動やスマートフォンでの承認が走るので、
// それもしない。
const VPNRouteNotice = "This connection goes through a VPN profile, so the check did not dial the destination: a direct dial from this machine does not use the VPN route. The authentication test connects through the VPN."

// DefaultReachabilityTimeout は、TCP ダイヤル一回に上限を設ける。
const DefaultReachabilityTimeout = 5 * time.Second

// 到達性の結果。
const (
	ReachabilityReached    = "reached"
	ReachabilityRefused    = "refused"
	ReachabilityTimeout    = "timeout"
	ReachabilityDNSFailure = "dns_failure"
	ReachabilityFailed     = "failed"
	// ReachabilityNotChecked は、ダイヤルしなかったことを表す。理由は Notice が述べる。
	ReachabilityNotChecked = "not_checked"
)

// Dialer は TCP 接続を開く。*net.Dialer がこれを満たす。テストは関数で差し替え、
// 自動テストがリモートのソケットを開くことがないようにする。
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// ReachabilityResult は、完了したダイヤル試行ひとつ分。
type ReachabilityResult struct {
	Address string
	Outcome string
	Elapsed time.Duration
	Detail  string
	Notice  string
}

// Reachability は接続先へ直接ダイヤルし、意図的に ProxyJump と VPN を無視する。
type Reachability struct {
	Dialer  Dialer
	Timeout time.Duration
}

// Check は hostname:port へ一度ダイヤルし、その結果を分類する。
func (r Reachability) Check(ctx context.Context, hostname, port string) ReachabilityResult {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultReachabilityTimeout
	}
	dialContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	address := net.JoinHostPort(hostname, port)
	result := ReachabilityResult{Address: address, Notice: DirectDialNotice}

	started := time.Now()
	connection, err := r.Dialer.DialContext(dialContext, "tcp", address)
	result.Elapsed = time.Since(started)
	if err == nil {
		connection.Close()
		result.Outcome = ReachabilityReached
		return result
	}

	result.Detail = err.Error()
	var dnsError *net.DNSError
	switch {
	case errors.As(err, &dnsError):
		result.Outcome = ReachabilityDNSFailure
	case isConnectionRefused(err):
		result.Outcome = ReachabilityRefused
	case errors.Is(err, os.ErrDeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		result.Outcome = ReachabilityTimeout
	default:
		result.Outcome = ReachabilityFailed
	}
	return result
}

func isConnectionRefused(err error) bool {
	for _, refused := range connectionRefused {
		if errors.Is(err, refused) {
			return true
		}
	}
	return false
}
