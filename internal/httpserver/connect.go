package httpserver

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/handoff"
	"sshc/internal/secret"
	"sshc/internal/session"
	"sshc/internal/terminal"
	"sshc/internal/validate"
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

// ConnectHandlers は `sshc ssh <alias>` に応答する。
type ConnectHandlers struct {
	// Secret は呼び出し側が提示すべきものである。空であればすべての
	// リクエストを拒否する。handoff を書けなかったサーバーは受け付けてはならない。
	Secret string
	// Passwords は保存済みの鍵パスフレーズとアカウントパスワードを持つ。nil で
	// あれば保存された結果は一切提供されず、それはプロンプトが出る正常な接続である。
	Passwords *secret.Service
	vault     *vaultOperations
	// WorkspaceKeys は、その alias が使うワークスペース内の秘密鍵を返す。
	// 解決できない設定では空を返す。推測しない。
	WorkspaceKeys func(alias string) (relativePaths []string, err error)
	// Warnings は、OpenSSH がこの host に対して実行するディレクティブを報告する。
	// 接続の最中に気付くのではなく、事前に伝えられる。
	Warnings func(alias string) []string
	// Aliases は、この接続に現れる alias を、ProxyJump の手前も含めて返す。
	// nil なら行き先ひとつだけを見る。
	Aliases func(alias string) []string
	// PasswordBinding resolves the authentication destination digest for each
	// alias in the connection chain.
	PasswordBinding func(alias string) (string, error)
	// Bootstrap はブラウザ用 URL を生成し、BaseURL はその接続先を返す。
	// 両方が nil であれば、このアプリケーションはコマンドラインから開けない。
	// これは session manager を持たないビルドの状態である。
	//
	// かつては Sessions という名だった。その名は下の、実行中コンソール
	// の本数を返す field に譲った。`sshc status` が尋ねるのは本数であり、
	// *session.Manager そのものではないので、そちらのほうが呼び出し側に近い名である。
	Bootstrap *session.Manager
	BaseURL   string
	// Sessions は、実行中コンソールの本数を返す。nil なら 0。
	Sessions func() int
	// Owner、Version、ProtocolVersion は handoff を読んだ CLI が、応答元を
	// 自分が見つけた engine と照合するための値である。
	Owner           handoff.Owner
	Version         string
	ProtocolVersion int
	// StopEngine は engine の停止を要求する。nil の場合は停止 API を登録しない。
	//
	// 信号ではなく、頼みごとにする。Windows に SIGTERM は無く、
	// TerminateProcess は即死である。開いている端末も転送も vault も畳まれない
	// まま消える。自分自身に頼めば、どの OS でも同じ畳み方を通る。
	StopEngine func()
}

type connectRequest struct {
	Alias string `json:"alias"`
}

// connectResponse は、`sshc <接続先>` が接続に使うものである。
//
// 単回トークンではなく結果そのものを返す。トークンにしていたのは、
// 引き換える相手が OpenSSH の起動する別のプログラムだったからである。要求を
// 出したユーザー本人が結果を受け取るなら、発行と引き換えを分ける理由が無い。
// localhost を通るものは変わっていない。いままでもパスフレーズはこの経路を
// 通っており、間に立つプログラムがひとつ消えただけである。
type connectResponse struct {
	Alias string `json:"alias"`
	// Passphrases は、この接続に現れる鍵ごとの保存済みパスフレーズである。
	// キーはワークスペース相対の表記で、保管庫が知っているのがその形である。
	//
	// 行き先ひとつではなく連鎖ぶんである。ProxyJump の手前に立つホストも
	// それ自身が alias であり、そこにも別の鍵が指定されうる。行き先のぶんだけを
	// 渡すと、手前で止まる接続がそのたびに手入力を求める。アカウントパスワードの
	// 側は最初からそうしており、パスフレーズだけが行き先 1 件だった。
	Passphrases map[string]string `json:"passphrases,omitempty"`
	// Passwords は、この接続に現れる alias ごとの保存済みアカウントパスワード。
	//
	// 行き先ひとつではなく連鎖ぶんである。ProxyJump の手前に立つホストは
	// それ自身が alias であり、そこにもパスワードは保存されうる。行き先のぶん
	// だけを渡すと、手前で止まる接続がそのたびに手入力を求める。
	//
	// Passphrase とは別の名前空間である。あちらはローカルの秘密鍵を開く
	// ための秘密で、こちらはリモートのアカウントへログインするための秘密である。
	// 混ぜれば、鍵を開くための秘密がそのままリモートへ送られる。
	Passwords        map[string]string `json:"passwords,omitempty"`
	PasswordBindings map[string]string `json:"passwordBindings,omitempty"`
	Warnings         []string          `json:"warnings"`
}

// savedPassword は、その alias について保存されているアカウントパスワードを返す。
//
// これが載るのは、この経路がもう外部のプログラムへ渡さないからである。
// askpass だった頃は、結果を受け取るのが OpenSSH の起動する別のプログラムだった。
// 現在は要求元の `sshc` が資格情報を受け取り、プロセス内で SSH 接続を行う。渡す先が
// 増えないなら、埋め込みターミナルと違う結果を返す理由も無い。
//
// この経路を読めるのは `~/.ssh/sshc/cli`（0600）を読める者だけであり、その者は
// すでに、どの alias についても保存済みパスフレーズを引き出せる。秘密が一種類
// 増えることは書いておく。境界は動かないが、動かないことは自明ではない。
// 返すのはこの接続に現れる alias のぶんだけである。保管庫を一覧にはしない
// 尋ねられた接続に要るものと、要らないものを区別する。
func savedPasswords(
	passwords *secret.Service,
	aliases []string,
	binding func(alias string) (string, error),
) (map[string]string, map[string]string) {
	if passwords == nil || binding == nil {
		return nil, nil
	}
	found := map[string]string{}
	bindings := map[string]string{}
	for _, alias := range aliases {
		current, err := binding(alias)
		if err != nil {
			continue
		}
		if password := passwords.BoundPasswordFor(alias, current); password != "" {
			found[alias] = password
			bindings[alias] = current
		}
	}
	if len(found) == 0 {
		return nil, nil
	}
	return found, bindings
}

// connectionAliases は、この接続に現れる alias を返す。行き先と、ProxyJump の
// 手前に立つホストである。連鎖を解決できなければ行き先だけを返す。解決の失敗は
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

// savedPassphrases は、この接続に現れる鍵ごとの保存済みパスフレーズを返す。
//
// アカウントのパスワードはここに現れない。名前空間が別だからであり、混ぜれば、
// ローカルの鍵を開くための秘密がリモートへログインパスワードとして送られる。
//
// 連鎖ぶんを見る。手前に立つホストが別の鍵を指定していれば、その鍵の結果も
// 要る。そうでないと、行き先には届く接続が手前で止まって手入力を求める。
// savedPasswords がしていることと同じである。
func savedPassphrases(
	passwords *secret.Service,
	aliases []string,
	workspaceKeys func(alias string) ([]string, error),
) map[string]string {
	if passwords == nil || workspaceKeys == nil {
		return nil
	}
	found := map[string]string{}
	for _, alias := range aliases {
		paths, err := workspaceKeys(alias)
		if err != nil {
			continue
		}
		for _, path := range paths {
			if _, seen := found[path]; seen {
				continue
			}
			if passphrase, ok := passwords.KeyPassphraseFor(path); ok && passphrase != "" {
				found[path] = passphrase
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	return found
}

// OpenPath は、CLI が単回使用のブラウザ URL を取得するエンドポイント。
// URL をログへ永続化せず、handoff secret で認証された要求ごとに発行する。
const OpenPath = "/cli/open"

type openResponse struct {
	URL string `json:"url"`
}

// StatusPath は、走っている engine に「いまどうなっているか」を尋ねる場所である。
//
// これは CLI 専用のエンドポイントであり、Web UI は自身の session を使う。
// ここが返す相手は `sshc status` であり、認可は handoff の秘密ひとつである。
const StatusPath = "/cli/status"

// StopPath は、走っている engine に「畳んで終わってくれ」と頼む場所である。
//
// どこで起動したか分からない engine を、止められる必要がある。tmux の中か、
// 閉じた端末か、supervisor の下か。探して回るより、走っているものに頼む方が
// 短い。認可は handoff の秘密ひとつで、それを読めるのはこの計算機の利用者だけ
// である。
const StopPath = "/cli/stop"

type CLIStatus struct {
	Owner           handoff.Owner `json:"owner"`
	Version         string        `json:"version"`
	ProtocolVersion int           `json:"protocolVersion"`
	// Vault は vault が作成済みかを示す。
	//
	// 「ロックされている」と「保管庫が無い」は別の状態である。Unlocked は
	// どちらでも false になるので、これが無いと、保管庫を一度も作っていない
	// 未作成の vault に対してパスワード入力を求めないため、この値を分ける。
	Vault bool `json:"vault"`
	// Unlocked は vault が開いているか。
	Unlocked bool `json:"unlocked"`
	// Sessions は実行中コンソールの本数。終了済みは数えない。
	// 「閉じてよいか」を問うための数だからである。
	Sessions int `json:"sessions"`
}

// liveSessions は、まだ終わっていないものだけを数える。終了済みは registry に
// 残っていても数えない。この数は「閉じてよいか」を問うためのものだからである。
func liveSessions(views []terminal.View) int {
	live := 0
	for _, view := range views {
		if view.Exited == nil {
			live++
		}
	}
	return live
}

func registerConnectRoutes(engine *echo.Echo, handlers ConnectHandlers) {
	if handlers.vault == nil {
		handlers.vault = newVaultOperations(handlers.Passwords)
	}
	engine.POST(ConnectPath, handlers.Connect)
	engine.POST(OpenPath, handlers.Open)
	engine.GET(StatusPath, handlers.Status)
	engine.POST(StopPath, handlers.Stop)
	registerVaultCLIRoutes(engine, handlers)
}

// Stop は HTTP 応答を返したあとで engine の停止を要求する。
func (h ConnectHandlers) Stop(c *echo.Context) error {
	if !h.authorised(c.Request()) {
		return c.NoContent(http.StatusForbidden)
	}
	if h.StopEngine == nil {
		return c.NoContent(http.StatusNotImplemented)
	}
	stop := h.StopEngine
	c.Response().Header().Set("Connection", "close")
	if err := c.NoContent(http.StatusAccepted); err != nil {
		return err
	}
	go stop()
	return nil
}

// Status は `sshc status` と停止確認に使う engine の現在状態を返す。
// 停止は実行中の端末と転送も終了するため、明示的な要求だけを受け付ける。
func (h ConnectHandlers) Status(c *echo.Context) error {
	if !h.authorised(c.Request()) {
		return c.NoContent(http.StatusForbidden)
	}
	answer, err := h.cliStatus()
	if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}
	return c.JSON(http.StatusOK, answer)
}

func (h ConnectHandlers) cliStatus() (CLIStatus, error) {
	answer := CLIStatus{
		Owner: h.Owner, Version: h.Version, ProtocolVersion: h.ProtocolVersion,
	}
	state, err := h.vault.State()
	if err != nil {
		return CLIStatus{}, err
	}
	answer.Vault, answer.Unlocked = state.Exists, state.Unlocked
	if h.Sessions != nil {
		answer.Sessions = h.Sessions()
	}
	return answer, nil
}

// Open は、セッションを確立する URL で応答する。
func (h ConnectHandlers) Open(c *echo.Context) error {
	if !h.authorised(c.Request()) {
		return c.NoContent(http.StatusForbidden)
	}
	if h.Bootstrap == nil || h.BaseURL == "" {
		return c.NoContent(http.StatusServiceUnavailable)
	}
	bootstrap, err := h.Bootstrap.Reissue()
	if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}
	return c.JSON(http.StatusOK, openResponse{URL: h.BaseURL + "/#bootstrap=" + bootstrap})
}

// authorised は、呼び出し側が handoff を推測ではなく読んだかどうかを報告する。
// あらゆる拒否は外から見て同じ形をしているので、secret を持たない呼び出し側は
// 何も知ることができない。
func (h ConnectHandlers) authorised(request *http.Request) bool {
	return cliAuthorised(request, h.Secret)
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
	if err := validate.Alias(decoded.Alias); err != nil {
		return c.NoContent(http.StatusBadRequest)
	}

	answer := connectResponse{Alias: decoded.Alias, Warnings: []string{}}
	if h.Warnings != nil {
		if warnings := h.Warnings(decoded.Alias); len(warnings) > 0 {
			answer.Warnings = warnings
		}
	}
	// 保存済みの値だけを返す。vault がロック中、値が未保存、または設定を解決
	// できない場合は、CLI が対話的に入力を求める。
	//
	// 鍵もパスワードも、同じ連鎖を見る。
	aliases := h.connectionAliases(decoded.Alias)
	answer.Passphrases = savedPassphrases(h.Passwords, aliases, h.WorkspaceKeys)
	answer.Passwords, answer.PasswordBindings = savedPasswords(h.Passwords, aliases, h.PasswordBinding)
	return c.JSON(http.StatusOK, answer)
}
