package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"sshc/internal/platform"
)

// askpass ヘルパー。
//
// SSH_ASKPASS がプログラムを指定し、SSH_ASKPASS_REQUIRE が `force` のとき、
// OpenSSH は端末ではなくそのプログラムから秘密を受け取る。そのプログラムがこの
// バイナリであり、標準出力へ書いたものがそのまま回答になる。
//
// OpenSSH は指定されたプログラムを、プロンプトだけを引数として直接 exec する。
// 間にシェルは挟まらないため、サブコマンドの語を置く場所はどこにもなく、これが
// askpass としての起動であることを示すのは環境変数である。`sshc askpass
// <prompt>` は同じことを手作業で行う。
//
// このヘルパー自身は秘密を持たず、何も復号できない。パスワードは暗号化された
// ファイルに置かれ、その鍵は起動中でロック解除済みの sshc の内部にしか存在しない。
// そこでこのヘルパーはループバック経由でそのプロセスに問い合わせ、ユーザーが接続を
// 求めたときに同じプロセスが発行したワンタイムトークンを提示する。手で実行しても
// 何も得られない。トークンがなく、トークンはそれが作られた接続によって使い切られる
// からである。
//
// internal/platform はこのアプリケーションが起動するすべての子プロセスから
// SSH_ASKPASS を取り除いており、その判断は変わらない。理由は、このアプリケーション
// が選んだのではないプログラムへ秘密を渡してはならない、というものだ。ここでその
// プログラムはこのアプリケーション自身であり、絶対パスで、ひとつの alias に対して、
// ユーザーが求めたひとつの接続のためだけに武装されている。

// 武装した接続の語彙は internal/platform が持つ。ヘルパー（このファイル）と、
// それを武装させる側——コマンドラインと埋め込みターミナル——が同じ文字列を読む
// ようにするためであり、名前をここに残しているのは既存の読み手のためである。
const (
	// AliasVariable は回答が属するホストの名前を保持する。
	//
	// alias は環境変数として渡され、プロンプトから読み取ることは決してしない。OpenSSH
	// のプロンプトが運ぶのは *解決後の* ユーザー名とホスト名であり、パスワードが登録
	// されている名前とは違ううえ、解析すれば他人のソースにある書式文字列にこちらが
	// 縛られることになるからだ。
	AliasVariable = platform.AskpassAliasVariable
	// URLVariable は、これを武装した sshc のループバックエンドポイント。
	URLVariable = platform.AskpassURLVariable
	// TokenVariable は、この接続のためのワンタイムトークン。
	TokenVariable = platform.AskpassTokenVariable
	// KindVariable は、トークンが回答できる認証情報の種類を保持する。
	// アカウントパスワードと鍵パスフレーズをプロンプトの文面だけで取り違えない。
	KindVariable = platform.AskpassKindVariable
	// KeyPathVariable is the exact, resolved private-key path selected when
	// the token was issued. The helper compares it before presenting the bearer
	// token, so a prompt for another key never consumes or exposes that token.
	KeyPathVariable = platform.AskpassKeyPathVariable

	askpassKindKeyPassphrase = platform.AskpassKindKeyPassphrase

	// AskpassTokenHeader はトークンを運ぶ。独自ヘッダーは CORS のプリフライトを強制し、
	// このサーバーはそれに応答しないため、エンドポイントをどれだけ知っていてもウェブ
	// ページから到達することはできない。
	AskpassTokenHeader = "X-SSHC-Askpass"
)

// keyPassphrasePrompt は OpenSSH が秘密鍵の復号値を求める問いだけを受け入れる。
// どの鍵かの照合は、トークンを発行したサーバー側でも行う。
func keyPassphrasePrompt(prompt, expectedPath string) bool {
	if expectedPath == "" {
		return false
	}
	trimmed := strings.TrimRight(prompt, " \t\r\n")
	const prefix = "Enter passphrase for key '"
	if !strings.HasPrefix(trimmed, prefix) || !strings.HasSuffix(trimmed, "':") {
		return false
	}
	promptPath := strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), "':")
	return filepath.Clean(promptPath) == filepath.Clean(expectedPath)
}

// 終了ステータス。OpenSSH は非ゼロをすべて「回答なし」として扱うので、この区別は
// ssh のためではなく、Terminal のウィンドウを読む人のためのものである。
const (
	askpassOK      = 0
	askpassRefused = 1
	askpassNoEntry = 2
)

// terminalPrompter は、人間に問い、その答えを返す。
//
// インターフェースにしてあるのは、テストが本物の端末に触れないためである。この
// リポジトリのどのテストも、制御端末を開かない。
type terminalPrompter interface {
	// Prompt は question を人間に見せ、打たれた 1 行を返す。echo が偽なら入力を
	// 画面に出さない。
	Prompt(question string, echo bool) (string, error)
	Close() error
}

type askpassRequest struct {
	Alias  string `json:"alias"`
	Prompt string `json:"prompt"`
}

type askpassResponse struct {
	Password string `json:"password"`
}

// runAskpass は `sshc askpass <prompt>` のすべてである。
//
// 成功時は out にパスワードだけを書き、それ以外は何も書かない。拒否時は out に 1
// バイトも書かない。標準出力に出した診断メッセージは、OpenSSH にパスワードとして
// 渡されてしまうからである。
func runAskpass(
	ctx context.Context,
	arguments []string,
	lookup func(string) string,
	client *http.Client,
	out io.Writer,
	errOut io.Writer,
	openTerminal func() (terminalPrompter, error),
) int {
	if len(arguments) == 0 {
		return refuse(errOut, "sshc askpass expects the prompt as its argument")
	}
	prompt := arguments[0]

	alias := lookup(AliasVariable)
	endpoint := lookup(URLVariable)
	token := lookup(TokenVariable)
	kind := lookup(KindVariable)
	keyPath := lookup(KeyPathVariable)
	if alias == "" || endpoint == "" || token == "" {
		// 三つがそろわなければ問い合わせる相手もなく、その問いが認可されたものだと
		// 示す手段もない。違うホストに答えれば、そのホストへ資格情報を漏らすことに
		// なる。
		return refuse(errOut, "sshc askpass was started without "+
			AliasVariable+", "+URLVariable+" and "+TokenVariable)
	}
	if err := validateLoopbackEndpoint(endpoint); err != nil {
		return refuse(errOut, "sshc askpass refuses to send a password to "+endpoint)
	}

	answerable := kind == askpassKindKeyPassphrase && keyPassphrasePrompt(prompt, keyPath)
	if !answerable {
		return relayToHuman(prompt, out, errOut, openTerminal)
	}

	password, status := fetchPassword(ctx, client, endpoint, token, alias, prompt)
	if status != askpassOK {
		// A saved value is an optimisation, not the only way to unlock a key.
		// If the vault was locked or the assignment changed, preserve ordinary
		// OpenSSH behaviour and let the owner answer.
		return relayToHuman(prompt, out, errOut, openTerminal)
	}

	// OpenSSH は末尾の改行をひとつだけ取り除くので、正当に空白で終わるパスワードも
	// そのまま残る。
	if _, err := io.WriteString(out, password+"\n"); err != nil {
		return askpassRefused
	}
	return askpassOK
}

func fetchPassword(
	ctx context.Context, client *http.Client, endpoint, token, alias, prompt string,
) (string, int) {
	body, err := json.Marshal(askpassRequest{Alias: alias, Prompt: prompt})
	if err != nil {
		return "", askpassRefused
	}
	callContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(callContext, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", askpassRefused
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(AskpassTokenHeader, token)

	response, err := client.Do(request)
	if err != nil {
		return "", askpassRefused
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusConflict:
		return "", askpassNoEntry
	default:
		return "", askpassRefused
	}

	var decoded askpassResponse
	// 本文には上限を設ける。パスワードはメガバイト単位ではないし、このプロセスは読んだ
	// ものをそのまま、OpenSSH が握っているパイプへ書き込もうとしている。
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&decoded); err != nil {
		return "", askpassRefused
	}
	if decoded.Password == "" {
		return "", askpassNoEntry
	}
	return decoded.Password, askpassOK
}

// validateLoopbackEndpoint は、このマシン以外へパスワードを送ることを拒否する。
// エンドポイントは環境変数で渡される以上それは入力であり、他人のサーバーを指す
// SSHC_ASKPASS_URL がエクスポートされていれば、このヘルパーは持ち出しの道具に
// なってしまう。
func validateLoopbackEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" {
		return errNotLoopback
	}
	host, _, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return err
	}
	address := net.ParseIP(host)
	if address == nil || !address.Equal(net.IPv4(127, 0, 0, 1)) {
		return errNotLoopback
	}
	return nil
}

var errNotLoopback = &endpointError{}

type endpointError struct{}

func (*endpointError) Error() string { return "the askpass endpoint is not 127.0.0.1" }

// hostKeyPromptSuffix は、OpenSSH がホスト鍵の確認の末尾に置く文字列。
//
// 使うのは、答えを画面に出してよいかの判定にだけである。合致しなければ伏せる側へ
// 倒れるので、OpenSSH が文言を変えても、最悪 yes が見えなくなるだけで済む。
// パスフレーズが画面に残る方へは倒れない。
const hostKeyPromptSuffix = "(yes/no/[fingerprint])?"

// noTerminalMessage は、答える人間がいないときに残す説明である。
const noTerminalMessage = "sshc askpass could not reach a terminal to ask. " +
	"Answer the prompt yourself, or add the host key through the Known Hosts screen first."

// echoesTheAnswer は、この問いの答えを打ち手に見せてよいかを報告する。
//
// ホスト鍵への yes/no は秘密ではないので見せる。それ以外は、パスフレーズかもしれず
// ワンタイムコードかもしれないので伏せる。
func echoesTheAnswer(prompt string) bool {
	return strings.HasSuffix(strings.TrimRight(prompt, " \t\r\n"), hostKeyPromptSuffix)
}

// relayToHuman は、このヘルパーが答えられない問いを持ち主へ渡す。
//
// SSH_ASKPASS_REQUIRE=force は対話的な問いを *すべて* ここへ通す。ホスト鍵の確認も、
// 鍵のパスフレーズも、2FA のコードもである。答えられないからといって握りつぶせば、
// パスワードを保存したという操作が、それらに答える手段を奪ってしまう。保存して
// いなければ素の ssh が端末で訊いていたのだから、それは後退である。
//
// 答えるのは人間であって、このプログラムではない。ホスト鍵の検査は取り除かれず、
// 信頼モデルは素の ssh と同じままである。プロンプトは加工せずに見せる。別名で既知か
// 初見かを OpenSSH が本文で述べており、こちらが言い換えれば情報が減るだけである。
func relayToHuman(prompt string, out, errOut io.Writer, open func() (terminalPrompter, error)) int {
	if open == nil {
		return refuse(errOut, noTerminalMessage)
	}
	terminal, err := open()
	if err != nil {
		return refuse(errOut, noTerminalMessage)
	}
	defer func() { _ = terminal.Close() }()

	answer, err := terminal.Prompt(prompt, echoesTheAnswer(prompt))
	if err != nil {
		return refuse(errOut, "sshc askpass could not read the answer from the terminal")
	}
	// パスワードの経路と同じ形で返す。OpenSSH は末尾の改行をひとつだけ取り除く。
	if _, err := io.WriteString(out, answer+"\n"); err != nil {
		return askpassRefused
	}
	return askpassOK
}

func refuse(errOut io.Writer, message string) int {
	writeLine(errOut, message)
	return askpassRefused
}

func writeLine(w io.Writer, message string) {
	if w == nil {
		return
	}
	_, _ = io.WriteString(w, message+"\n")
}
