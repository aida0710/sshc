package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/session"
	"sshc/internal/validate"
)

// VaultHandlers は、ブラウザから Vault を扱うルートを持つ。
//
// /api/v1/passwords では次を扱う。
//   - Vault の状態の取得、作成、ロックの解除、ロック
//   - マスターパスワードの変更
//   - 別のバージョンが書いた Vault を開けないときの、互換backupからの復旧と作り直し
//   - 接続先ごとにパスワードを保存できるかの確認
//
// /api/v1/credentials では、Vault に保存したパスワード、鍵のパスフレーズ、TOTP を扱う。
//
// パスは /api/v1/passwords のままにしている。扱うのはパスワードに限らないが、名前を
// そろえる利点が、web・openapi・生成物をまとめて直す費用に見合わないためである。
// CLI から Vault を扱うルート（/cli/vault/）は、vault_cli.go の registerVaultCLIRoutes が
// CLIHandlers に登録する。
type VaultHandlers struct {
	Service *secret.Service
	Actions ActionHandlers
	Now     func() time.Time
	// KeyHosts projects saved key subjects through the current SSH configuration.
	// It returns relationships only; the vault values never cross this boundary.
	KeyHosts func(relativePaths []string) (map[string][]string, error)
	// Eligibility は alias と保存されたパスワードの間に何があるかを返す。
	// これが注入されているのは、結果が設定グラフと known_hosts から
	// 来るためで、そのどちらについても vault は何も知らない。nil なら
	// 何も確かめない。
	Eligibility func(alias string) (application.PasswordEligibility, error)
	// Binding returns the resolved authentication destination that a stored
	// account password is allowed to reach.
	Binding func(alias string) (string, error)
}

var errVaultUnavailable = errors.New("vault service unavailable")

// withVault は service で operation を行う。Vault を組み込まずに組んだ engine
// （service が nil）では errVaultUnavailable を返す。
func withVault(service *secret.Service, operation func(*secret.Service) error) error {
	if service == nil {
		return errVaultUnavailable
	}
	return operation(service)
}

func registerVaultRoutes(engine *echo.Echo, handlers VaultHandlers) {
	engine.GET("/api/v1/passwords", handlers.Status)
	engine.POST("/api/v1/passwords/initialise", handlers.Initialise)
	engine.POST("/api/v1/passwords/unlock", handlers.Unlock)
	engine.POST("/api/v1/passwords/recover-compatible-backup", handlers.RecoverCompatibleBackup)
	engine.POST("/api/v1/passwords/reset-unsupported", handlers.ResetUnsupported)
	engine.POST("/api/v1/passwords/change", handlers.Change)
	engine.POST("/api/v1/passwords/lock", handlers.Lock)
	engine.GET("/api/v1/passwords/:alias/eligibility", handlers.Eligible)
	engine.GET("/api/v1/credentials", handlers.ListCredentials)
	engine.PUT("/api/v1/credentials/:kind/assign", handlers.AssignCredential)
	engine.DELETE("/api/v1/credentials/:kind/assign/:subject", handlers.UnassignCredential)
	engine.PUT("/api/v1/credentials/:kind/:name", handlers.SetCredential)
	engine.PATCH("/api/v1/credentials/:kind/:name", handlers.UpdateCredential)
	engine.POST("/api/v1/credentials/:kind/:name/reveal", handlers.RevealCredential)
	engine.POST("/api/v1/credentials/totp/:name/codes", handlers.GenerateTOTPCodes)
	engine.DELETE("/api/v1/credentials/:kind/:name", handlers.DeleteCredential)
}

// status は Vault の状態を返す。/api/v1/passwords の状態の取得、作成、ロックの解除、
// ロック、互換backupからの復旧と作り直しは、成功するとこれを返す。
// どの接続先がパスワードを持つかを運び、パスワードそのものは運ばない。
func (h VaultHandlers) status(c *echo.Context) error {
	var state secret.State
	err := withVault(h.Service, func(vault *secret.Service) (err error) {
		state, err = vault.State()
		return err
	})
	if err != nil {
		return unexpectedProblem(c, "vault_unreadable", err)
	}
	minimum := secret.MinPassphraseLength
	aliases := h.Service.Aliases()
	if aliases == nil {
		aliases = []string{}
	}
	dedicatedKeyPassphrases := h.Service.DedicatedKeyPassphrases()
	if dedicatedKeyPassphrases == nil {
		dedicatedKeyPassphrases = []string{}
	}
	answer := api.PasswordVaultStatus{
		Passwordless:            &state.Passwordless,
		Exists:                  state.Exists,
		Unlocked:                state.Unlocked,
		Aliases:                 aliases,
		DedicatedKeyPassphrases: dedicatedKeyPassphrases,
		MinPassphraseLength:     &minimum,
	}
	if state.LastMigration.Applied() {
		answer.MigratedFromVersion = &state.LastMigration.From
		answer.MigratedToVersion = &state.LastMigration.To
	}
	return c.JSON(http.StatusOK, answer)
}

func (h VaultHandlers) Status(c *echo.Context) error { return h.status(c) }

func (h VaultHandlers) Initialise(c *echo.Context) error {
	var request api.PassphraseRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := withVault(h.Service, func(vault *secret.Service) error {
		return vault.Initialise(request.Passphrase)
	}); err != nil {
		return vaultProblem(c, err)
	}
	return h.status(c)
}

func (h VaultHandlers) Unlock(c *echo.Context) error {
	var request api.PassphraseRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := withVault(h.Service, func(vault *secret.Service) error {
		return vault.Unlock(request.Passphrase)
	}); err != nil {
		return vaultProblem(c, err)
	}
	return h.status(c)
}

func (h VaultHandlers) RecoverCompatibleBackup(c *echo.Context) error {
	var request api.PassphraseRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := withVault(h.Service, func(vault *secret.Service) error {
		return vault.RecoverCompatibleBackup(request.Passphrase)
	}); err != nil {
		return vaultProblem(c, err)
	}
	return h.status(c)
}

func (h VaultHandlers) ResetUnsupported(c *echo.Context) error {
	var request api.ResetUnsupportedVaultRequest
	if err := decodeJSON(c, &request); err != nil || !request.Acknowledged {
		return problem(c, http.StatusBadRequest, "vault_reset_acknowledgement_required")
	}
	if err := withVault(h.Service, func(vault *secret.Service) error {
		return vault.ResetUnsupported(request.Passphrase)
	}); err != nil {
		return vaultProblem(c, err)
	}
	return h.status(c)
}

// Change はマスターパスワードを変更し、ローカルのvault、同期設定、世代backupを
// 再封印する。remote snapshotは専用の同期鍵で暗号化されるため変更しない。
func (h VaultHandlers) Change(c *echo.Context) error {
	var request api.ChangeMasterPasswordRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := withVault(h.Service, func(vault *secret.Service) error {
		return vault.ChangeMasterPassword(c.Request().Context(), request.Current, request.Next)
	}); err != nil {
		return vaultProblem(c, err)
	}

	answer := api.ChangeMasterPasswordResult{}

	var state secret.State
	err := withVault(h.Service, func(vault *secret.Service) (err error) {
		state, err = vault.State()
		return err
	})
	if err != nil {
		return unexpectedProblem(c, "vault_unreadable", err)
	}
	minimum := secret.MinPassphraseLength
	aliases := h.Service.Aliases()
	if aliases == nil {
		aliases = []string{}
	}
	dedicatedKeyPassphrases := h.Service.DedicatedKeyPassphrases()
	if dedicatedKeyPassphrases == nil {
		dedicatedKeyPassphrases = []string{}
	}
	answer.Vault = api.PasswordVaultStatus{
		Passwordless: &state.Passwordless, Exists: state.Exists, Unlocked: state.Unlocked, Aliases: aliases,
		DedicatedKeyPassphrases: dedicatedKeyPassphrases, MinPassphraseLength: &minimum,
	}
	return c.JSON(http.StatusOK, answer)
}

func (h VaultHandlers) Lock(c *echo.Context) error {
	if err := withVault(h.Service, (*secret.Service).LockManually); err != nil {
		return vaultProblem(c, err)
	}
	return h.status(c)
}

// Eligible は alias と保存されたパスワードの間に何があるかを報告する。
func (h VaultHandlers) Eligible(c *echo.Context) error {
	alias := c.Param("alias")
	if err := validate.Alias(alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	var answer api.PasswordEligibility
	if h.Eligibility == nil {
		answer = api.PasswordEligibility{
			Alias: alias, Storable: true,
			Blockers: []api.Notice{}, Warnings: []api.Notice{},
		}
	} else {
		report, err := h.Eligibility(alias)
		if err != nil {
			return unexpectedProblem(c, "config_unreadable", err)
		}
		answer = describeEligibility(report)
	}
	if h.Service != nil && h.Binding != nil {
		binding, err := h.Binding(alias)
		if err != nil {
			return unexpectedProblem(c, "config_unreadable", err)
		}
		password, totp := h.Service.AuthenticationBindingStates(alias, binding)
		passwordState := api.PasswordEligibilityPasswordBinding(password)
		totpState := api.PasswordEligibilityTotpBinding(totp)
		answer.PasswordBinding = &passwordState
		answer.TotpBinding = &totpState
	}
	return c.JSON(http.StatusOK, answer)
}

func describeEligibility(report application.PasswordEligibility) api.PasswordEligibility {
	described := api.PasswordEligibility{
		Alias:    report.Alias,
		Storable: report.Storable,
		Blockers: make([]api.Notice, 0, len(report.Blockers)),
		Warnings: make([]api.Notice, 0, len(report.Warnings)),
	}
	for _, notice := range report.Blockers {
		described.Blockers = append(described.Blockers, eligibilityNotice(notice))
	}
	for _, notice := range report.Warnings {
		described.Warnings = append(described.Warnings, eligibilityNotice(notice))
	}
	if report.HostName != "" {
		host := report.HostName
		described.HostName = &host
	}
	if report.Port != "" {
		port := report.Port
		described.Port = &port
	}
	if binding := api.PasswordEligibilityPasswordBinding(report.PasswordBinding); binding.Valid() {
		described.PasswordBinding = &binding
	}
	if binding := api.PasswordEligibilityTotpBinding(report.TOTPBinding); binding.Valid() {
		described.TotpBinding = &binding
	}
	return described
}

func eligibilityNotice(notice application.Notice) api.Notice {
	described := api.Notice{Code: notice.Code}
	if notice.Path != "" {
		path := notice.Path
		described.Path = &path
	}
	if notice.Line != 0 {
		line := notice.Line
		described.Line = &line
	}
	if notice.Detail != "" {
		detail := notice.Detail
		described.Detail = &detail
	}
	return described
}

// kindOf はパスから namespace を読み取る。未知の namespace は既定値にせず
// ここで拒否する。既定値にすると、typo が暗黙に namespace を選ぶことになるからだ。
func kindOf(c *echo.Context) (secret.Kind, bool) {
	kind := secret.Kind(c.Param("kind"))
	return kind, secret.ValidKind(kind)
}

// credentialProblem は vault の拒否を画面が扱える応答に変換する。
func credentialProblem(c *echo.Context, err error, uses []string) error {
	switch {
	case errors.Is(err, secret.ErrLocked):
		return problem(c, http.StatusConflict, "vault_locked")
	case errors.Is(err, secret.ErrCredentialInUse):
		return problemWith(c, http.StatusConflict, problemPayload{Code: "credential_in_use", Blockers: uses})
	case errors.Is(err, secret.ErrUnknownCredential):
		return problem(c, http.StatusNotFound, "unknown_credential")
	case errors.Is(err, secret.ErrCredentialAlreadyExists):
		return problem(c, http.StatusConflict, "credential_already_exists")
	case errors.Is(err, secret.ErrUnsafeName), errors.Is(err, secret.ErrEmptySecret),
		errors.Is(err, secret.ErrUnknownKind), errors.Is(err, secret.ErrInvalidTOTP):
		return problem(c, http.StatusBadRequest, "invalid_request")
	default:
		return unexpectedProblem(c, "vault_failed", err)
	}
}

// listCredentials は名前とそれを使うものを返す。値は決して返さ
// ない。secret を読める画面は、乗っ取られたブラウザがそこから
// 読み取れる画面でもあり、選択には名前だけあれば十分だからだ。
func (h VaultHandlers) listCredentials(c *echo.Context) error {
	listed, err := h.Service.Credentials()
	if err != nil {
		return credentialProblem(c, err, nil)
	}
	dedicated := h.Service.DedicatedKeyPassphrases()
	keySet := map[string]bool{}
	for _, uses := range listed[secret.KindKeyPassphrase] {
		for _, key := range uses {
			keySet[key] = true
		}
	}
	for _, key := range dedicated {
		keySet[key] = true
	}
	keyPaths := make([]string, 0, len(keySet))
	for key := range keySet {
		keyPaths = append(keyPaths, key)
	}
	sort.Strings(keyPaths)

	hostsByKey := map[string][]string{}
	keyHostUsageComplete := true
	if len(keyPaths) > 0 {
		keyHostUsageComplete = h.KeyHosts != nil
		if h.KeyHosts != nil {
			projected, projectionErr := h.KeyHosts(keyPaths)
			if projectionErr != nil {
				keyHostUsageComplete = false
			} else {
				hostsByKey = projected
			}
		}
	}

	answer := api.CredentialList{
		Credentials:             []api.Credential{},
		DedicatedKeyPassphrases: []api.DedicatedKeyPassphraseUsage{},
		KeyHostUsageComplete:    keyHostUsageComplete,
	}
	for _, kind := range []secret.Kind{secret.KindPassword, secret.KindKeyPassphrase, secret.KindTOTP} {
		names := make([]string, 0, len(listed[kind]))
		for name := range listed[kind] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			uses := listed[kind][name]
			hosts := append([]string{}, uses...)
			if kind == secret.KindKeyPassphrase {
				hosts = joinedKeyHosts(uses, hostsByKey)
			}
			answer.Credentials = append(answer.Credentials, api.Credential{
				Kind: api.CredentialKind(kind), Name: name, Uses: uses, Hosts: hosts,
			})
		}
	}
	for _, key := range dedicated {
		answer.DedicatedKeyPassphrases = append(answer.DedicatedKeyPassphrases, api.DedicatedKeyPassphraseUsage{
			Key: key, Hosts: append([]string{}, hostsByKey[key]...),
		})
	}
	return c.JSON(http.StatusOK, answer)
}

func joinedKeyHosts(keys []string, hostsByKey map[string][]string) []string {
	set := map[string]bool{}
	for _, key := range keys {
		for _, host := range hostsByKey[key] {
			set[host] = true
		}
	}
	hosts := make([]string, 0, len(set))
	for host := range set {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

func (h VaultHandlers) ListCredentials(c *echo.Context) error {
	return h.listCredentials(c)
}

func (h VaultHandlers) SetCredential(c *echo.Context) error {
	kind, ok := kindOf(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	var request api.StoreCredentialRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Service.SetCredential(kind, c.Param("name"), request.Secret); err != nil {
		return credentialProblem(c, err, nil)
	}
	return h.listCredentials(c)
}

func (h VaultHandlers) UpdateCredential(c *echo.Context) error {
	kind, ok := kindOf(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	var request api.UpdateCredentialRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Service.UpdateCredential(kind, c.Param("name"), request.Name, request.Secret); err != nil {
		return credentialProblem(c, err, nil)
	}
	return h.listCredentials(c)
}

func (h VaultHandlers) RevealCredential(c *echo.Context) error {
	kind, ok := kindOf(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	name := c.Param("name")
	if allowed, response := h.Actions.consume(c, session.ActionRevealCredential, credentialActionTarget(kind, name)); !allowed {
		return response
	}
	value, err := h.Service.Credential(kind, name)
	if err != nil {
		return credentialProblem(c, err, nil)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, api.RevealCredentialResponse{
		Kind: string(kind), Name: name, Secret: value,
	})
}

// GenerateTOTPCodes returns only the short-lived adjacent codes. The
// provisioning secret remains inside the engine and is never serialized into
// either the browser or CLI response.
func (h VaultHandlers) GenerateTOTPCodes(c *echo.Context) error {
	name := c.Param("name")
	target := credentialActionTarget(secret.KindTOTP, name)
	if allowed, response := h.Actions.consume(c, session.ActionRevealCredential, target); !allowed {
		return response
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	codes, err := h.Service.TOTPCodes(name, now)
	if err != nil {
		return credentialProblem(c, err, nil)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, api.TOTPCodeSet{
		Previous: codes.Previous, Current: codes.Current, Next: codes.Next,
		PeriodSeconds: codes.PeriodSeconds, RemainingSeconds: codes.RemainingSeconds,
	})
}

func credentialActionTarget(kind secret.Kind, name string) string {
	return string(kind) + "\n" + name
}

func addCredentialActions(registry actionRegistry, service *secret.Service) {
	registry[session.ActionRevealCredential] = actionKind{
		evidence: func(_ context.Context, target string) (string, error) {
			kindText, name, ok := strings.Cut(target, "\n")
			kind := secret.Kind(kindText)
			if !ok || !secret.ValidKind(kind) || name == "" {
				return "", secret.ErrUnknownCredential
			}
			return service.CredentialEvidence(kind, name)
		},
		fail: func(c *echo.Context, err error) error { return credentialProblem(c, err, nil) },
	}
}

func (h VaultHandlers) DeleteCredential(c *echo.Context) error {
	kind, ok := kindOf(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	name := c.Param("name")
	if err := h.Service.DeleteCredential(kind, name); err != nil {
		uses, _ := h.Service.Credentials()
		return credentialProblem(c, err, uses[kind][name])
	}
	return h.listCredentials(c)
}

func (h VaultHandlers) AssignCredential(c *echo.Context) error {
	kind, ok := kindOf(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	var request api.AssignCredentialRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if kind == secret.KindPassword || kind == secret.KindTOTP {
		if kind == secret.KindPassword {
			if blocked, response := h.ensurePasswordStorable(c, request.Subject); blocked {
				return response
			}
		}
		binding, resolved, response := h.passwordBinding(c, request.Subject)
		if !resolved {
			return response
		}
		assignment := secret.BoundAssignment{Kind: kind, Subject: request.Subject, Name: request.Name, Binding: binding}
		if err := h.Service.AssignBoundCredential(assignment); err != nil {
			return credentialProblem(c, err, nil)
		}
		return h.listCredentials(c)
	}
	if err := h.Service.AssignCredential(kind, request.Subject, request.Name); err != nil {
		return credentialProblem(c, err, nil)
	}
	return h.listCredentials(c)
}

func (h VaultHandlers) UnassignCredential(c *echo.Context) error {
	kind, ok := kindOf(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Service.UnassignCredential(kind, c.Param("subject")); err != nil {
		return credentialProblem(c, err, nil)
	}
	return h.listCredentials(c)
}

// passwordBinding は割り当てを結び付ける接続先の binding を求める。真偽値が
// false のときは拒否のレスポンスを書き込み済みなので、呼び出し側はすぐ戻る。
func (h VaultHandlers) passwordBinding(c *echo.Context, alias string) (string, bool, error) {
	if h.Binding == nil {
		return "", false, unexpectedProblem(c, "config_unreadable", nil)
	}
	binding, err := h.Binding(alias)
	if err != nil {
		return "", false, unexpectedProblem(c, "config_unreadable", err)
	}
	return binding, true, nil
}

// ensurePasswordStorable guards every route that creates a password-to-host
// relationship. Removal stays available even when current config blocks use.
func (h VaultHandlers) ensurePasswordStorable(c *echo.Context, alias string) (bool, error) {
	if h.Eligibility == nil {
		return false, nil
	}
	report, err := h.Eligibility(alias)
	if err != nil {
		return true, unexpectedProblem(c, "config_unreadable", err)
	}
	if report.Storable {
		return false, nil
	}
	blockers := make([]string, 0, len(report.Blockers))
	for _, notice := range report.Blockers {
		blockers = append(blockers, notice.Code)
	}
	return true, problemWith(c, http.StatusConflict, problemPayload{
		Code: "password_not_storable", Blockers: blockers,
	})
}

// vaultProblem は、Vault（secret.Service）が返したエラーを、画面が見分けられる
// status と code に変換する。
func vaultProblem(c *echo.Context, err error) error {
	if refusal, ok := boundaryRefusalFor(err); ok {
		return writeProblemReply(c, refusal)
	}
	var schema *secret.SchemaVersionError
	var migration *secret.MigrationError
	switch {
	case errors.Is(err, secret.ErrAlreadyExists):
		return problem(c, http.StatusConflict, "vault_already_exists")
	case errors.Is(err, secret.ErrNoVault):
		return problem(c, http.StatusNotFound, "vault_missing")
	case errors.As(err, &migration) && errors.Is(err, secret.ErrMigrationFailed):
		return problemWith(c, http.StatusConflict, problemPayload{
			Code: "vault_migration_failed", Detail: fmt.Sprintf("vault migration from schema %d to %d failed before the original vault was replaced", migration.From, migration.To),
			CurrentVersion: &migration.From, RequiredVersion: &migration.To,
		})
	case errors.Is(err, secret.ErrWrongPassphrase):
		return problem(c, http.StatusForbidden, "wrong_passphrase")
	case errors.As(err, &schema) && errors.Is(err, secret.ErrOlderSchema):
		return problemWith(c, http.StatusConflict, problemPayload{
			Code: "vault_schema_older", Detail: fmt.Sprintf("vault schema %d is older than supported schema %d", schema.Found, schema.Supported),
			CurrentVersion: &schema.Found, RequiredVersion: &schema.Supported,
		})
	case errors.As(err, &schema) && errors.Is(err, secret.ErrNewerSchema):
		return problemWith(c, http.StatusConflict, problemPayload{
			Code: "vault_schema_newer", Detail: fmt.Sprintf("vault schema %d is newer than supported schema %d", schema.Found, schema.Supported),
			CurrentVersion: &schema.Found, RequiredVersion: &schema.Supported,
		})
	case errors.Is(err, secret.ErrUnsupportedVersion):
		return problemDetail(c, http.StatusConflict, "vault_envelope_unsupported",
			"the encrypted vault envelope version is not supported")
	case errors.Is(err, secret.ErrNoCompatibleBackup):
		return problem(c, http.StatusNotFound, "vault_compatible_backup_missing")
	case errors.Is(err, secret.ErrRecoveryNotNeeded):
		return problem(c, http.StatusConflict, "vault_recovery_not_needed")
	case errors.Is(err, secret.ErrCostRefused):
		return problem(c, http.StatusConflict, "vault_cost_refused")
	case errors.Is(err, secret.ErrWeakPassphrase):
		return problem(c, http.StatusBadRequest, "passphrase_too_short")
	case errors.Is(err, secret.ErrEmptySecret):
		return problem(c, http.StatusBadRequest, "password_empty")
	case errors.Is(err, secret.ErrUnsafeName):
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return unexpectedReply(c, problemReply{
			status: http.StatusInternalServerError, code: "vault_storage_permission_denied",
			detail: "the operating system denied access to the app's private storage",
		}, err)
	case errors.Is(err, syscall.ENOSPC):
		return problemDetail(c, http.StatusInsufficientStorage, "vault_storage_full", "the app's private storage does not have enough free space")
	case errors.Is(err, syscall.EROFS):
		return unexpectedReply(c, problemReply{
			status: http.StatusInternalServerError, code: "vault_storage_read_only",
			detail: "the app's private storage is read-only",
		}, err)
	case errors.Is(err, syscall.EIO):
		return unexpectedReply(c, problemReply{
			status: http.StatusInternalServerError, code: "vault_storage_io_failed",
			detail: "the operating system reported an input/output failure while accessing private storage",
		}, err)
	default:
		return unexpectedReply(c, problemReply{
			status: http.StatusInternalServerError, code: "vault_write_failed",
			detail: "the encrypted vault could not be committed to app storage",
		}, err)
	}
}
