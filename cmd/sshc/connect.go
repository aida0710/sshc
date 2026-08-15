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
	"time"

	"golang.org/x/term"

	"sshc/internal/app"
	"sshc/internal/config"
	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/platform"
	"sshc/internal/sshclient"
)

// connectAnswer は、起動中のアプリケーションが返す内容。
type connectAnswer struct {
	Alias string `json:"alias"`
	// KeyPath は、その接続に使う鍵のワークスペース相対パス。
	KeyPath string `json:"keyPath"`
	// Passphrase は、その鍵について保存されている答え。無ければ空である。
	Passphrase string `json:"passphrase"`
	// Passwords は、この接続に現れる alias ごとの保存済みアカウントパスワード。
	// 行き先だけでなく ProxyJump の手前も含む。**Passphrase とは別の名前空間である。**
	Passwords map[string]string `json:"passwords"`
	Warnings  []string          `json:"warnings"`
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
	if word == "" || word[0] == '-' || word == OpenSubcommand || word == ListSubcommand ||
		word == ConnectSubcommand || word == HelpSubcommand || word == StatusSubcommand {
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

// openInvocation は、`sshc open` とその引数を切り出す。
func openInvocation(argv []string) ([]string, bool) {
	if len(argv) >= 2 && argv[1] == OpenSubcommand {
		return argv[2:], true
	}
	return nil, false
}

// runOpen は、起動中のアプリケーションに入口を求め、それを書き出す。
//
// **開く相手はもう居ない。** 画面はデスクトップの外殻が出すので、この
// コマンドの仕事は「入口をひとつ発行して渡す」ことだけになった。渡す先は、
// 自分でそれを開く親プロセスか、それを読む人である。
func runOpen(
	ctx context.Context, stateDir string, client *http.Client, stdout, stderr io.Writer,
) int {
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
	fmt.Fprintln(stdout, answer.URL)
	return 0
}

// waitForHandoff は、ロックを取った方が handoff を書き終えるのを短く待つ。
//
// **待つのはロックに負けた経路だけである。** そこには「1 台目が確かに居る」と
// いう根拠がある——ロックがそう言った。`sshc open` を打った人には根拠が無いので、
// 居なければ待たずに 1 で終わる方がよい。
//
// 上限を持つのは、1 台目が listener を上げる前に殺された日でも、ここで永久に
// 待たないためである。
func waitForHandoff(ctx context.Context, stateDir string) {
	for attempt := 0; attempt < 40; attempt++ {
		if _, err := handoff.Read(stateDir); err == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// sshFinder は ssh プログラムを解決する。このアプリケーションの他のすべての部分が
// 使うのと同じ継ぎ目なので、「どの ssh か」への答えはひとつしかない。
type sshFinder interface{ SSH() (string, error) }

// runConnect は、この接続に必要なものを起動中のアプリケーションに尋ね、
// **このプロセスの中で** SSH を話す。
//
// アプリケーションが動いていないことはエラーではない。保存済みパスフレーズが
// 手に入らないだけで、その鍵のパスフレーズは端末で尋ねられる——尋ねられる端末が
// ここにはある。stderr にその旨を書けば、邪魔にならずに違いが見える。
func runConnect(
	ctx context.Context, alias, home, stateDir string, client *http.Client,
	stdin *os.File, stdout, stderr io.Writer,
) int {
	if err := platform.ValidateAlias(alias); err != nil {
		fmt.Fprintf(stderr, "sshc: %q is not an alias this will connect to\n", alias)
		return 2
	}

	var saved func(string) (string, bool)
	var password func(string) (string, bool)
	answer, err := askApplication(ctx, alias, stateDir, client)
	if err != nil && launchApp(ctx) {
		// **上がるまで待つ。** 待ち方を知っているのはここだけで、
		// 上限は外殻が入口を書き出すのに掛ける時間と同じにしてある。
		for attempt := 0; attempt < 40 && err != nil; attempt++ {
			time.Sleep(500 * time.Millisecond)
			answer, err = askApplication(ctx, alias, stateDir, client)
		}
	}

	// **ブラウザを開かずに答えられる。** 解錠はエンジンの中に残るので、
	// あとで窓を開けば解錠済みである。
	//
	// **端末でなければ尋ねない。** パイプの向こうに問いを出しても答えは返って
	// こない——答えられない問いを書き置いて、そのまま先へ進むだけである。
	if err == nil && term.IsTerminal(int(stdin.Fd())) && locked(ctx, stateDir, client) {
		fmt.Fprint(stderr, "sshc: master password (leave empty to skip): ")
		typed, readErr := term.ReadPassword(int(stdin.Fd()))
		fmt.Fprintln(stderr)
		if readErr == nil && len(typed) > 0 {
			// **間違えても聞き直さない。** 繋げることの方が強い要求であり、
			// 誤りの遅延を端末で待たせる価値が無い。
			if unlock(ctx, stateDir, client, string(typed)) {
				answer, _ = askApplication(ctx, alias, stateDir, client)
			}
		}
	}
	switch {
	case err != nil:
		fmt.Fprintf(stderr, "sshc: connecting without a saved key passphrase (%v)\n", err)
	default:
		for _, warning := range answer.Warnings {
			fmt.Fprintf(stderr, "sshc: %s\n", warning)
		}
		if answer.KeyPath != "" && answer.Passphrase != "" {
			saved = func(relativePath string) (string, bool) {
				if relativePath != answer.KeyPath {
					return "", false
				}
				return answer.Passphrase, true
			}
		}
		// **本体が連鎖を解決して、そこに現れる alias のぶんだけを返している。**
		// 手前に立つホストも別の alias としてここに入る。表に無いものは
		// 保存が無いということであり、そのときは端末で尋ねる。
		if len(answer.Passwords) > 0 {
			password = func(candidate string) (string, bool) {
				stored, found := answer.Passwords[candidate]
				return stored, found && stored != ""
			}
		}
	}

	connection, err := app.NewCLIConnection(home, saved, password)
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

// connectAdvice は、断った理由に「では何をすればよいか」を添える。
func connectAdvice(err error) error {
	if errors.Is(err, sshclient.ErrProxyCommand) {
		return fmt.Errorf("%w; ~/.ssh/config is untouched, so ssh %s still works", err, "<alias>")
	}
	return err
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
