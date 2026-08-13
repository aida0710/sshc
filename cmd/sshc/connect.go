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
	"os/exec"
	"path/filepath"
	"time"

	"sshc/internal/config"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/platform"
)

// connectResponse は、起動中のアプリケーションが返す内容。
type connectAnswer struct {
	Alias        string   `json:"alias"`
	AskpassToken string   `json:"askpassToken"`
	AskpassKind  string   `json:"askpassKind"`
	AskpassURL   string   `json:"askpassUrl"`
	IdentityFile string   `json:"identityFile"`
	SSHConfig    string   `json:"sshConfig"`
	Warnings     []string `json:"warnings"`
}

// connectInvocation は、このプロセスが接続のために起動されたかを報告する。
//
// フラグでもなく askpass サブコマンドでもない裸の語は alias である。コマンドは
// それで全部だ。`sshc <alias>`。これが置き換える五つの環境変数は、Terminal の
// ボタンがすでに自前でやっていたことを手書きにした形にすぎず、そもそも人が打ち込む
// ためのものではなかった。
func connectInvocation(argv []string) (string, bool) {
	if len(argv) != 2 {
		return "", false
	}
	word := argv[1]
	if word == "" || word[0] == '-' || word == AskpassSubcommand || word == OpenSubcommand || word == ListSubcommand || word == ConnectSubcommand || word == HelpSubcommand || word == ServiceSubcommand {
		return "", false
	}
	return word, true
}

// OpenSubcommand は、起動中のアプリケーションをブラウザで開く。
//
// ブートストラップトークンは初回の使用で消費される。標準出力がどこにも届かないよう
// 意図して作られたバックグラウンドエージェント — その URL は有効なトークンを運ぶ
// からだ — には、これを配る手段がない。そこで、ユーザーが何かを見たいときには
// 新しいものを要求する。"open" という名のホストは、この方法では名前で接続できない
// ホストになる。語はひとつだけで、それがこれである。
const OpenSubcommand = "open"

// runOpen は、起動中のアプリケーションに入口を求め、ブラウザを開く。
func runOpen(ctx context.Context, stateDir string, client *http.Client, browser func(string) error, stderr io.Writer) int {
	found, err := handoff.Read(stateDir)
	if err != nil {
		fmt.Fprintln(stderr, "sshc: not running")
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
		fmt.Fprintln(stderr, "sshc: not answering")
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
		fmt.Fprintln(stderr, "sshc: the answer carried no way in")
		return 1
	}
	// URL はブラウザに渡すだけで、決して表示しない。有効なブートストラップトークンを
	// 運んでおり、端末は見せられたものを残すからだ。
	if err := browser(answer.URL); err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	return 0
}

// sshFinder は ssh プログラムを解決する。このアプリケーションの他のすべての部分が
// 使うのと同じ継ぎ目なので、「どの ssh か」への答えはひとつしかない。
type sshFinder interface{ SSH() (string, error) }

func executeSSH(ssh string, arguments, environment []string, stdin io.Reader, stdout, stderr io.Writer) int {
	command := exec.Command(ssh, arguments...)
	command.Env = environment
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	fmt.Fprintf(stderr, "sshc: %v\n", err)
	return 1
}

// runConnect は、この接続に必要なものを起動中のアプリケーションに尋ね、端末を ssh に
// 引き渡す。
//
// アプリケーションが動いていないことはエラーではない。ユーザーはホストへ接続したいの
// であり、`ssh <alias>` は本人が打ち込んだであろうコマンドだ。OpenSSH が自分で
// パスワードを尋ねる以上、それは機能する接続である。stderr にその旨を書けば、邪魔に
// ならずに違いが見える。
func runConnect(ctx context.Context, alias string, stateDir string, client *http.Client, toolchain sshFinder, stderr io.Writer) int {
	if err := platform.ValidateAlias(alias); err != nil {
		fmt.Fprintf(stderr, "sshc: %q is not an alias this will put on a command line\n", alias)
		return 2
	}

	var credential platform.AskpassCredential
	answer, err := askApplication(ctx, alias, stateDir, client)
	switch {
	case err != nil:
		fmt.Fprintf(stderr, "sshc: connecting without a saved key passphrase (%v)\n", err)
	default:
		for _, warning := range answer.Warnings {
			fmt.Fprintf(stderr, "sshc: %s\n", warning)
		}
		if answer.AskpassToken != "" {
			resolved, pathErr := os.Executable()
			if pathErr != nil {
				fmt.Fprintf(stderr, "sshc: connecting without a saved key passphrase (%v)\n", pathErr)
			} else {
				credential = platform.AskpassCredential{
					Helper: resolved, URL: answer.AskpassURL, Token: answer.AskpassToken,
					Kind: answer.AskpassKind, IdentityFile: answer.IdentityFile, SSHConfig: answer.SSHConfig,
				}
			}
		}
	}

	// このアプリケーションが起動する他のすべての OpenSSH プログラムと同じやり方で
	// 解決する。固定のディレクトリ一覧から絶対パスへ、である。PATH は参照しない。
	// さもなければ、上のトークンが PATH の先頭にあるものへ渡されてしまうから
	// である。
	ssh, err := toolchain.SSH()
	if err != nil || !filepath.IsAbs(ssh) {
		fmt.Fprintf(stderr, "sshc: ssh was not found where it is expected: %v\n", err)
		return 1
	}

	// 起動一式の組み立ては internal/platform が持つ。埋め込みターミナルが同じ
	// 関数を呼ぶので、環境から五つの変数を取り除く処理はこのリポジトリに一つしかない。
	session, cleanup, err := platform.InteractiveSSH(platform.InteractiveRequest{
		SSH: ssh, Alias: alias, Inherited: os.Environ(), Credential: credential,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return 1
	}
	defer cleanup()
	if session.Notice != "" {
		fmt.Fprintf(stderr, "sshc: connecting without a saved key passphrase (%s)\n", session.Notice)
	}
	return executeSSH(session.Path, session.Arguments, session.Env, os.Stdin, os.Stdout, stderr)
}

// askApplication はハンドオフを読み、接続一回分を要求する。
func askApplication(ctx context.Context, alias, stateDir string, client *http.Client) (connectAnswer, error) {
	found, err := handoff.Read(stateDir)
	if err != nil {
		return connectAnswer{}, fmt.Errorf("sshc is not running")
	}
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
