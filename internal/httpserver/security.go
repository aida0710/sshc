package httpserver

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/session"
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
	// 緩めたのはここだけである。script-src は 'self' のままで、
	// require-trusted-types-for 'script' もそのままである。xterm.js の配布物には
	// innerHTML も document.write も new Function も eval も無いので（5.5.0 と
	// 6.0.0 の両方で 0 件）、スクリプト側は一文字も緩めずに済んでいる。
	//
	// 失ったもの: HTML を注入できる者は CSS も注入できる。得たもの: 端末が
	// 描画できる。前者に必要な注入点をこのアプリケーションは持たない。React が
	// エスケープし、dangerouslySetInnerHTML はどこにも無く、スクリプトは
	// 依然として止まる。README の「SSH 実行の境界」に同じことが書いてある。
	contentSecurityPolicy = "default-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; trusted-types sshc-service-worker; require-trusted-types-for 'script'"

	// spaFallbackRoute は single-page application を配信するパターンである。
	// これにしかマッチしなかったリクエストは、どの API ルートにもマッチしなかったことになる。
	spaFallbackRoute = "/*"

	// MaxRequestBodyCeiling は、あらゆる /api/ リクエストの body を読む際の
	// 絶対的な上限である。各ハンドラは自前のより小さな制限を課すが、これが存在するのは、
	// 後で追加されたルートが忘れて無制限の body を読めないようにするためだ。
	MaxRequestBodyCeiling         = 2 << 20
	MaxSFTPUploadRangeBodyCeiling = int64(4 << 30)
)

func requestBodyCeiling(request *http.Request) int64 {
	if request.Method == http.MethodPatch && strings.HasPrefix(request.URL.Path, "/api/v1/sftp/") &&
		strings.Contains(request.URL.Path, "/uploads/") && request.URL.Query().Get("range") == "true" {
		return MaxSFTPUploadRangeBodyCeiling
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
func gateExempt(method, path string) bool {
	switch path {
	case "/api/v1/health":
		return method == http.MethodGet
	case "/api/v1/session/bootstrap", "/api/v1/session/recover", "/api/v1/session/renew":
		return method == http.MethodPost
	case "/api/v1/passwords":
		return method == http.MethodGet
	case "/api/v1/passwords/initialise", "/api/v1/passwords/unlock":
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
		if !s.Sessions.Authenticate(cookie.Value) {
			return problem(c, http.StatusUnauthorized, "invalid_session")
		}
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

// problemDetail は上限のある説明付きで拒否を返す。
//
// 呼び出し元は、固定文字列か、platform 層がすでに無害化した
// メッセージのどちらかを渡さなければならない。detail に鍵材料、
// パスフレーズ、セッションやアクショントークン、絶対パスを含めてはならない。
func problemDetail(c *echo.Context, status int, code, detail string) error {
	const detailLimit = 512
	if len(detail) > detailLimit {
		detail = detail[:detailLimit]
	}
	c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
	return c.JSON(status, api.Problem{Code: code, Message: "request rejected", Detail: &detail})
}
