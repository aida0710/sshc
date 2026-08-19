package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

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
// そろえるという復旧可能な失敗として返す。
//
// **「アプリを再起動してください」とは言わない。** 食い違っているのがどちら側かを、
// このプロセスは知らない——アプリが新しいのかもしれないし、いま走っているこの
// 実体が古いのかもしれない。そして**古いのがこちらである状況は、この設計が自分で
// 作っている**: 外殻は `~/.local/bin/sshc` に自分が張ったリンク以外のものがあれば
// 触らないので、`make install` で入れた実体はアプリを入れ替えても古いまま残る。
// 再起動を勧めても、その人の `sshc` は変わらない。
//
// 代わりに、いま話しているのがどの実体かを名指しする。**どちらを直すかを決める
// のに要るのは、それである。**
func readHandoff(stateDir string) (handoff.Handoff, error) {
	found, err := handoff.Read(stateDir)
	if errors.Is(err, handoff.ErrSchemaVersion) || errors.Is(err, handoff.ErrProtocolVersion) {
		return handoff.Handoff{}, fmt.Errorf(
			"the running app and this sshc (%s) are not the same version; update whichever is older: %w",
			runningExecutable(), err)
	}
	if err != nil {
		return handoff.Handoff{}, err
	}
	return found, nil
}

// runningExecutable は、いま走っているこの実体の綴りを返す。
//
// 読めない環境では名前だけを返す。**綴りが分からないことは、版が合わないことを
// 伝えない理由にはならない。**
func runningExecutable() string {
	path, err := os.Executable()
	if err != nil || path == "" {
		return "sshc"
	}
	return path
}
