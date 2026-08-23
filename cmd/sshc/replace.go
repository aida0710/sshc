package main

import (
	"bufio"
	"bytes"
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

// 走っている engine を止めてから起こす。
//
// **どこで起こしたか分からない engine を、止められる必要がある。** tmux の中か、
// 閉じた端末か、supervisor の下か——探して回るより、走っているものに頼む方が短い。
//
// **黙って止めない。** 止めれば開いている端末も転送も落ち、保管庫は施錠される。
// 端末なら訊き、端末でなければ `--replace` を書いた人にだけ従う。

// engineReleaseTimeout は、頼んだ engine が席を空けるのを待つ上限である。
//
// **engine 自身の締切から導く。** あちらが畳むのに使ってよい時間より短く待てば、
// 間に合ったはずのものを諦めることになる。倍率は、頼みが届いてから畳み始める
// までの往復と、ロックが落ちるのを見に行く間隔ぶんの余裕である。
const engineReleaseTimeout = 5 * app.DefaultShutdownTimeout

// askToReplace は、走っている engine を止めてよいかを決める。
//
// 止めてよいなら true を返す。**訊けない場面では訊かない** ——手順の中や
// supervisor の下で問いを出せば、答える人の居ない待ちで止まったままになる。
func askToReplace(found handoff.Handoff, sessions int, replace bool, stdin io.Reader, stdout io.Writer) bool {
	if replace {
		return true
	}
	file, isFile := stdin.(*os.File)
	if !isFile || !term.IsTerminal(int(file.Fd())) {
		return false
	}
	fmt.Fprintf(stdout, "sshc: an sshc engine is already running (pid %d, %s)\n", found.PID, found.URL)
	if sessions > 0 {
		fmt.Fprintf(stdout, "sshc: it has %d live console(s); stopping it closes them\n", sessions)
	}
	fmt.Fprintln(stdout, "sshc: stopping it also locks the password vault")
	fmt.Fprint(stdout, "sshc: stop it and start here? [y/N] ")

	answer, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil {
		return false
	}
	// **既定は No である。** 何も読まずに Enter を叩いた人が落ちる先は、
	// 失うものが無い方でなければならない。
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}

// stopRunningEngine は、走っている engine に畳んで終わるよう頼み、席が空くのを待つ。
//
// **頼むのであって、殺すのではない。** 開いている端末も転送も vault も、engine
// 自身にしか畳めない。
func stopRunningEngine(
	ctx context.Context, stateDir string, found handoff.Handoff, client *http.Client,
	acquire func(string) (func() error, error),
) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, found.URL+httpserver.StopPath, bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(handoff.HeaderName, found.Secret)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("the running engine did not answer: %w", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("the running engine refused to stop (%d)", response.StatusCode)
	}

	// **席が空くまで待つ。** 頼んだ直後に自分が握りにいくと、まだ畳んでいる
	// 相手からロックを奪えず、理由の分からない失敗になる。
	deadline := time.Now().Add(engineReleaseTimeout)
	for {
		release, err := acquire(stateDir)
		if err == nil {
			// 空いた。**すぐ手放す** ——本番の取得はこのあと通常の道で行う。
			_ = release()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the running engine did not let go within %s", engineReleaseTimeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// replaceRunningEngine は、走っている engine を止めて席を受け取る。
//
// **止めてよいと決まったときだけ止める。** 決まらなければ、居ることと入口の
// 取り方を言って断る。
func replaceRunningEngine(
	ctx context.Context, home string, options engineOptions,
	stdin io.Reader, stdout, stderr io.Writer,
	acquire func(string) (func() error, error),
) (func() error, error) {
	stateDir := app.HandoffDir(home)
	found, err := readHandoff(stateDir)
	if err != nil {
		// handoff が読めないのに錠は握られている。**何が居るのかを言えない以上、
		// 止めてよいとも言えない。**
		return nil, fmt.Errorf("an sshc engine holds the lock but left no readable handoff; stop it yourself")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	sessions := 0
	if status, statusErr := engineStatus(ctx, stateDir, client); statusErr == nil {
		sessions = status.Sessions
	}

	if !askToReplace(found, sessions, options.Replace, stdin, stdout) {
		fmt.Fprintln(stderr, "sshc: an sshc engine is already running")
		fmt.Fprintln(stderr, "sshc: run sshc to get a way into it, or sshc engine --replace to take over")
		return nil, errAlreadyRunning
	}

	if err := stopRunningEngine(ctx, stateDir, found, client, acquire); err != nil {
		return nil, err
	}
	return acquire(stateDir)
}

// errAlreadyRunning は、断ったことそのものである。**理由は既に出してある** ——
// 呼び出し側はこれを見て、二度目の綴りを出さない。
var errAlreadyRunning = errors.New("an sshc engine is already running")
