package main

import (
	"context"
	"crypto/rand"
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
func runStatus(ctx context.Context, environment commandEnvironment, asJSON bool) int {
	found, answer, err := verifiedStatus(ctx, environment.stateDir, environment.client)
	if err != nil {
		if asJSON {
			return finishCommandFailure(commandFailureReport{
				cause: err, failure: classifyCommandFailure(err), asJSON: true, stdout: environment.stdout,
			})
		}
		// engine に届かなかった理由は、sshc open や sshc ssh と同じ分け方で出す。
		return reportEngineUnreachable(ctx, err, environment.stderr)
	}
	if asJSON {
		if err := writeCommandSuccess(environment.stdout, answer); err != nil {
			fmt.Fprintf(environment.stderr, "sshc: %v\n", err)
			return exitFailure
		}
		return 0
	}
	writeStatus(environment.stdout, found, answer)
	return 0
}

// verifiedStatus は、handoff の engine が秘密を持つことを確かめてから、その engine に
// 状態を尋ねる。
func verifiedStatus(ctx context.Context, stateDir string, client *http.Client) (handoff.Handoff, statusAnswer, error) {
	found, err := verifiedHandoff(ctx, stateDir, client)
	if err != nil {
		return handoff.Handoff{}, statusAnswer{}, err
	}
	answer, err := requestStatus(ctx, found, client)
	return found, answer, err
}

// writeStatus は status と vault status で共有する表形式を出力する。
func writeStatus(out io.Writer, found handoff.Handoff, answer statusAnswer) {
	rows := [][2]string{
		{"engine", fmt.Sprintf("running (pid %d)", found.PID)},
		{"address", found.URL},
		{"version", answer.Version},
		{"protocol", strconv.Itoa(answer.ProtocolVersion)},
		{"vault", vaultState(answer)},
		// engine は終了していないターミナルだけを数える。sshc terminal list は終了済みも
		// 並べるので、件数が違っても読み違えないよう「open」を付ける。
		{"terminals", fmt.Sprintf("%d open", answer.Sessions)},
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
	case answer.Vault && answer.Unlocked && answer.Passwordless:
		return "unlocked (passwordless)"
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
	return fetchEngineStatus(ctx, client, found, httpserver.StatusPath)
}

// statusAnswer は engine の状態応答である。
type statusAnswer struct {
	Passwordless    bool          `json:"passwordless"`
	Owner           handoff.Owner `json:"owner"`
	Version         string        `json:"version"`
	ProtocolVersion int           `json:"protocolVersion"`
	// Vault は、開けるべき錠がそもそも有るか。
	Vault    bool `json:"vault"`
	Unlocked bool `json:"unlocked"`
	Sessions int  `json:"sessions"`
}

// verifiedHandoff は handoff を読み、その URL にいる process が handoff の秘密を持つ
// engine であることを確かめてから返す。engine が終了処理を経ずに消えると handoff
// だけが残り、同じ port を別の process が取れる。確かめる前は秘密も資格情報も送らない。
func verifiedHandoff(ctx context.Context, stateDir string, client *http.Client) (handoff.Handoff, error) {
	found, err := readHandoff(stateDir)
	if err != nil {
		return handoff.Handoff{}, err
	}
	// 呼び出し側の上限のまま確かめる。応答しない process の前で、status や ssh が
	// 長いコマンド用の上限まで待たないようにする。
	if err := proveEngine(ctx, found, noRedirectClient(client)); err != nil {
		return handoff.Handoff{}, err
	}
	return found, nil
}

// proveEngine は乱数を送り、handoff の秘密で署名した答えが返ることを確かめる。
func proveEngine(ctx context.Context, found handoff.Handoff, client *http.Client) error {
	challenge, err := handoff.MintChallenge(rand.Reader)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, found.URL+httpserver.ChallengePath, nil)
	if err != nil {
		return err
	}
	request.Header.Set(handoff.ChallengeHeader, challenge)
	response, err := client.Do(request)
	if err != nil {
		if response != nil {
			discardEngineResponse(response)
		}
		return transportProblem(err, false)
	}
	discardEngineResponse(response)
	if response.StatusCode != http.StatusNoContent ||
		!handoff.VerifyProof(found.Secret, challenge, response.Header.Get(handoff.ProofHeader)) {
		return errEngineUnproven
	}
	return nil
}

// readHandoff は CLI の全サブコマンドで同じ互換性判定を使う。旧形式を補完すると、
// owner や protocol を知らないまま稼働中の app へ要求を送れてしまうため、バージョンを
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
