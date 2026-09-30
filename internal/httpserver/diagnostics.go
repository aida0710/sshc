package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/config"
	"sshc/internal/diagnostics"
	"sshc/internal/effective"
	"sshc/internal/session"
	"sshc/internal/validate"
)

// DiagnosticsHandlers は、個別に起動されるチェック群を公開する。
type DiagnosticsHandlers struct {
	Service *diagnostics.Service
	Actions ActionHandlers
}

func registerDiagnosticsRoutes(engine *echo.Echo, handlers DiagnosticsHandlers) {
	engine.POST("/api/v1/diagnostics/config", handlers.CheckConfig)
	engine.POST("/api/v1/diagnostics/effective", handlers.Effective)
	engine.POST("/api/v1/diagnostics/reachability", handlers.Reachability)
	engine.POST("/api/v1/diagnostics/authentication", handlers.Authentication)
}

// addDiagnosticsActions は、このサブシステムが所有する確認を登録する。
//
// そのいずれもが、現時点での設定が持つ実行可能なディレクティブに結び付く。
// それこそが確認ダイアログの表示内容そのものだからである。したがって、確認と
// リクエストの間に編集が入ると、別のコマンドを暗黙に実行するのではなく、
// トークンが無効になる。
func addDiagnosticsActions(registry actionRegistry, service *diagnostics.Service) {
	evidence := func(_ context.Context, target string) (string, error) {
		if err := validate.Alias(target); err != nil {
			return "", err
		}
		report, err := service.Safety()
		if err != nil {
			return "", err
		}
		return report.Evidence(), nil
	}
	for _, kind := range []string{
		session.ActionReachability,
		session.ActionAuthentication,
	} {
		registry[kind] = actionKind{evidence: evidence, fail: diagnosticsProblem}
	}
}

// diagnosticsProblem は、evidence 導出の失敗を通信形式に対応付ける。
func diagnosticsProblem(c *echo.Context, err error) error {
	if errors.Is(err, validate.ErrUnsafeAlias) {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	return unexpectedProblem(c, "config_unreadable", err)
}

// CheckConfig は構文チェックと Include チェックを実行する。プロセスを
// 起動しないので action トークンは不要で、セッションと CSRF ヘッダーだけでよい。
func (h DiagnosticsHandlers) CheckConfig(c *echo.Context) error {
	report, err := h.Service.ConfigCheck()
	if err != nil {
		return unexpectedProblem(c, "config_unreadable", err)
	}

	response := api.ConfigCheckResponse{
		Root:        report.Root,
		Files:       make([]api.ConfigFileSummary, 0, len(report.Files)),
		Diagnostics: make([]api.ConfigDiagnostic, 0, len(report.Diagnostics)),
	}
	for _, file := range report.Files {
		response.Files = append(response.Files, api.ConfigFileSummary{
			Path: file.Path, Editable: file.Editable, Missing: file.Missing,
			Loads: file.Loads, Includes: file.Includes,
		})
	}
	for _, diagnostic := range report.Diagnostics {
		response.Diagnostics = append(response.Diagnostics, api.ConfigDiagnostic{
			Severity: severityName(diagnostic.Severity),
			Code:     diagnostic.Code,
			Path:     diagnostic.Path,
			Line:     diagnostic.Line,
			Detail:   diagnostic.Detail,
		})
	}
	return c.JSON(http.StatusOK, response)
}

// Effective は alias ひとつについて、エンジン自身の射影（値とその出所）、経路、
// 実行されうるディレクティブの一覧を返す。
//
// 設定を読むだけで何も実行しないので、action トークンによる確認は要らない。
// Match ブロックの値は評価せず、complexity として理由を返す。
func (h DiagnosticsHandlers) Effective(c *echo.Context) error {
	var request api.AliasRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := validate.Alias(request.Alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}

	inspection, err := h.Service.Inspect(request.Alias)
	if err != nil {
		return unexpectedProblem(c, "inspection_failed", err)
	}

	response := api.EffectiveResponse{
		Alias:                inspection.Alias,
		TokenWarning:         effective.TokenEscapeWarning,
		ExecutableDirectives: describeDirectives(inspection.Report.Directives),
		Sources:              make([]api.ValueSource, 0, len(inspection.Projection.Sources)),
		Complexities:         make([]api.ComplexityNote, 0, len(inspection.Projection.Complexities)),
		Route:                make([]api.JumpStage, 0, len(inspection.Route)),
	}
	for _, source := range inspection.Projection.Sources {
		response.Sources = append(response.Sources, api.ValueSource{
			Keyword: source.Keyword, Value: source.Value, Path: source.Path,
			Line: source.Line, Condition: source.Condition, Kind: source.Kind, Winner: source.Winner,
		})
	}
	for _, complexity := range append(inspection.Projection.Complexities, inspection.RouteComplexities...) {
		response.Complexities = append(response.Complexities, api.ComplexityNote{
			Code: complexity.Code, Path: complexity.Path, Line: complexity.Line,
			Condition: complexity.Condition, Detail: complexity.Detail,
		})
	}
	for _, stage := range inspection.Route {
		response.Route = append(response.Route, api.JumpStage{
			Order: stage.Order, Depth: stage.Depth, Parent: stage.Parent, Hop: stage.Hop.Raw,
			Hostname: stage.Hostname, User: stage.User, Port: stage.Port, Complex: stage.Complex,
		})
	}
	return c.JSON(http.StatusOK, response)
}

// Reachability は接続先へ直接ダイアルする。
func (h DiagnosticsHandlers) Reachability(c *echo.Context) error {
	var request api.AliasRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := validate.Alias(request.Alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	if allowed, response := h.Actions.consume(c, session.ActionReachability, request.Alias); !allowed {
		return response
	}

	result, err := h.Service.Reach(c.Request().Context(), request.Alias)
	switch {
	case errors.Is(err, diagnostics.ErrUnsafeDestination):
		return problem(c, http.StatusBadRequest, "unsafe_destination")
	case err != nil:
		// 設定や VPN の紐付けを読めなかったのは接続先の問題ではない。確認 token を発行する
		// ときの読み込みの失敗と同じく config_unreadable で返す。
		return diagnosticsProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.ReachabilityResponse{
		Address:   result.Address,
		Outcome:   result.Outcome,
		ElapsedMs: int(result.Elapsed.Milliseconds()),
		Detail:    result.Detail,
		Notice:    result.Notice,
	})
}

// Authentication は、上限付きの認証テストを実行する。
func (h DiagnosticsHandlers) Authentication(c *echo.Context) error {
	var request api.AuthenticationRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := validate.Alias(request.Alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	if allowed, response := h.Actions.consume(c, session.ActionAuthentication, request.Alias); !allowed {
		return response
	}

	result, err := h.Service.Authenticate(c.Request().Context(), request.Alias, request.AcknowledgeExecutable)
	var directiveError *diagnostics.ExecutableDirectiveError
	switch {
	case errors.As(err, &directiveError):
		return problem(c, http.StatusConflict, "executable_directive_not_acknowledged")
	case err != nil:
		return unexpectedProblem(c, "authentication_test_failed", err)
	}
	return c.JSON(http.StatusOK, api.AuthenticationResponse{
		Outcome:       result.Outcome,
		Authenticated: result.Authenticated,
		Method:        result.Method,
		Detail:        result.Detail,
		Truncated:     result.Truncated,
		ElapsedMs:     int(result.Elapsed.Milliseconds()),
	})
}

func describeDirectives(directives []effective.Executable) []api.ExecutableDirective {
	described := make([]api.ExecutableDirective, 0, len(directives))
	for _, directive := range directives {
		described = append(described, api.ExecutableDirective{
			Keyword:     directive.Keyword,
			Command:     directive.Command,
			Path:        directive.Path,
			Line:        directive.Line,
			Condition:   directive.Condition,
			OnEvaluate:  directive.OnEvaluate,
			OnConnect:   directive.OnConnect,
			Overridable: directive.Overridable,
		})
	}
	return described
}

func severityName(severity config.Severity) string {
	switch severity {
	case config.SeverityError:
		return "error"
	case config.SeverityWarning:
		return "warning"
	default:
		return "info"
	}
}
