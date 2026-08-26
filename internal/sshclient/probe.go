package sshclient

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Probe は、認証だけを試した結果である。
type Probe struct {
	// Method は通った認証方式。空なら、どれも通らなかった。
	//
	// どの鍵で通ったかは言わない。公開鍵認証では、鍵の一覧を渡したあと
	// どれを使うかを決めるのは相手であり、こちらはその判断を見ていない。
	Method string
	// Tried は、こちらから試した方式を試した順に並べたもの。
	Tried []string
	// Banner はサーバーが送った文言。空でありうる。
	Banner string
	// Elapsed は接続を始めてから結果が出るまで。
	Elapsed time.Duration
}

// Probe は、接続して認証だけを試し、チャンネルを開かずに閉じる。
//
// 保存済み資格情報は対話接続と同じ認証経路で使うが、追加質問は拒否する。
// パスフレーズもパスワードも端末には出ず、入力も待たない。これは「上限つきで
// 非対話」というこの検査の約束をそのまま引き継いでいる。
func (d Dialer) Probe(ctx context.Context, target Target) (Probe, error) {
	started := time.Now()

	// 未知のホストを暗黙に受け入れない。StrictHostKeyChecking=yes 相当である。
	// 検査のために信頼を増やしてはならない。
	strict := target
	strict.Strict = "yes"

	recorder := &methodRecorder{}
	auth := d.Auth
	auth.Observe = recorder.note
	methods, closeAuth := auth.methodsWithCleanup(strict, noPrompt)
	defer closeAuth()
	if len(methods) == 0 {
		return Probe{Elapsed: time.Since(started)}, ErrNoAuthMethod
	}

	client, closers, err := d.probeChain(ctx, strict, methods, recorder)
	if err != nil {
		return Probe{Tried: recorder.tried(), Banner: recorder.banner(), Elapsed: time.Since(started)}, err
	}
	_ = client.Close()
	for index := len(closers) - 1; index >= 0; index-- {
		_ = closers[index].Close()
	}

	return Probe{
		Method:  recorder.last(),
		Tried:   recorder.tried(),
		Banner:  recorder.banner(),
		Elapsed: time.Since(started),
	}, nil
}

// probeChain は、ProxyJump の手前側を普通に繋いでから、最後の一段だけを
// 記録付きで繋ぐ。
//
// 手前のホップも認証を要求するが、応答したい問いは「最後のホストに認証できるか」
// である。手前で止まったなら、それはそのまま失敗として返る。
func (d Dialer) probeChain(
	ctx context.Context, target Target, auth []ssh.AuthMethod, recorder *methodRecorder,
) (*ssh.Client, []ssh.Conn, error) {
	var through *ssh.Client
	var opened []ssh.Conn
	for _, hop := range target.Jump {
		client, err := d.connectOne(ctx, hop, through, noPrompt, nil)
		if err != nil {
			for _, conn := range opened {
				_ = conn.Close()
			}
			return nil, nil, err
		}
		opened = append(opened, client)
		through = client
	}

	timeout := target.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := d.open(ctx, target, through, nil)
	if err != nil {
		for _, existing := range opened {
			_ = existing.Close()
		}
		return nil, nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	connection, channels, requests, err := ssh.NewClientConn(conn, target.Address(), &ssh.ClientConfig{
		User:            target.User,
		Auth:            auth,
		HostKeyCallback: d.HostKeys.Callback(target, nil),
		// 認証テストは、実接続と同じ鍵の種類を名乗る。ここだけ既定の順序に
		// 任せていたので、三種類の鍵を持つホストが known_hosts にある 1 行とは
		// 違う種類を出し、実際には繋がるホストを認証テストが host_key_changed と
		// 報告しうた。検査が本番と違う条件で繋ぐなら、それは検査ではない。
		HostKeyAlgorithms: d.HostKeys.Algorithms(target),
		BannerCallback:    recorder.noteBanner,
		Timeout:           timeout,
	})
	if err != nil {
		_ = conn.Close()
		for _, existing := range opened {
			_ = existing.Close()
		}
		return nil, nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(connection, channels, requests), opened, nil
}

// methodRecorder は、どの方式がいつ試されたかを見る。
//
// x/crypto は「どれで通ったか」を返さないが、方式は順に試され、通った時点で
// 握手が終わる。だから最後に呼ばれた方式が通った方式である。推測ではない。
type methodRecorder struct {
	mutex    sync.Mutex
	attempts []string
	message  string
}

func (r *methodRecorder) note(name string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.attempts = append(r.attempts, name)
}

func (r *methodRecorder) noteBanner(message string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.message = strings.TrimSpace(message)
	return nil
}

func (r *methodRecorder) tried() []string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]string(nil), r.attempts...)
}

func (r *methodRecorder) last() string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if len(r.attempts) == 0 {
		return ""
	}
	return r.attempts[len(r.attempts)-1]
}

func (r *methodRecorder) banner() string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.message
}

// ErrNoAuthMethod は、試せる認証方式がひとつも無いことを報告する。
//
// 鍵も agent も無く、尋ねる先も無い接続である。繋がらなかったのではなく、
// 差し出すものが何も無かったので、失敗の理由としては別物である。
var ErrNoAuthMethod = errors.New("no authentication method is available for this connection")
