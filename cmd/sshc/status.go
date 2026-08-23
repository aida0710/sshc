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

// runStatus は、走っている engine の様子を書き出す。
//
// **既定は人が読む形である。** かつてここは JSON だけを出しており、それは
// メニューバーが読むためだった——その読み手はもう居ない。打つのは人であり、
// 人が最初に見たいのは「動いているか、金庫は開いているか、端末は何本か」である。
//
// **JSON は残す。** 手順の中から読んでいる人が居る道を、黙って塞がない。
// エンジンが居なければ 1 で終わる。
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

// writeStatus は、engine の様子を人が読む形で書く。
//
// **描くのはここだけである。** 同じ内容が `sshc status` と `sshc vault status` に
// 別々の書式で書かれていた——片方に項目を足しても、もう片方は古いままになる。
//
// **列で揃える。** 打った人が探すのは値であって、ラベルの綴りではない。
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

// vaultState は、金庫の 3 つの状態をひとつの語にする。
//
// **「無い」と「施錠」を混ぜない。** 前者に要るのは `sshc vault create` で、
// 後者に要るのは `sshc vault unlock` である——読む人が次に打つものが違う。
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
	// **無いことは、壊れていることではない。** engine を一度も起こしていない
	// 機械では handoff そのものが無く、そのまま返すと利用者が読むのは
	// `open /home/a/.ssh/sshc/cli: no such file or directory` になる。**入れた
	// 直後の人が最初に打つのがこれである** ——道の綴りではなく、何が起きていて
	// 次に何をすればよいかを言う。
	if errors.Is(err, fs.ErrNotExist) {
		return handoff.Handoff{}, engineNotRunning{cause: err}
	}
	if err != nil {
		return handoff.Handoff{}, err
	}
	return found, nil
}

// engineNotRunning は、engine が居ないことを利用者の言葉で言う。
//
// **元の err を包んだまま持つ。** 呼び出し側には `errors.Is(err, fs.ErrNotExist)`
// で判定しているところがあり、文言を変えるためにその判定を壊さない。
type engineNotRunning struct{ cause error }

func (e engineNotRunning) Error() string {
	return "sshc is not running; run sshc engine in another terminal and keep it open"
}

func (e engineNotRunning) Unwrap() error { return e.cause }

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
