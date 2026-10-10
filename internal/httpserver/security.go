package httpserver

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

const (
	SessionCookie     = "sshc_session"
	CSRFHeader        = "X-SSHC-CSRF"
	SessionContextKey = "sshc-session"

	// style-src だけが 'unsafe-inline' を持つ。
	//
	// 実測の結果である。xterm.js は文字の実寸を測ってから、その寸法を
	// 持つ規則を <style> 要素として差し込み、DOM レンダラーは各セルへ
	// setAttribute("style", …) を書く。nonce を渡す口は無い。したがって
	// 埋め込みターミナルを持つには、インラインのスタイルを許すしかない。
	//
	// xterm.js のために緩めたのはここだけである。script-src は 'self' のままで、
	// require-trusted-types-for 'script' もそのままである。xterm.js の配布物には
	// innerHTML も document.write も new Function も eval も無いので（5.5.0 と
	// 6.0.0 の両方で 0 件）、xterm.js のためにスクリプト側は一文字も緩めていない。
	// trusted-types に並べた policy の名前は、下の serviceWorkerTrustedTypesPolicy と
	// monacoTrustedTypesPolicies に理由を書いてある。
	//
	// 失ったもの: HTML を注入できる者は CSS も注入できる。得たもの: 端末が
	// 描画できる。前者に必要な注入点をこのアプリケーションは持たない。React が
	// エスケープし、dangerouslySetInnerHTML はどこにも無く、スクリプトは
	// 依然として止まる。docs/design.md の「更新の境界」にある CSP の記述にも
	// 同じことが書いてある。
	contentSecurityPolicy = "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; " +
		"trusted-types " + serviceWorkerTrustedTypesPolicy + " " + monacoTrustedTypesPolicies + "; require-trusted-types-for 'script'"

	// serviceWorkerTrustedTypesPolicy は、main.tsx が service worker の URL を
	// TrustedScriptURL にするために作る policy である。
	serviceWorkerTrustedTypesPolicy = "sshc-service-worker"

	// monacoTrustedTypesPolicies は、SFTP のエディタ（Monaco Editor）が作る
	// Trusted Types の policy の名前である。
	//
	// 許す理由: Monaco は行、折り返しの計測、差分などの HTML を自分で組み立てて
	// innerHTML へ入れる。require-trusted-types-for 'script' のもとでは、その値は
	// 名前を許した policy を通さないと止められ、エディタの本文が 1 行も描かれない。
	// dompurify は Monaco が同梱する DOMPurify の policy で、読み取り専用のときの
	// メッセージのような Markdown を表示するときに作られる。defaultWorkerFactory は
	// worker のスクリプトの URL を TrustedScriptURL にする policy で、Monaco の代わりに
	// web/src/sftp/monacoEnvironment.ts が同じ名前で 1 回だけ作る。Monaco の policy は
	// 値をそのまま通す。ファイルの本文は Monaco がエスケープしてから HTML にする
	// （web/e2e/sftp-editor.spec.ts が、HTML のような行が文字のまま描かれることを
	// 確かめる）。
	//
	// 安全な理由: require-trusted-types-for 'script' は保つので、policy を通さない
	// 文字列の代入は今までどおり止まる。ここで許すのは名前だけで、policy を作れるのは
	// この画面ですでにスクリプトを実行できる者に限られる。'allow-duplicates' は
	// 付けないので、先に作られた名前を、後からほかのスクリプトが作り直すことは
	// できない。作られた policy は、作ったモジュールの中にだけ置かれる。
	// MonacoEnvironment.createTrustedTypesPolicy も作った policy を保持せず、
	// ほかのスクリプトに渡さない。
	//
	// 一覧は Monaco が実際に作る名前だけにする。Monaco を上げて名前が増減すると、
	// 埋め込み UI（internal/ui/dist）と照合する
	// TestThePagePolicyAllowsExactlyTheTrustedTypesPoliciesTheEmbeddedUICreates が落ちる。
	monacoTrustedTypesPolicies = "defaultWorkerFactory diffEditorWidget diffReview domLineBreaksComputer dompurify " +
		"editorGhostText editorViewLayer richScreenReaderContent standaloneColorizer stickyScrollViewLayer tokenizeToString"

	// spaFallbackRoute は single-page application を配信するパターンである。
	// これにしかマッチしなかったリクエストは、どの API ルートにもマッチしなかったことになる。
	spaFallbackRoute = "/*"

	// MaxRequestBodyCeiling は、あらゆる /api/ リクエストの body を読む際の
	// 絶対的な上限である。各ハンドラは自前のより小さな制限を課すが、これが存在するのは、
	// 後で追加されたルートが忘れて無制限の body を読めないようにするためだ。
	MaxRequestBodyCeiling         = 2 << 20
	MaxSFTPUploadRangeBodyCeiling = int64(4 << 30)
	// JSON can encode one text byte as six bytes (\uXXXX); the envelope
	// must not reduce the editor's decoded 2 MiB limit.
	MaxSFTPTextJSONBodyCeiling = 6*sshcSFTP.MaxEditableFileBytes + (16 << 10)
)

func requestBodyCeiling(request *http.Request) int64 {
	textRoute := strings.TrimPrefix(request.URL.Path, "/api/v1/sftp/")
	if request.Method == http.MethodPut && textRoute != request.URL.Path && strings.Count(textRoute, "/") == 1 && strings.HasSuffix(textRoute, "/text") {
		return MaxSFTPTextJSONBodyCeiling
	}
	if request.Method == http.MethodPatch && strings.HasPrefix(request.URL.Path, "/api/v1/sftp/") &&
		strings.Contains(request.URL.Path, "/uploads/") && request.URL.Query().Get("range") == "true" {
		return MaxSFTPUploadRangeBodyCeiling
	}
	if request.Method == http.MethodPost && request.URL.Path == backgroundsRoute {
		return MaxBackgroundUploadBodyCeiling
	}
	return MaxRequestBodyCeiling
}

type Security struct {
	ExpectedHost   string
	ExpectedOrigin string
	Sessions       *session.Manager
	// Unlocked は vault のロック解除状態を返す。nil は安全側としてロック中と扱う。
	Unlocked func() bool
}

// gateExempt は、vault がロック中でも利用できる初期化・認証ルートを指定する。
// session/bootstrap と session/recover は gate より前に通すので、ここには無い。
// recover-compatible-backup と reset-unsupported は、別の版が書いた Vault を
// 開けずロックされたままのときにだけ意味を持つため、ロック中に通す必要がある。
// どちらも unlock と同じく master password を検証する。
func gateExempt(method, path string) bool {
	switch path {
	case "/api/v1/health", "/api/v1/update":
		return method == http.MethodGet
	case "/api/v1/session/renew":
		return method == http.MethodPost
	case "/api/v1/passwords":
		return method == http.MethodGet
	case "/api/v1/passwords/initialise", "/api/v1/passwords/unlock",
		"/api/v1/passwords/recover-compatible-backup", "/api/v1/passwords/reset-unsupported":
		return method == http.MethodPost
	}
	return false
}

func (s Security) Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		request := c.Request()
		isAPI := strings.HasPrefix(request.URL.Path, "/api/")
		setSecurityHeaders(c.Response().Header(), isAPI)

		if request.Host != s.ExpectedHost {
			return problem(c, http.StatusForbidden, "invalid_host")
		}

		if !isAPI {
			return next(c)
		}

		// Fetch Metadata はすべての API リクエストでチェックされ、状態を
		// 変えるものだけではない。cross-site の GET はすでに SameSite=Strict
		// によって cookie を奪われているが、design §8.1 はヘッダを完全一致で
		// 検証するよう求めている。SameSite の扱いを誤るブラウザが、他のサイト
		// とこの API の間に立つ唯一のものになってはならないからだ。
		if request.Header.Get("Sec-Fetch-Site") != "same-origin" {
			return problem(c, http.StatusForbidden, "cross_site_request")
		}
		// この上限はリクエストへの制限であり、ハンドラがたまたま読む量への
		// 制限にとどまらない。上限を超えると宣言された長さは、ハンドラが
		// 実行される前に拒否される。だから body を無視するルート。現状の
		// /diagnostics/config と /keys/:keyId/trash、そしてパスだけから
		// 入力を得る後日追加されるどんなルートも。無制限の body を渡されずに済む。
		bodyCeiling := requestBodyCeiling(request)
		if request.ContentLength > bodyCeiling {
			return problem(c, http.StatusRequestEntityTooLarge, "request_body_too_large")
		}
		// chunked リクエストは長さを宣言しないため、読み取るハンドラのために
		// reader 自体にも上限を設けておく必要がある。
		if request.Body != nil {
			request.Body = http.MaxBytesReader(c.Response(), request.Body, bodyCeiling)
		}

		isSessionEntry := request.Method == http.MethodPost &&
			(request.URL.Path == "/api/v1/session/bootstrap" || request.URL.Path == "/api/v1/session/recover")
		isStateChanging := request.Method != http.MethodGet && request.Method != http.MethodHead
		if (isStateChanging || isSessionEntry) && request.Header.Get(echo.HeaderOrigin) != s.ExpectedOrigin {
			return problem(c, http.StatusForbidden, "cross_site_request")
		}
		if isSessionEntry {
			return next(c)
		}

		cookie, err := request.Cookie(SessionCookie)
		if err != nil {
			return problem(c, http.StatusUnauthorized, "session_required")
		}
		if s.Sessions == nil {
			return problem(c, http.StatusUnauthorized, "invalid_session")
		}
		endRequest, authenticated := s.Sessions.BeginRequest(cookie.Value)
		if !authenticated {
			return problem(c, http.StatusUnauthorized, "invalid_session")
		}
		defer endRequest()
		// トークンは書き込みだけでなく読み取りにも必要である。cookie は
		// ポートに紐づかず site もそうなので、127.0.0.1 上の別のサーバーが
		// これを受け取ってしまう。SameSite は scheme と registrable domain
		// を比較し、IP はそれ自体が site のすべてだからだ。トークンは
		// port を含む origin に分離された sessionStorage にあり、別 port へは
		// 渡らないため、それを要求することで漏洩した cookie 単体を無価値にする。
		claimed := c.Path() != "" && c.Path() != spaFallbackRoute
		// Health はトークンからもゲートからも除外されている。これが運ぶ
		// のはバージョン文字列だけであり、ページが落ち着く前になされる
		// 唯一のリクエストでもあるため、ここでトークンを要求すれば、得る
		// ものなく bootstrap の順序の罠にはまるだけになる。
		isHealth := request.Method == http.MethodGet && request.URL.Path == "/api/v1/health"
		if (isStateChanging || claimed) && !isHealth &&
			!s.Sessions.VerifyCSRF(cookie.Value, request.Header.Get(CSRFHeader)) {
			return problem(c, http.StatusForbidden, "invalid_csrf")
		}

		// 未登録パスは router に 404 を返させ、vault_locked と区別する。
		if claimed && !gateExempt(request.Method, request.URL.Path) && (s.Unlocked == nil || !s.Unlocked()) {
			return problem(c, http.StatusConflict, "vault_locked")
		}

		c.Set(SessionContextKey, cookie.Value)
		return next(c)
	}
}

func setSecurityHeaders(header http.Header, apiResponse bool) {
	header.Set("Content-Security-Policy", contentSecurityPolicy)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	if apiResponse {
		header.Set("Cache-Control", "no-store")
	}
}

func problem(c *echo.Context, status int, code string) error {
	c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
	return c.JSON(status, api.Problem{Code: code, Message: "request rejected"})
}

// problemReply は、problem 応答の status、code、説明である。いくつかの対応付けが同じ形で
// 返すときに、組み立てと書き出しを分けるために使う。
type problemReply struct {
	status int
	code   string
	// detail は problemDetail と同じ決まりに従う固定文である。空なら説明を付けない。
	detail string
}

func writeProblemReply(c *echo.Context, reply problemReply) error {
	if reply.detail == "" {
		return problem(c, reply.status, reply.code)
	}
	return problemDetail(c, reply.status, reply.code, reply.detail)
}

// problemDetailLimit は、problem の detail に載せる文の上限である。detail は内部のエラーの
// 文で、長さに上限が無いと応答が際限なく大きくなる。
const problemDetailLimit = 512

func boundedProblemDetail(detail string) string {
	if len(detail) > problemDetailLimit {
		return detail[:problemDetailLimit]
	}
	return detail
}

// problemDetail は上限のある説明付きで拒否を返す。
//
// 呼び出し元は、固定文字列か、platform 層がすでに無害化した
// メッセージのどちらかを渡さなければならない。detail に鍵材料、
// パスフレーズ、セッションやアクショントークン、絶対パスを含めてはならない。
func problemDetail(c *echo.Context, status int, code, detail string) error {
	detail = boundedProblemDetail(detail)
	c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
	return c.JSON(status, api.Problem{Code: code, Message: "request rejected", Detail: &detail})
}
