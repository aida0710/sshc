package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
)

// runStatus は、エンジンの様子をそのまま JSON で書き出す。
//
// **これは人のための表示ではない。** 読むのはメニューバーであり、だから
// 整形もしないし、翻訳もしない。エンジンが居なければ 1 で終わる。
func runStatus(
	ctx context.Context, stateDir string, client *http.Client, stdout, stderr io.Writer,
) int {
	answer, err := engineStatus(ctx, stateDir, client)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	encoded, err := json.Marshal(answer)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(encoded))
	return 0
}

// engineStatus は、handoff を読んだうえで、そのエンジンに尋ねる。
func engineStatus(ctx context.Context, stateDir string, client *http.Client) (statusAnswer, error) {
	found, err := readHandoff(stateDir)
	if err != nil {
		return statusAnswer{}, err
	}
	return requestStatus(ctx, found, client)
}

// requestStatus は、渡された一台にだけ尋ねる。
//
// **handoff を読み直さない。** 待っているあいだに書き換わったものへ黙って
// 乗り換えれば、利用者が起こしたのではないエンジンが接続材料を渡しうる。
// 入れ替わりは、この一台が答えなくなることとして現れる。
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

// statusAnswer は、エンジンが答える「いまどうなっているか」である。
type statusAnswer struct {
	Owner           handoff.Owner `json:"owner"`
	Version         string        `json:"version"`
	ProtocolVersion int           `json:"protocolVersion"`
	// Vault は、開けるべき錠がそもそも有るか。
	Vault    bool `json:"vault"`
	Unlocked bool `json:"unlocked"`
	Sessions int  `json:"sessions"`
}

// readHandoff は CLI の全入口で同じ互換性判定を使う。旧形式を推測して補うと、
// owner や protocol を知らないまま稼働中の app へ要求を送れてしまうため、版を
// そろえて再起動するという復旧可能な失敗として返す。
func readHandoff(stateDir string) (handoff.Handoff, error) {
	found, err := handoff.Read(stateDir)
	if errors.Is(err, handoff.ErrSchemaVersion) || errors.Is(err, handoff.ErrProtocolVersion) {
		return handoff.Handoff{}, fmt.Errorf("running app and CLI must use the same version; restart the app: %w", err)
	}
	if err != nil {
		return handoff.Handoff{}, err
	}
	return found, nil
}
