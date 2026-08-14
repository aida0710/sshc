package httpserver

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/handoff"
	"sshc/internal/platform"
	"sshc/internal/secret"
	"sshc/internal/session"
)

// ConnectPath は、コマンドラインが接続に必要なものを尋ねる場所である。
//
// これは意図的に /api/ の下に置かれていない。その面はブラウザ向けであり、
// セッション cookie と CSRF ヘッダーと Fetch Metadata で守られている。
// シェルはそのいずれでもなく、そのいずれも持たない。このルートは代わりに、
// 実行中のアプリケーションが state ディレクトリに残した secret で認証する。
// 埋め込みターミナルのストリームも /api/ の外に置かれているのと同じ理由による。
const ConnectPath = "/cli/connect"

// maxConnectBody はリクエストを制限する。alias は 1 語である。
const maxConnectBody = 4 << 10

// ConnectHandlers は `sshc <alias>` に応答する。
type ConnectHandlers struct {
	// Secret は呼び出し側が提示すべきものである。空であればすべての
	// リクエストを拒否する。handoff を書けなかったサーバーは受け付けてはならない。
	Secret string
	// Passwords は保存済みの鍵パスフレーズとアカウントパスワードを持つ。nil で
	// あれば保存された答えは一切提供されず、それはプロンプトが出る正常な接続である。
	Passwords *secret.Service
	// KeyPassphraseTarget resolves the one direct workspace key whose saved
	// passphrase may answer this connection. A false result is never guessed.
	KeyPassphraseTarget func(alias string) (relativePath, promptPath, configSnapshot, evidence string, ok bool, err error)
	// Warnings は、OpenSSH がこの host に対して実行するディレクティブを報告する。
	// 接続の最中に気付くのではなく、事前に伝えられる。
	Warnings func(alias string) []string
	// Aliases は、この接続に現れる alias を、ProxyJump の手前も含めて返す。
	// nil なら行き先ひとつだけを見る。
	Aliases func(alias string) []string
	// Sessions はブラウザへの入口を発行し、BaseURL はその入口が導く先である。
	// 両方が nil であれば、このアプリケーションはコマンドラインから開けない。
	// これは session manager を持たないビルドの状態である。
	Sessions *session.Manager
	BaseURL  string
	// Shutdown は、この常駐を終わらせる。nil なら止める手段が無いと答える。
	//
	// **これを呼ぶのはデスクトップの外殻であって、画面ではない。** 画面から
	// 常駐を止める道は用意しない——ウィンドウを閉じることと、常駐を終わらせることは
	// 別の意思である。
	Shutdown func()
}

type connectRequest struct {
	Alias string `json:"alias"`
}

// connectResponse は、`sshc <接続先>` が接続に使うものである。
//
// **単回トークンではなく答えそのものを返す。** トークンにしていたのは、
// 引き換える相手が OpenSSH の起こす別のプログラムだったからである。要求を
// 出した本人が答えを受け取るなら、発行と引き換えを分ける理由が無い。
// localhost を通るものは変わっていない——いままでもパスフレーズはこの経路を
// 通っており、間に立つプログラムがひとつ消えただけである。
type connectResponse struct {
	Alias string `json:"alias"`
	// KeyPath は、その接続に使う鍵のワークスペース相対パス。
	KeyPath string `json:"keyPath,omitempty"`
	// Passphrase は、その鍵について保存されている答え。無ければ空である。
	Passphrase string `json:"passphrase,omitempty"`
	// Passwords は、この接続に現れる alias ごとの保存済みアカウントパスワード。
	//
	// **行き先ひとつではなく連鎖ぶんである。** ProxyJump の手前に立つホストは
	// それ自身が alias であり、そこにもパスワードは保存されうる。行き先のぶん
	// だけを渡すと、手前で止まる接続がそのたびに手入力を求める。
	//
	// **Passphrase とは別の名前空間である。** あちらはローカルの秘密鍵を開く
	// ための秘密で、こちらはリモートのアカウントへログインするための秘密である。
	// 混ぜれば、鍵を開くための秘密がそのままリモートへ送られる。
	Passwords map[string]string `json:"passwords,omitempty"`
	Warnings  []string          `json:"warnings"`
}

// savedPassword は、その alias について保存されているアカウントパスワードを返す。
//
// **これが載るのは、この経路がもう外部のプログラムへ渡さないからである。**
// askpass だった頃は、答えを受け取るのが OpenSSH の起こす別のプログラムだった。
// いまは要求を出した `sshc` 自身が受け取り、自分でプロトコルを話す。渡す先が
// 増えないなら、埋め込みターミナルと違う答えを返す理由も無い。
//
// この経路を読めるのは `~/.ssh/sshc/cli`（0600）を読める者だけであり、その者は
// すでに、どの alias についても保存済みパスフレーズを引き出せる。**秘密が一種類
// 増えることは書いておく**——境界は動かないが、動かないことは自明ではない。
// **返すのはこの接続に現れる alias のぶんだけである。** 保管庫を一覧にはしない
// ——尋ねられた接続に要るものと、要らないものを区別する。
func savedPasswords(passwords *secret.Service, aliases []string) map[string]string {
	if passwords == nil {
		return nil
	}
	found := map[string]string{}
	for _, alias := range aliases {
		if password := passwords.PasswordFor(alias); password != "" {
			found[alias] = password
		}
	}
	if len(found) == 0 {
		return nil
	}
	return found
}

// connectionAliases は、この接続に現れる alias を返す。行き先と、ProxyJump の
// 手前に立つホストである。連鎖を解決できなければ行き先だけを返す——解決の失敗は
// このあとの接続そのものが報告するので、ここで二度言わない。
func (h ConnectHandlers) connectionAliases(alias string) []string {
	if h.Aliases == nil {
		return []string{alias}
	}
	if listed := h.Aliases(alias); len(listed) > 0 {
		return listed
	}
	return []string{alias}
}

// savedPassphrase は、その alias が使う鍵と、保存されているパスフレーズを返す。
//
// アカウントのパスワードはここに現れない。名前空間が別だからであり、混ぜれば、
// ローカルの鍵を開くための秘密がリモートへログインパスワードとして送られる。
func savedPassphrase(
	passwords *secret.Service,
	alias string,
	target func(string) (relativePath, promptPath, configSnapshot, evidence string, ok bool, err error),
) (string, string) {
	if passwords == nil || target == nil {
		return "", ""
	}
	relativePath, _, _, _, ok, err := target(alias)
	if err != nil || !ok {
		return "", ""
	}
	passphrase, found := passwords.KeyPassphraseFor(relativePath)
	if !found {
		return "", ""
	}
	return relativePath, passphrase
}

// OpenPath は、コマンドラインがブラウザへの入口を求める場所である。
//
// bootstrap トークンは初回使用時に消費され、別のトークンを出力できるのは新しい
// プロセスだけだった。これは、ユーザーがアプリケーションを起動して URL が印字
// される場合には問題ないが、標準出力がどこにも届かないバックグラウンドエージェント
// では役に立たない。その URL を——何にも守られていない場所にある生きた credential
// を——ログファイルへ書く代わりに、誰かが尋ねたときにここで発行する。
const OpenPath = "/cli/open"

type openResponse struct {
	URL string `json:"url"`
}

// HealthPath は、そこに我々のエンジンが居るかを確かめる場所である。
//
// **/api/v1/health ではない。** あちらは /api/ の下にあり、すべての /api/
// 要求は Sec-Fetch-Site: same-origin を要求する——ブラウザでないものは
// 通れない。ここが /cli/ にあるのはそのためであり、**handoff の秘密で
// 認証するので「何かが答えた」ではなく「我々のエンジンが答えた」と言える。**
// ポートが別のものに再利用されていたら、あちらはこの秘密を知らない。
const HealthPath = "/cli/engine/health"

// StopPath は、走っているエンジンへ終了を頼む場所である。
//
// /api/ の外にあるのは、セッションではなく handoff の秘密で認証するからで
// ある。**これを呼ぶのはデスクトップの外殻であって、画面ではない。**
const StopPath = "/cli/engine/stop"

func registerConnectRoutes(engine *echo.Echo, handlers ConnectHandlers) {
	engine.POST(ConnectPath, handlers.Connect)
	engine.POST(OpenPath, handlers.Open)
	engine.POST(StopPath, handlers.Stop)
	engine.POST(HealthPath, handlers.Health)
}

// Health は、我々のエンジンがここに居ることを答える。
//
// 運ぶのは「居る」という事実だけである。版も、設定も、施錠の状態も返さない
// ——**この経路が答えるのは、繋ぎ直してよい相手かどうかだけ**である。
func (h ConnectHandlers) Health(c *echo.Context) error {
	if !h.authorised(c.Request()) {
		return c.NoContent(http.StatusForbidden)
	}
	return c.NoContent(http.StatusNoContent)
}

// Stop は、この常駐を終わらせる。
//
// **答えてから止める。** 止めてから答えると、呼んだ側は成功と切断を区別
// できない。実際の停止は応答が出ていったあとに起きる。
func (h ConnectHandlers) Stop(c *echo.Context) error {
	if !h.authorised(c.Request()) {
		return c.NoContent(http.StatusForbidden)
	}
	if h.Shutdown == nil {
		return c.NoContent(http.StatusServiceUnavailable)
	}
	go h.Shutdown()
	return c.NoContent(http.StatusAccepted)
}

// Open は、セッションを確立する URL で応答する。
func (h ConnectHandlers) Open(c *echo.Context) error {
	if !h.authorised(c.Request()) {
		return c.NoContent(http.StatusForbidden)
	}
	if h.Sessions == nil || h.BaseURL == "" {
		return c.NoContent(http.StatusServiceUnavailable)
	}
	bootstrap, err := h.Sessions.Reissue()
	if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}
	return c.JSON(http.StatusOK, openResponse{URL: h.BaseURL + "/#bootstrap=" + bootstrap})
}

// authorised は、呼び出し側が handoff を推測ではなく読んだかどうかを報告する。
// あらゆる拒否は外から見て同じ形をしているので、secret を持たない呼び出し側は
// 何も知ることができない。
func (h ConnectHandlers) authorised(request *http.Request) bool {
	presented := request.Header.Get(handoff.HeaderName)
	return h.Secret != "" && len(presented) == len(h.Secret) &&
		subtle.ConstantTimeCompare([]byte(presented), []byte(h.Secret)) == 1
}

// Connect は、1 個の接続が必要とするものだけを返し、それより長生きするものは何も返さない。
//
// あらゆる拒否は外から見て同じ形をしているので、このエンドポイントを
// 使ってどの alias が存在するか、どれにパスワードがあるかを知ることはできない。
// secret を持たない呼び出し側は何も知ることができない。
func (h ConnectHandlers) Connect(c *echo.Context) error {
	request := c.Request()
	if request.Header.Get(echo.HeaderContentType) != "application/json" {
		return c.NoContent(http.StatusUnsupportedMediaType)
	}
	if !h.authorised(request) {
		return c.NoContent(http.StatusForbidden)
	}

	var decoded connectRequest
	if err := json.NewDecoder(io.LimitReader(request.Body, maxConnectBody)).Decode(&decoded); err != nil {
		return c.NoContent(http.StatusBadRequest)
	}
	if err := platform.ValidateAlias(decoded.Alias); err != nil {
		return c.NoContent(http.StatusBadRequest)
	}

	answer := connectResponse{Alias: decoded.Alias, Warnings: []string{}}
	if h.Warnings != nil {
		if warnings := h.Warnings(decoded.Alias); len(warnings) > 0 {
			answer.Warnings = warnings
		}
	}
	// 答えが返るのは、保存されているものがある場合だけである。それ以外
	// ——施錠された vault、保存が無い、鍵が一つに定まらない——では、
	// コマンドラインが自分で尋ねる接続になる。それは正常な接続である。
	answer.KeyPath, answer.Passphrase = savedPassphrase(
		h.Passwords, decoded.Alias, h.KeyPassphraseTarget)
	answer.Passwords = savedPasswords(h.Passwords, h.connectionAliases(decoded.Alias))
	return c.JSON(http.StatusOK, answer)
}
