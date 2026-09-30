package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
	"sshc/internal/validate"
)

// connectAnswer は engine が返す接続情報である。
type connectAnswer struct {
	Alias string `json:"alias"`
	// Passphrases は接続経路で使用する鍵パスフレーズ。キーはワークスペース相対パス。
	Passphrases map[string]string `json:"passphrases"`
	// Passwords は、この接続に現れる alias ごとの保存済みアカウントパスワード。
	// 行き先だけでなく ProxyJump の手前も含む。Passphrase とは別の名前空間である。
	Passwords        map[string]string `json:"passwords"`
	PasswordBindings map[string]string `json:"passwordBindings"`
	StalePasswords   []string          `json:"stalePasswords"`
	TOTPs            map[string]string `json:"totps"`
	TOTPBindings     map[string]string `json:"totpBindings"`
	StaleTOTPs       []string          `json:"staleTotps"`
	Warnings         []string          `json:"warnings"`
}

// runOpen は engine から一度限りの UI URL を取得して出力する。
// open が true の場合だけ URL をブラウザへ渡す。
func runOpen(ctx context.Context, environment commandEnvironment, open bool) int {
	stateDir, client, stdout, stderr := environment.stateDir, environment.client, environment.stdout, environment.stderr
	found, err := verifiedHandoff(ctx, stateDir, client)
	if err != nil {
		return reportEngineUnreachable(ctx, err, stderr)
	}
	var answer struct {
		URL string `json:"url"`
	}
	err = newHandoffEndpoint(found, client).exchange(ctx,
		handoffCall{method: http.MethodPost, path: httpserver.OpenPath, body: strings.NewReader("{}")},
		handoffAnswer{status: http.StatusOK, into: &answer})
	refusal, refused := engineRefusal(err)
	switch {
	case isTransportProblem(err):
		fmt.Fprintln(stderr, "sshc: the running engine did not respond")
		return exitFailure
	case refused:
		fmt.Fprintf(stderr, "sshc: %v\n", refusedRequestError(refusal))
		return exitFailure
	case err != nil || answer.URL == "":
		fmt.Fprintln(stderr, "sshc: the engine response did not include a UI URL")
		return exitFailure
	}
	fmt.Fprintln(stdout, answer.URL)
	if open {
		// URL は既に出力済みなので、ブラウザ起動の失敗は終了コードに反映しない。
		openInBrowser(answer.URL)
	}
	return 0
}

// runConnect は engine から保存済み認証情報を取得し、プロセス内で SSH 接続する。
// engine に接続できない場合は認証情報なしの接続へフォールバックしない。
func runConnect(ctx context.Context, alias string, environment commandEnvironment) int {
	home, stateDir, client, stdin, stdout, stderr :=
		environment.home, environment.stateDir, environment.client, environment.stdin, environment.stdout, environment.stderr
	if err := validate.Alias(alias); err != nil {
		fmt.Fprintf(stderr, "sshc: %q is not an alias this will connect to\n", alias)
		return exitUsage
	}

	// ctx は呼び出し側が notifySignals で作る。シグナルで止まったら、どの段階でも
	// interactiveStopExitCode の終了コードで終える。SSH 開始後の Ctrl-C は、raw の
	// ターミナルからそのままリモートへ届くのでシグナルにならない。
	session, err := reachUnlockedEngine(ctx, stateDir, client, func(found handoff.Handoff) engineProbe {
		return httpProbe{found: found, client: client}
	})
	if err != nil {
		if code, stopped := interactiveStopExitCode(ctx); stopped {
			return code
		}
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return exitFailure
	}

	// 解錠確認後、指定された alias の接続情報を一度だけ要求する。
	answer, err := session.Connection(ctx, alias)
	if err != nil {
		if code, stopped := interactiveStopExitCode(ctx); stopped {
			return code
		}
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return exitFailure
	}

	writeConnectionNotices(stderr, answer)
	// engine は ProxyJump を含む接続経路を解決済み。保存値が無い場合は端末で入力する。
	connection, err := app.NewCLIConnection(app.CLIConnectionOptions{
		Home:        home,
		Passphrase:  savedPassphraseFor(answer),
		Password:    savedPasswordFor(answer),
		OneTimeCode: savedTOTPFor(answer),
		VPNRoute:    vpnRouteThroughEngine(stateDir, client),
	})
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return exitFailure
	}
	process, err := connection.Open(ctx, alias, terminal.DefaultSize())
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", describeConnectionFailure(err))
		return exitFailure
	}
	// Attach は ctx が止まると接続を閉じ、ターミナルを戻してから戻る。
	code, err := sshclient.Attach(ctx, process, stdin, stdout)
	if stoppedCode, stopped := interactiveStopExitCode(ctx); stopped {
		return stoppedCode
	}
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return exitFailure
	}
	return code
}

// writeConnectionNotices は対話接続と非対話実行に同じ接続診断を表示する。
func writeConnectionNotices(stderr io.Writer, answer connectAnswer) {
	for _, warning := range answer.Warnings {
		fmt.Fprintf(stderr, "sshc: %s\n", warning)
	}
	for _, stale := range answer.StalePasswords {
		fmt.Fprintf(stderr, "sshc: saved password for %s was not used because its authentication route changed; select the password again in Connections to confirm the current route\n", stale)
	}
	for _, stale := range answer.StaleTOTPs {
		fmt.Fprintf(stderr, "sshc: saved one-time password for %s was not used because its authentication route changed; select it again in Connections to confirm the current route\n", stale)
	}
}

// requestConnection は、確かめ済みの一台に接続一回分を要求する。
func requestConnection(ctx context.Context, found handoff.Handoff, alias string, client *http.Client) (connectAnswer, error) {
	body, err := json.Marshal(map[string]string{"alias": alias})
	if err != nil {
		return connectAnswer{}, err
	}
	var answer connectAnswer
	err = newHandoffEndpoint(found, client).exchange(ctx,
		handoffCall{method: http.MethodPost, path: httpserver.ConnectPath, body: bytes.NewReader(body)},
		handoffAnswer{status: http.StatusOK, into: &answer, limit: maxConnectAnswer})
	switch {
	case isTransportProblem(err):
		return connectAnswer{}, fmt.Errorf("sshc is not answering")
	case err != nil:
		return connectAnswer{}, explainRefusedRequest(err)
	}
	return answer, nil
}

// maxConnectAnswer は接続情報の応答を読む上限である。応答が運ぶのは、接続経路で使う
// 保存済みの鍵パスフレーズ・パスワード・ワンタイムパスワードと短い警告だけで、保存済みの
// 値はどれも 1 MiB までの Vault から取り出す。JSON のエスケープで膨らんでも収まるよう、
// Vault の上限の4倍を取る。
const maxConnectAnswer = 4 << 20

// connectTimeout は、CLI が engine へ送る短い要求（challenge、status、接続情報、
// UI URL の発行）のそれぞれに上限を設ける。ネットワーク越しに何かをするのではなく、
// このマシン上のプロセスに尋ねているだけだ。
const connectTimeout = 5 * time.Second
