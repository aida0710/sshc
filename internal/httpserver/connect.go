package httpserver

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

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

// CLIHandlers は、`sshc` のコマンドが engine へ話す /cli/ のルート（接続、状態、停止、
// ブラウザを開く URL、CLI セッション、Vault の操作）に応答する。
type CLIHandlers struct {
	// Secret は呼び出し側が提示すべきものである。空であればすべての
	// リクエストを拒否する。handoff を書けなかったサーバーは受け付けてはならない。
	Secret string
	// Vault は保存済みの鍵パスフレーズ、アカウントパスワード、TOTP を持つ。nil で
	// あれば保存された結果は一切提供されず、それはプロンプトが出る正常な接続である。
	Vault *secret.Service
	// WorkspaceKeys は、その alias が使うワークスペース内の秘密鍵を返す。
	// 解決できない設定では空を返す。推測しない。
	WorkspaceKeys func(alias string) (relativePaths []string, err error)
	// Warnings は、OpenSSH がこの host に対して実行するディレクティブを報告する。
	// 接続の最中に気付くのではなく、事前に伝えられる。
	Warnings func(alias string) []string
	// Aliases は、この接続に現れる alias を、ProxyJump の手前も含めて返す。
	// nil なら行き先ひとつだけを見る。
	Aliases func(alias string) []string
	// RouteBindings は、alias への接続に現れる alias ごとの認証先の digest
	// （sshclient.Target.AuthenticationBinding）を、その接続が組み立てるとおりに返す。
	// ProxyJump の踏み台はホップとしての値で、単独で繋ぐときの値とは違いうる（ホップは
	// VPN を通らない）。埋め込みターミナルが照合するのも同じ値である。nil なら保存済みの
	// パスワードと TOTP を返さない。
	RouteBindings func(alias string) (map[string]string, error)
	// Bootstrap はブラウザ用 URL を生成し、BaseURL はその接続先を返す。
	// 両方が nil であれば、このアプリケーションはコマンドラインから開けない。
	// これは session manager を持たないビルドの状態である。
	Bootstrap *session.Manager
	BaseURL   string
	// LiveTerminalCount は、終了していないターミナルの本数を返す。nil なら 0。
	LiveTerminalCount func() int
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

// connectResponse は、`sshc ssh <alias>` が接続に使うものである。
//
// 単回トークンではなく結果そのものを返す。結果を受け取るのは要求を出した
// CLI 自身なので、発行と引き換えを分ける理由が無い。
type connectResponse struct {
	Alias string `json:"alias"`
	// Passphrases は、この接続に現れる鍵ごとの保存済みパスフレーズである。
	// キーはワークスペース相対の表記で、保管庫が知っているのがその形である。
	//
	// 行き先ひとつではなく連鎖ぶんである。ProxyJump の手前に立つホストも
	// それ自身が alias であり、そこにも別の鍵が指定されうる。行き先のぶんだけを
	// 渡すと、手前で止まる接続がそのたびに手入力を求める。
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
	// StalePasswords は、値は存在するが現在の認証経路には解放できなかった alias。
	// 秘密そのものは含めず、CLI が突然の prompt の理由を説明するためだけに使う。
	StalePasswords []string `json:"stalePasswords,omitempty"`
	// TOTPs contains provisioning data only for explicitly assigned hosts on
	// the resolved route. The CLI generates a fresh code when challenged.
	TOTPs        map[string]string `json:"totps,omitempty"`
	TOTPBindings map[string]string `json:"totpBindings,omitempty"`
	StaleTOTPs   []string          `json:"staleTotps,omitempty"`
	Warnings     []string          `json:"warnings"`
}

// authenticationBindings は、alias への接続に現れる alias ごとの認証先の digest を返す。
//
// パスワードと TOTP は同じ map を使うので、接続ごとに一度だけ解く。種類ごとに
// 解き直すと、そのあいだに設定が変わったとき、2 つが別の経路に結び付いた値として
// 返る。接続を組み立てられなければ空で、秘密は返らず、CLI が入力を求める（接続
// そのものも同じ理由で失敗する）。保管庫が無ければ返す秘密も無いので、設定を解かない。
func (h CLIHandlers) authenticationBindings(alias string) map[string]string {
	if h.Vault == nil || h.RouteBindings == nil {
		return nil
	}
	bindings, err := h.RouteBindings(alias)
	if err != nil {
		return nil
	}
	return bindings
}

// savedBoundSecrets は、この接続に現れる alias ごとに、その kind（アカウントパスワード
// か TOTP）の保存済みの値を返す。2つ目の戻り値は値を結び付けた認証先、3つ目は
// 割り当てはあるが今の認証先には渡せなかった alias である。bindings は
// authenticationBindings が解いたもの。
//
// 要求元の `sshc` は受け取った値をプロセス内の SSH 接続で使い、別のプログラムへは
// 渡さない。渡す先が増えないので、埋め込みターミナルと違う結果を返す理由も無い。
//
// この経路を読めるのは `~/.ssh/sshc/cli`（0600）を読める者だけであり、その者は
// すでに、どの alias についても保存済みパスフレーズを引き出せる。秘密が一種類
// 増えることは書いておく。境界は動かないが、動かないことは自明ではない。
// 返すのはこの接続に現れる alias のぶんだけで、Vault を一覧にはしない。
// 尋ねられた接続に要るものと、要らないものを区別する。
func (h CLIHandlers) savedBoundSecrets(
	kind secret.Kind,
	aliases []string,
	bindings map[string]string,
) (map[string]string, map[string]string, []string) {
	if h.Vault == nil {
		return nil, nil, nil
	}
	found := map[string]string{}
	foundBindings := map[string]string{}
	var stale []string
	for _, alias := range aliases {
		current, resolved := bindings[alias]
		if !resolved {
			continue
		}
		if value := h.Vault.BoundFor(kind, alias, current); value != "" {
			found[alias] = value
			foundBindings[alias] = current
		} else if h.Vault.HasAssignmentFor(kind, alias) {
			stale = append(stale, alias)
		}
	}
	if len(found) == 0 {
		return nil, nil, stale
	}
	return found, foundBindings, stale
}

// connectionAliases は、この接続に現れる alias を返す。行き先と、ProxyJump の
// 手前に立つホストである。連鎖を解決できなければ行き先だけを返す。解決の失敗は
// このあとの接続そのものが報告するので、ここで二度言わない。
func (h CLIHandlers) connectionAliases(alias string) []string {
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
// savedBoundSecrets がしていることと同じである。
func savedPassphrases(
	vault *secret.Service,
	aliases []string,
	workspaceKeys func(alias string) ([]string, error),
) map[string]string {
	if vault == nil || workspaceKeys == nil {
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
			if passphrase, ok := vault.KeyPassphraseFor(path); ok && passphrase != "" {
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

// ChallengePath は、CLI が秘密を送る前に「この engine は handoff の秘密を持って
// いるか」を確かめる場所である。認可は要らない。答えは challenge に固有の HMAC で、
// 秘密そのものは明かさない。
const ChallengePath = "/cli/challenge"

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
	Unlocked     bool `json:"unlocked"`
	Passwordless bool `json:"passwordless"`
	// Sessions は終了していないターミナルの本数。終了済みは数えない。
	// 「閉じてよいか」を問うための数だからである。
	Sessions int `json:"sessions"`
}

// countLiveTerminals は、まだ終わっていないターミナルだけを数える。終了済みは registry に
// 残っていても数えない。この数は「閉じてよいか」を問うためのものだからである。
func countLiveTerminals(views []terminal.View) int {
	live := 0
	for _, view := range views {
		if view.Exited == nil {
			live++
		}
	}
	return live
}

// cliPrefix は、handoff の秘密で認証する CLI 向けルートの共通の頭である。
const cliPrefix = "/cli"

// registerCLIRoutes は /cli/ のルートを登録する。
//
// Challenge 以外は、handoff の秘密を確かめる group に置く。ルートを足した人が
// handler で検査を書き忘れても、秘密なしでは handler に届かない。Challenge は、
// CLI が秘密を見せる前に相手の engine が秘密を持っているかを確かめるためのものなので、
// 秘密を求めない。group の検査は e.Use の stoppingGate より後に動くので、停止中は
// 秘密を確かめる前に 503 を返す順序も変わらない。
func registerCLIRoutes(engine *echo.Echo, handlers CLIHandlers) {
	engine.GET(ChallengePath, handlers.Challenge)
	authenticated := engine.Group(cliPrefix, handlers.requireHandoffSecret)
	authenticated.POST(cliRoute(ConnectPath), handlers.Connect)
	authenticated.POST(cliRoute(OpenPath), handlers.Open)
	authenticated.POST(cliRoute(CLISessionPath), handlers.CLISession)
	authenticated.DELETE(cliRoute(CLISessionPath), handlers.RevokeCLISession)
	authenticated.GET(cliRoute(StatusPath), handlers.Status)
	authenticated.POST(cliRoute(StopPath), handlers.Stop)
	registerVaultCLIRoutes(authenticated, handlers)
}

// cliRoute は、/cli/ の完全なパスを group の中の相対パスにする。
func cliRoute(path string) string {
	return strings.TrimPrefix(path, cliPrefix)
}

// cliAuthorised は同じ長さの handoff secret を constant-time で比較する。
func cliAuthorised(request *http.Request, expected string) bool {
	presented := request.Header.Get(handoff.HeaderName)
	return expected != "" && len(presented) == len(expected) &&
		subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) == 1
}

// requireHandoffSecret は、handoff の秘密を示さない要求を handler へ届けずに 401 で断る。
// あらゆる拒否は外から見て同じ形をしているので、secret を持たない呼び出し側は
// どのルートがあるかも、どの alias が存在するかも知ることができない。
func (h CLIHandlers) requireHandoffSecret(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if !cliAuthorised(c.Request(), h.Secret) {
			return c.NoContent(http.StatusUnauthorized)
		}
		return next(c)
	}
}

// Stop は HTTP 応答を返したあとで engine の停止を要求する。
func (h CLIHandlers) Stop(c *echo.Context) error {
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
func (h CLIHandlers) Status(c *echo.Context) error {
	answer, err := h.cliStatus()
	if err != nil {
		return unexpectedProblem(c, "vault_unreadable", err)
	}
	return c.JSON(http.StatusOK, answer)
}

func (h CLIHandlers) cliStatus() (CLIStatus, error) {
	answer := CLIStatus{
		Owner: h.Owner, Version: h.Version, ProtocolVersion: h.ProtocolVersion,
	}
	var state secret.State
	err := withVault(h.Vault, func(vault *secret.Service) (err error) {
		state, err = vault.State()
		return err
	})
	if err != nil {
		return CLIStatus{}, err
	}
	answer.Vault, answer.Unlocked = state.Exists, state.Unlocked
	answer.Passwordless = state.Passwordless
	if h.LiveTerminalCount != nil {
		answer.Sessions = h.LiveTerminalCount()
	}
	return answer, nil
}

// Open は、セッションを確立する URL で応答する。
func (h CLIHandlers) Open(c *echo.Context) error {
	if h.Bootstrap == nil || h.BaseURL == "" {
		return problem(c, http.StatusServiceUnavailable, "bootstrap_unavailable")
	}
	bootstrap, err := h.Bootstrap.Reissue()
	if err != nil {
		return unexpectedProblem(c, "bootstrap_failed", err)
	}
	return c.JSON(http.StatusOK, openResponse{URL: h.BaseURL + "/#bootstrap=" + bootstrap})
}

// Challenge は、CLI の乱数に handoff の秘密で署名して返す。秘密を書けなかった
// engine（Secret が空）は何も証明できないので断る。
func (h CLIHandlers) Challenge(c *echo.Context) error {
	challenge := c.Request().Header.Get(handoff.ChallengeHeader)
	if h.Secret == "" || !handoff.ValidChallenge(challenge) {
		return c.NoContent(http.StatusBadRequest)
	}
	c.Response().Header().Set(handoff.ProofHeader, handoff.Prove(h.Secret, challenge))
	return c.NoContent(http.StatusNoContent)
}

// Connect は、1 個の接続が必要とするものだけを返し、それより長生きするものは何も返さない。
//
// secret を持たない呼び出し側は何も知ることができない（requireHandoffSecret）。
// secret を示した呼び出し側への断りは、要求の形の誤りだけを理由の code で伝える。
// 未知の alias も、パスワードの無い alias も断らないので、このエンドポイントを
// 使ってどの alias が存在するか、どれにパスワードがあるかを知ることはできない。
func (h CLIHandlers) Connect(c *echo.Context) error {
	request := c.Request()
	if request.Header.Get(echo.HeaderContentType) != "application/json" {
		return problem(c, http.StatusUnsupportedMediaType, "unsupported_media_type")
	}

	var decoded connectRequest
	if err := decodeJSONWithin(c, maxConnectBody, &decoded); err != nil {
		if errors.Is(err, errBodyTooLarge) {
			return problem(c, http.StatusRequestEntityTooLarge, "request_body_too_large")
		}
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := validate.Alias(decoded.Alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
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
	answer.Passphrases = savedPassphrases(h.Vault, aliases, h.WorkspaceKeys)
	bindings := h.authenticationBindings(decoded.Alias)
	answer.Passwords, answer.PasswordBindings, answer.StalePasswords =
		h.savedBoundSecrets(secret.KindPassword, aliases, bindings)
	answer.TOTPs, answer.TOTPBindings, answer.StaleTOTPs =
		h.savedBoundSecrets(secret.KindTOTP, aliases, bindings)
	return c.JSON(http.StatusOK, answer)
}
