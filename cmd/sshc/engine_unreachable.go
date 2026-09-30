package main

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// errEngineNotResponding は、handoff の port で接続は受け付けたが、上限までに
// 応答が無かったことを表す。止めた（Ctrl+Z）か固まった engine であることが多い。
// ただ、たいていは証明の段で上限を超えるので、相手が sshc だとはまだ言えない。
// 「動いていない」と案内すると、利用者が打つ `sshc engine` はロックに阻まれ、
// 原因に辿り着けない。fg は Unix のターミナルで止めた場合だけ、`sshc service install`
// は service（systemd・launchd）で動かしている場合だけに当てはまるので、条件を付けて
// 案内する。
var errEngineNotResponding = errors.New("the process at the engine address did not respond; " +
	"if you suspended sshc engine with Ctrl+Z, resume it with fg; " +
	"if the sshc service runs it, restart it with sshc service install")

// explainEngineUnreachable は、engine に届かなかった err を、利用者が次にする
// ことで分けた誤りに直す。
//
//   - 取り消し（Ctrl-C）は errInterrupted にする。
//   - 上限までに応答が無ければ errEngineNotResponding にする。
//   - handoff が無い、または port で誰も受け付けていなければ engineNotRunning にする。
//   - 版の不一致、証明の失敗、identity の不一致、不正な応答は、それぞれの案内のまま返す。
func explainEngineUnreachable(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		return errInterrupted
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		return errEngineNotResponding
	case isEngineNotRunning(err):
		return err
	case isTransportProblem(err):
		return engineNotRunning{cause: err}
	default:
		return err
	}
}

func isEngineNotRunning(err error) bool {
	var notRunning engineNotRunning
	return errors.As(err, &notRunning)
}

func isTransportProblem(err error) bool {
	var problem engineProblem
	return errors.As(err, &problem) && problem.Code == transportErrorCode
}

// reportEngineUnreachable は、engine に届かなかった err を explainEngineUnreachable で
// 分けて、終了コードを返す。取り消しは何も出さずに 130、それ以外は理由を標準エラー
// 出力へ出して 1 を返す。
func reportEngineUnreachable(ctx context.Context, err error, stderr io.Writer) int {
	explained := explainEngineUnreachable(ctx, err)
	if errors.Is(explained, errInterrupted) {
		return exitInterrupted
	}
	fmt.Fprintf(stderr, "sshc: %v\n", explained)
	return exitFailure
}
