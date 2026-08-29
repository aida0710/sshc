package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"time"

	"sshc/internal/app"
	"sshc/internal/config"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/sshclient"
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
	Warnings         []string          `json:"warnings"`
}

// OpenSubcommand は起動中の engine から UI URL を取得する予約語である。
const OpenSubcommand = "open"

// runOpen は engine から一度限りの UI URL を取得して出力する。
// open が true の場合だけ URL をブラウザへ渡す。
func runOpen(
	ctx context.Context, stateDir string, client *http.Client, stdout, stderr io.Writer, open bool,
) int {
	found, err := readHandoff(stateDir)
	if err != nil {
		if errors.Is(err, handoff.ErrSchemaVersion) || errors.Is(err, handoff.ErrProtocolVersion) {
			fmt.Fprintf(stderr, "sshc: %v\n", err)
			return 1
		}
		fmt.Fprintln(stderr, "sshc: not running")
		fmt.Fprintln(stderr, "sshc: start one with `sshc engine`, and keep that terminal open")
		return 1
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, found.URL+httpserver.OpenPath, bytes.NewReader([]byte("{}")))
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(handoff.HeaderName, found.Secret)
	response, err := client.Do(request)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: the running engine did not respond")
		return 1
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		fmt.Fprintln(stderr, "sshc: refused")
		return 1
	}
	var answer struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&answer); err != nil || answer.URL == "" {
		fmt.Fprintln(stderr, "sshc: the engine response did not include a UI URL")
		return 1
	}
	fmt.Fprintln(stdout, answer.URL)
	if open {
		// URL は既に出力済みなので、ブラウザ起動の失敗は終了コードに反映しない。
		openInBrowser(ctx, answer.URL)
	}
	return 0
}

// runConnect は engine から保存済み認証情報を取得し、プロセス内で SSH 接続する。
// engine に接続できない場合は認証情報なしの接続へフォールバックしない。
func runConnect(
	ctx context.Context, alias, home, stateDir string, client *http.Client, stdin *os.File, stdout, stderr io.Writer,
) int {
	if err := validate.Alias(alias); err != nil {
		fmt.Fprintf(stderr, "sshc: %q is not an alias this will connect to\n", alias)
		return 2
	}

	// 接続待機中だけ Ctrl-C を処理する。SSH 開始後は raw 端末へそのまま渡す。
	waitCtx, stopWaiting := signal.NotifyContext(ctx, os.Interrupt)
	session, err := reachUnlockedEngine(waitCtx, stateDir, client, func(found handoff.Handoff) engineProbe {
		return httpProbe{found: found, client: client}
	}, stderr)
	stopWaiting()
	if err != nil {
		if errors.Is(err, errInterrupted) {
			return 130
		}
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}

	// 解錠確認後、指定された alias の接続情報を一度だけ要求する。
	answer, err := session.Connection(ctx, alias)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}

	writeConnectionNotices(stderr, answer)
	// engine は ProxyJump を含む接続経路を解決済み。保存値が無い場合は端末で入力する。
	connection, err := app.NewCLIConnection(home,
		savedPassphraseFor(answer), savedPasswordFor(answer))
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	process, err := connection.Open(ctx, alias, sshclient.DefaultLocalSize)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", connectAdvice(err))
		return 1
	}
	code, err := sshclient.Attach(ctx, process, stdin, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
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
}

// connectAdvice は設定競合の解消方法をエラーに追加する。
func connectAdvice(err error) error {
	if errors.Is(err, sshclient.ErrProxyCommandWithJump) {
		return fmt.Errorf("%w; keep whichever one you meant", err)
	}
	return err
}

// requestConnection は、確かめ済みの一台に接続一回分を要求する。
func requestConnection(ctx context.Context, found handoff.Handoff, alias string, client *http.Client) (connectAnswer, error) {
	body, err := json.Marshal(map[string]string{"alias": alias})
	if err != nil {
		return connectAnswer{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		found.URL+httpserver.ConnectPath, bytes.NewReader(body))
	if err != nil {
		return connectAnswer{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(handoff.HeaderName, found.Secret)

	response, err := client.Do(request)
	if err != nil {
		return connectAnswer{}, fmt.Errorf("sshc is not answering")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return connectAnswer{}, fmt.Errorf("sshc refused the request")
	}
	var answer connectAnswer
	if err := json.NewDecoder(io.LimitReader(response.Body, config.MaxSnapshotSize+(64<<10))).Decode(&answer); err != nil {
		return connectAnswer{}, err
	}
	return answer, nil
}

// connectTimeout は、これが行う唯一のリクエストに上限を設ける。ネットワーク越しに何か
// をするのではなく、このマシン上のプロセスにトークンを尋ねているだけだ。
const connectTimeout = 5 * time.Second
