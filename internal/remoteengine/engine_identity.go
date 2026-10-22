package remoteengine

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sshc/internal/handoff"
)

// handoffCommand は、接続先で、そのエンジンの handoff（待ち受けのポートと CLI の秘密）を
// 読む固定のコマンドである。接続先の sshc CLI が読むのと同じファイルを、同じ利用者として
// 読む。無ければ（エンジンが動いていなければ）0 以外で終わる。
const handoffCommand = `sh -c 'cat "$HOME/.ssh/sshc/cli"'`

// handoffLimit は、handoffCommand が終わるまでの上限である。小さいファイルを読むだけで、
// SSH の往復を含めても数秒で終わる。
const handoffLimit = 10 * time.Second

// proofLimit は、開いた TCP の接続で、相手が handoff の秘密を持つことを確かめ終えるまでの
// 上限である。本物のエンジンは乱数に HMAC を返すだけなので、SSH の往復を含めても数秒で
// 答える。答えない相手に、取り次ぎをいつまでも止めさせない。
const proofLimit = 10 * time.Second

// engineIdentity は、接続先のエンジンの待ち受けのポートと、そのエンジンだけが持つ
// handoff の秘密である。接続先の利用者だけが読める handoff から読む。
type engineIdentity struct {
	port    int
	secret  string
	version string
}

// readIdentity は、接続先で handoff を読む。
func readIdentity(ctx context.Context, link *hostLink) (engineIdentity, error) {
	live, err := link.live(ctx)
	if err != nil {
		return engineIdentity{}, err
	}
	result, err := live.Run(ctx, handoffCommand, handoffLimit)
	if err != nil {
		return engineIdentity{}, err
	}
	if result.ExitCode != 0 {
		return engineIdentity{}, ErrNotRunning
	}
	document, err := handoff.Parse(result.Stdout)
	if err != nil {
		return engineIdentity{}, fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	port, err := strconv.Atoi(strings.TrimPrefix(document.URL, "http://"+handoff.EngineHost+":"))
	if err != nil {
		return engineIdentity{}, fmt.Errorf("%w: %v", ErrNotRunning, err)
	}
	return engineIdentity{port: port, secret: document.Secret, version: document.Version}, nil
}

// dialFailure は、接続先のエンジンへの TCP を開けなかった、または開いた相手が身元を
// 示せなかったことを表す。要求はまだ何も送っていないので、handoff を読み直して
// 送り直してよい。
type dialFailure struct{ err error }

func (e *dialFailure) Error() string {
	return "open a connection to the remote engine: " + e.err.Error()
}
func (e *dialFailure) Unwrap() error { return e.err }

// isDialFailure は、err が、要求を送る前の接続の失敗かを返す。
func isDialFailure(err error) bool {
	var failure *dialFailure
	return errors.As(err, &failure)
}

// dialProven は、接続先の 127.0.0.1:<identity.port> へ TCP を開き、相手が handoff の
// 秘密を持つことを確かめてから返す。
//
// 接続先のエンジンが起動し直すと、別のポートへ移る。空いた古いポートは、接続先の別の
// 利用者でも取れる。最初の 1 回だけ確かめて資格を持ち続けると、cookie や Vault の
// パスワードをそのプロセスへ渡してしまう。そこで、新しく開く TCP の接続ごとに、資格を
// 送る前に確かめる。確かめた接続は同じ相手のままなので、使い回してよい。
func (h *hostLink) dialProven(ctx context.Context, identity engineIdentity) (net.Conn, error) {
	connection, err := h.dialLoopback(ctx, identity.port)
	if err != nil {
		return nil, &dialFailure{err: err}
	}
	if err := proveConnection(ctx, connection, identity); err != nil {
		_ = connection.Close()
		return nil, &dialFailure{err: err}
	}
	return connection, nil
}

// proveConnection は、開いたばかりの connection に /cli/challenge の要求を書き、handoff の
// 秘密で署名した答えが返ることを確かめる。手順はこのマシンの sshc CLI と同じである。
// 確かめに失敗したら ErrUnproven を包んで返す。
func proveConnection(ctx context.Context, connection net.Conn, identity engineIdentity) error {
	ctx, cancel := context.WithTimeout(ctx, proofLimit)
	defer cancel()
	// SSH のチャネルの net.Conn は読み書きの期限を持てない。期限が来たら閉じて、止まった
	// 読み取りを終わらせる。
	stopWatching := context.AfterFunc(ctx, func() { _ = connection.Close() })

	challenge, err := handoff.MintChallenge(rand.Reader)
	if err != nil {
		stopWatching()
		return err
	}
	request, err := http.NewRequest(http.MethodGet, "http://"+loopbackHost(identity.port)+challengePath, nil)
	if err != nil {
		stopWatching()
		return err
	}
	request.Header.Set(handoff.ChallengeHeader, challenge)
	answered := askChallenge(connection, request)
	if !stopWatching() {
		return fmt.Errorf("%w: the challenge was not answered: %v", ErrUnproven, ctx.Err())
	}
	if answered.err != nil {
		return fmt.Errorf("%w: %v", ErrUnproven, answered.err)
	}
	// 本物のエンジンは、204 のあと次の要求まで何も書かない。続けて届いたものがあれば、
	// この接続の上の次の答えと食い違うので使わない。
	if answered.status != http.StatusNoContent || answered.trailing ||
		!handoff.VerifyProof(identity.secret, challenge, answered.proof) {
		return ErrUnproven
	}
	return nil
}

// challengeAnswer は、/cli/challenge への答えのうち、確かめに使うものである。
type challengeAnswer struct {
	status int
	proof  string
	// trailing は、答えのあとに、まだ読んでいないバイトが届いていたかである。
	trailing bool
	err      error
}

// askChallenge は、request を connection に書き、答えを 1 つ読む。
func askChallenge(connection net.Conn, request *http.Request) challengeAnswer {
	if err := request.Write(connection); err != nil {
		return challengeAnswer{err: err}
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return challengeAnswer{err: err}
	}
	// 本文は読まない。本物の答え（204）には本文が無く、それ以外の答えなら呼び出し側が
	// 接続ごと閉じる。閉じる前に読み切ると、本文を流し続ける相手に止められる。
	return challengeAnswer{
		status:   response.StatusCode,
		proof:    response.Header.Get(handoff.ProofHeader),
		trailing: reader.Buffered() > 0,
	}
}
