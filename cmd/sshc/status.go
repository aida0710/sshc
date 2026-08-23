package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"io/fs"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// runStatus は engine の状態を表形式または JSON で出力する。
func runStatus(
	ctx context.Context, stateDir string, client *http.Client, asJSON bool, stdout, stderr io.Writer,
) int {
	found, err := readHandoff(stateDir)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	answer, err := requestStatus(ctx, found, client)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	if asJSON {
		encoded, err := json.Marshal(answer)
		if err != nil {
			fmt.Fprintf(stderr, "sshc: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(encoded))
		return 0
	}
	writeStatus(stdout, found, answer)
	return 0
}

// writeStatus は status と vault status で共有する表形式を出力する。
func writeStatus(out io.Writer, found handoff.Handoff, answer statusAnswer) {
	rows := [][2]string{
		{"engine", fmt.Sprintf("running (pid %d)", found.PID)},
		{"address", found.URL},
		{"version", answer.Version},
		{"protocol", strconv.Itoa(answer.ProtocolVersion)},
		{"vault", vaultState(answer)},
		{"consoles", strconv.Itoa(answer.Sessions)},
	}
	width := 0
	for _, row := range rows {
		if len(row[0]) > width {
			width = len(row[0])
		}
	}
	for _, row := range rows {
		fmt.Fprintf(out, "%-*s  %s\n", width, row[0], row[1])
	}
}

// vaultState は Vault の状態を CLI 表示用の値に変換する。
func vaultState(answer statusAnswer) string {
	switch {
	case answer.Vault && answer.Unlocked:
		return "unlocked"
	case answer.Vault:
		return "locked"
	default:
		return "missing"
	}
}

// requestStatus は取得済みの handoff が示す engine に状態を要求する。
// 要求中の接続先変更を防ぐため handoff は読み直さない。
func requestStatus(ctx context.Context, found handoff.Handoff, client *http.Client) (statusAnswer, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		found.URL+httpserver.StatusPath, nil)
	if err != nil {
		return statusAnswer{}, err
	}
	request.Header.Set(handoff.HeaderName, found.Secret)

	response, err := client.Do(request)
	if err != nil {
		return statusAnswer{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return statusAnswer{}, fmt.Errorf("sshc refused the request")
	}
	var answer statusAnswer
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&answer); err != nil {
		return statusAnswer{}, err
	}
	return answer, nil
}

// statusAnswer は engine の状態応答である。
type statusAnswer struct {
	Owner           handoff.Owner `json:"owner"`
	Version         string        `json:"version"`
	ProtocolVersion int           `json:"protocolVersion"`
	// Vault は、開けるべき錠がそもそも有るか。
	Vault    bool `json:"vault"`
	Unlocked bool `json:"unlocked"`
	Sessions int  `json:"sessions"`
}

// readHandoff は CLI の全サブコマンドで同じ互換性判定を使う。旧形式を補完すると、
// owner や protocol を知らないまま稼働中の app へ要求を送れてしまうため、版を
// そろえるという復旧可能な失敗として返す。
//
// 互換性エラーには現在の実行ファイルを含める。engine と CLI のどちらが古いかは
// 判定できないため、特定の側の再起動は案内しない。
func readHandoff(stateDir string) (handoff.Handoff, error) {
	found, err := handoff.Read(stateDir)
	if errors.Is(err, handoff.ErrSchemaVersion) || errors.Is(err, handoff.ErrProtocolVersion) {
		return handoff.Handoff{}, fmt.Errorf(
			"the running app and this sshc (%s) are not the same version; update whichever is older: %w",
			runningExecutable(), err)
	}
	// handoff が無い場合は内部パスではなく engine の起動方法を案内する。
	if errors.Is(err, fs.ErrNotExist) {
		return handoff.Handoff{}, engineNotRunning{cause: err}
	}
	if err != nil {
		return handoff.Handoff{}, err
	}
	return found, nil
}

// engineNotRunning は engine の停止状態と元の fs.ErrNotExist を保持する。
type engineNotRunning struct{ cause error }

func (e engineNotRunning) Error() string {
	return "sshc is not running; run sshc engine in another terminal and keep it open"
}

func (e engineNotRunning) Unwrap() error { return e.cause }

// runningExecutable は現在の実行ファイルパスを返し、取得できない場合は名前を返す。
func runningExecutable() string {
	path, err := os.Executable()
	if err != nil || path == "" {
		return "sshc"
	}
	return path
}
