package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// 走っている engine を止めてから起動する。
//
// どこで起動したか分からない engine を、止められる必要がある。tmux の中か、
// 閉じた端末か、supervisor の下か。探して回るより、走っているものに頼む方が短い。
//
// 暗黙に止めない。止めれば開いている端末も転送も落ち、保管庫は施錠される。
// 端末なら訊き、端末でなければ `--replace` を書いたユーザーにだけ従う。

// engineReleaseTimeout は、頼んだ engine が席を空けるのを待つ上限である。
//
// engine 自身が engine lock を手放すまでの上限（VPN の経路を畳む時間を含む）から
// 導く。あちらが畳むのに使ってよい時間より短く待てば、間に合ったはずのものを
// 諦めることになる。
const engineReleaseTimeout = app.MaxShutdownDuration + engineReleaseMargin

// engineReleaseMargin は、engine の締切の外で掛かる時間への余裕である。頼みが
// 届いてから畳み始めるまでの往復、強制で閉じたあとの締切の無い後始末（SFTP の
// 接続を閉じる、Vault をロックする）、ロックが落ちるのを見に行く間隔がここに入る。
// 測った値ではないので、どれもが遅れたときにも足りるよう、多めに取っている。
const engineReleaseMargin = 10 * time.Second

// engineReleasePollInterval は、席が空いたかをロックで確かめ直す間隔である。
// 空いてから起動するまでの遅れがこの間隔以下になる。ロックの取得はファイルを
// 開いて OS のロックを試すだけなので、この頻度で試しても負担にならない。
const engineReleasePollInterval = 100 * time.Millisecond

// askToReplace は、走っている engine を止めてよいかを決める。
//
// 止めてよいなら true を返す。対話入力できない環境では確認を行わない。
func askToReplace(
	ctx context.Context, found handoff.Handoff, sessions int, replace bool, stdin io.Reader, stdout io.Writer,
) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if replace {
		return true, nil
	}
	file, isFile := stdin.(*os.File)
	if !isFile || !term.IsTerminal(int(file.Fd())) {
		return false, nil
	}
	fmt.Fprintf(stdout, "sshc: an sshc engine is already running (pid %d, %s)\n", found.PID, found.URL)
	if sessions > 0 {
		fmt.Fprintf(stdout, "sshc: it has %d open terminal(s); stopping it closes them\n", sessions)
	}
	fmt.Fprintln(stdout, "sshc: stopping it also locks the password vault")
	fmt.Fprint(stdout, "sshc: stop it and start here? [y/N] ")

	type result struct {
		answer string
		err    error
	}
	answered := make(chan result, 1)
	go func() {
		answer, err := bufio.NewReader(stdin).ReadString('\n')
		answered <- result{answer: answer, err: err}
	}()
	var answer string
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case read := <-answered:
		if read.err != nil {
			return false, nil
		}
		answer = read.answer
	}
	// 既定は No である。何も読まずに Enter を叩いたユーザーが落ちる先は、
	// 失うものが無い方でなければならない。
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

// stopRunningEngine は、走っている engine に畳んで終わるよう頼み、席が空くのを待つ。
//
// 頼むのであって、殺すのではない。開いている端末も転送も vault も、engine
// 自身にしか畳めない。
func stopRunningEngine(
	ctx context.Context, stateDir string, found handoff.Handoff, client *http.Client,
	acquire func(string) (func() error, error),
) error {
	err := newHandoffEndpoint(found, client).exchange(ctx,
		handoffCall{method: http.MethodPost, path: httpserver.StopPath, body: strings.NewReader("{}")},
		handoffAnswer{status: http.StatusAccepted})
	refusal, refused := engineRefusal(err)
	switch {
	case isTransportProblem(err):
		return fmt.Errorf("the running engine did not answer: %w", err)
	case refused:
		return fmt.Errorf("the running engine refused to stop (%d)", refusal.Status)
	case err != nil:
		return err
	}

	// 席が空くまで待つ。頼んだ直後に自分が握りにいくと、まだ畳んでいる
	// 相手からロックを奪えず、理由の分からない失敗になる。
	timeout := time.NewTimer(engineReleaseTimeout)
	defer timeout.Stop()
	retry := time.NewTicker(engineReleasePollInterval)
	defer retry.Stop()
	for {
		release, err := acquire(stateDir)
		if err == nil {
			// 空いた。すぐ手放す。本番の取得はこのあと通常の道で行う。
			_ = release()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return fmt.Errorf("the running engine did not let go within %s", engineReleaseTimeout)
		case <-retry.C:
		}
	}
}

// replaceRunningEngine は、走っている engine を止めて席を受け取る。
//
// 止めてよいと決まったときだけ止める。決まらなければ、居ることとアクセス URLの
// 取り方を言って断る。
func replaceRunningEngine(
	ctx context.Context, stateDir string, options engineOptions,
	stdin io.Reader, stdout, stderr io.Writer,
	acquire func(string) (func() error, error),
) (func() error, error) {
	// 確かめる、状態を尋ねる、止まるよう頼む、のどれも engine へ送る短い要求なので、
	// ほかのコマンドと同じ上限にする。止まり終えるのを待つのは engineReleaseTimeout。
	client := &http.Client{Timeout: connectTimeout}
	found, err := verifiedHandoff(ctx, stateDir, client)
	if errors.Is(err, errEngineUnproven) {
		return nil, err
	}
	if err != nil {
		// handoff が読めないのに錠は握られている。何が居るのかを言えない以上、
		// 止めてよいとも言えない。
		return nil, fmt.Errorf("an sshc engine holds the lock but left no readable handoff; stop it yourself")
	}

	sessions := 0
	// handoff を読み直さない。上で読んだ一台にだけ尋ねる。待っている
	// あいだに書き換わったものへ乗り換えれば、止める相手が入れ替わる。
	if status, statusErr := requestStatus(ctx, found, client); statusErr == nil {
		sessions = status.Sessions
	}

	replace, askErr := askToReplace(ctx, found, sessions, options.Replace, stdin, stdout)
	if askErr != nil {
		return nil, askErr
	}
	if !replace {
		fmt.Fprintln(stderr, "sshc: an sshc engine is already running")
		fmt.Fprintln(stderr, "sshc: run sshc to get a way into it, or sshc engine --replace to take over")
		return nil, errAlreadyRunning
	}

	if err := stopRunningEngine(ctx, stateDir, found, client, acquire); err != nil {
		return nil, err
	}
	return acquire(stateDir)
}

// errAlreadyRunning は、重複するエラーメッセージの出力を防ぐための内部エラーである。
var errAlreadyRunning = errors.New("an sshc engine is already running")
