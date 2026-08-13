package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/config"
	"sshc/internal/diagnostics"
	"sshc/internal/effective"
	"sshc/internal/platform"
	"sshc/internal/session"
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
// リクエストの間に編集が入ると、別のコマンドを黙って実行するのではなく、
// トークンが無効になる。
func addDiagnosticsActions(registry actionRegistry, service *diagnostics.Service) {
	evidence := func(target string) (string, error) {
		if err := platform.ValidateAlias(target); err != nil {
			return "", err
		}
		report, err := service.Safety()
		if err != nil {
			return "", err
		}
		return report.Evidence(), nil
	}
	for _, kind := range []string{
		session.ActionEvaluate,
		session.ActionReachability,
		session.ActionAuthentication,
	} {
		registry[kind] = actionKind{evidence: evidence, fail: diagnosticsProblem}
	}
}

// diagnosticsProblem は、evidence 導出の失敗を通信形式に対応付ける。
func diagnosticsProblem(c *echo.Context, err error) error {
	if errors.Is(err, platform.ErrUnsafeAlias) {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	return problem(c, http.StatusInternalServerError, "config_unreadable")
}

// CheckConfig は構文チェックと Include チェックを実行する。プロセスを
// 起動しないので action トークンは不要で、セッションと CSRF ヘッダーだけでよい。
func (h DiagnosticsHandlers) CheckConfig(c *echo.Context) error {
	report, err := h.Service.ConfigCheck()
	if err != nil {
		return problem(c, http.StatusInternalServerError, "config_unreadable")
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

// Effective は 1 個の alias を説明し、許されている場合はそれを評価する。
//
// action トークンが必要になるのは、評価がコマンドを実行する場合だけである。
// それはまさに設定が Match exec を持つ場合に一致する。
func (h DiagnosticsHandlers) Effective(c *echo.Context) error {
	var request api.AliasRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := platform.ValidateAlias(request.Alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}

	confirmed := false
	if c.Request().Header.Get(ActionHeader) != "" {
		allowed, response := h.Actions.consume(c, session.ActionEvaluate, request.Alias)
		if !allowed {
			return response
		}
		confirmed = true
	}

	inspection, err := h.Service.Inspect(c.Request().Context(), request.Alias, confirmed)
	if err != nil {
		return problem(c, http.StatusInternalServerError, "inspection_failed")
	}

	response := api.EffectiveResponse{
		Alias:                inspection.Alias,
		Evaluated:            inspection.Evaluated,
		RequiresConfirmation: inspection.RequiresConfirmation,
		TokenWarning:         effective.TokenEscapeWarning,
		ExecutableDirectives: describeDirectives(inspection.Report.Directives),
		Values:               make([]api.EffectiveValue, 0, len(inspection.Values.Keywords)),
		Sources:              make([]api.ValueSource, 0, len(inspection.Projection.Sources)),
		Complexities:         make([]api.ComplexityNote, 0, len(inspection.Projection.Complexities)),
		Route:                make([]api.JumpStage, 0, len(inspection.Route)),
		Failure:              api.OpenSSHFailure{},
	}
	for _, keyword := range inspection.Values.Keywords {
		response.Values = append(response.Values, api.EffectiveValue{
			Keyword: keyword,
			Values:  inspection.Values.All(keyword),
		})
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
	if inspection.Failure != nil {
		response.Failure = api.OpenSSHFailure{
			Failed:    true,
			ExitCode:  inspection.Failure.ExitCode,
			Stderr:    inspection.Failure.Stderr,
			Truncated: inspection.Failure.Truncated,
		}
	}
	return c.JSON(http.StatusOK, response)
}

// Reachability は接続先へ直接ダイアルする。
func (h DiagnosticsHandlers) Reachability(c *echo.Context) error {
	var request api.AliasRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := platform.ValidateAlias(request.Alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	if allowed, response := h.Actions.consume(c, session.ActionReachability, request.Alias); !allowed {
		return response
	}

	result, err := h.Service.Reach(c.Request().Context(), request.Alias)
	if err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_destination")
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
	if err := platform.ValidateAlias(request.Alias); err != nil {
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
		return problem(c, http.StatusInternalServerError, "authentication_test_failed")
	}
	return c.JSON(http.StatusOK, api.AuthenticationResponse{
		Outcome:       result.Outcome,
		Authenticated: result.Authenticated,
		ExitCode:      result.ExitCode,
		Stderr:        result.Stderr,
		Truncated:     result.Truncated,
		ElapsedMs:     int(result.Elapsed.Milliseconds()),
	})
}

// TerminalCommand は、alias に対するコマンドテキストを返す。
//
// 埋め込みターミナルができたあともこれが残っているのは、自分の端末で開きたい人が
// いるからである。何も起動しないので確認も要らない。

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
