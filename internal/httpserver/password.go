package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/session"
	"sshc/internal/validate"
)

// PasswordHandlers は vault とヘルパーを提供する。
type PasswordHandlers struct {
	Service *secret.Service
	vault   *vaultOperations
	Actions ActionHandlers
	// KeyHosts projects saved key subjects through the current SSH configuration.
	// It returns relationships only; the vault values never cross this boundary.
	KeyHosts func(relativePaths []string) (map[string][]string, error)
	// Eligibility は alias と保存されたパスワードの間に何があるかを返す。
	// これが注入されているのは、結果が設定グラフと known_hosts から
	// 来るためで、そのどちらについても vault は何も知らない。nil の
	// 関数は何もチェックしないことを意味し、これはこの仕組みができる
	// 前に vault がしていたことである。
	Eligibility func(alias string) (application.PasswordEligibility, error)
	// Binding returns the resolved authentication destination that a stored
	// account password is allowed to reach.
	Binding func(alias string) (string, error)
}

var errVaultUnavailable = errors.New("vault service unavailable")

// vaultOperations は browser と CLI が共有する vault operation の順序境界である。
// local rekeyの全transactionを一つとして扱い、次世代のchangeやlockが
// vault/settings/backupsの再封印途中へ入らないようにする。
type vaultOperations struct {
	mu      sync.Mutex
	service *secret.Service
}

func newVaultOperations(service *secret.Service) *vaultOperations {
	return &vaultOperations{service: service}
}

func (v *vaultOperations) State() (secret.State, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.service == nil {
		return secret.State{}, errVaultUnavailable
	}
	return v.service.State()
}

func (v *vaultOperations) Initialise(passphrase string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.service == nil {
		return errVaultUnavailable
	}
	return v.service.Initialise(passphrase)
}

func (v *vaultOperations) Unlock(passphrase string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.service == nil {
		return errVaultUnavailable
	}
	return v.service.Unlock(passphrase)
}

func (v *vaultOperations) RecoverCompatibleBackup(passphrase string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.service == nil {
		return errVaultUnavailable
	}
	return v.service.RecoverCompatibleBackup(passphrase)
}

func (v *vaultOperations) ResetUnsupported(passphrase string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.service == nil {
		return errVaultUnavailable
	}
	return v.service.ResetUnsupported(passphrase)
}

func (v *vaultOperations) Lock() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.service == nil {
		return errVaultUnavailable
	}
	v.service.Lock()
	return nil
}

func (v *vaultOperations) Change(
	ctx context.Context,
	current string,
	next string,
) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if v.service == nil {
		return errVaultUnavailable
	}
	state, err := v.service.State()
	if err != nil {
		return err
	}
	if !state.Unlocked {
		return secret.ErrLocked
	}
	return v.service.ChangeMasterPassword(current, next)
}

func registerPasswordRoutes(engine *echo.Echo, handlers PasswordHandlers) {
	if handlers.vault == nil {
		handlers.vault = newVaultOperations(handlers.Service)
	}
	engine.GET("/api/v1/passwords", handlers.Status)
	engine.POST("/api/v1/passwords/initialise", handlers.Initialise)
	engine.POST("/api/v1/passwords/unlock", handlers.Unlock)
	engine.POST("/api/v1/passwords/recover-compatible-backup", handlers.RecoverCompatibleBackup)
	engine.POST("/api/v1/passwords/reset-unsupported", handlers.ResetUnsupported)
	engine.POST("/api/v1/passwords/change", handlers.Change)
	engine.POST("/api/v1/passwords/lock", handlers.Lock)
	engine.GET("/api/v1/passwords/:alias/eligibility", handlers.Eligible)
	engine.PUT("/api/v1/passwords/:alias", handlers.Store)
	engine.DELETE("/api/v1/passwords/:alias", handlers.Forget)
	engine.GET("/api/v1/credentials", handlers.ListCredentials)
	engine.PUT("/api/v1/credentials/:kind/assign", handlers.AssignCredential)
	engine.DELETE("/api/v1/credentials/:kind/assign/:subject", handlers.UnassignCredential)
	engine.PUT("/api/v1/credentials/:kind/:name", handlers.SetCredential)
	engine.PATCH("/api/v1/credentials/:kind/:name", handlers.UpdateCredential)
	engine.POST("/api/v1/credentials/:kind/:name/reveal", handlers.RevealCredential)
	engine.DELETE("/api/v1/credentials/:kind/:name", handlers.DeleteCredential)
}

// status はこのファイルの全ルートが返す唯一のレスポンスである。
// どのホストがパスワードを持つかを運び、パスワードそのものは運ばない。
func (h PasswordHandlers) status(c *echo.Context) error {
	state, err := h.vault.State()
	if err != nil {
		return problem(c, http.StatusInternalServerError, "vault_unreadable")
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

func (h PasswordHandlers) Status(c *echo.Context) error { return h.status(c) }

func (h PasswordHandlers) Initialise(c *echo.Context) error {
	var request api.PassphraseRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.vault.Initialise(request.Passphrase); err != nil {
		return passwordProblem(c, err)
	}
	return h.status(c)
}

func (h PasswordHandlers) Unlock(c *echo.Context) error {
	var request api.PassphraseRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.vault.Unlock(request.Passphrase); err != nil {
		return passwordProblem(c, err)
	}
	return h.status(c)
}

func (h PasswordHandlers) RecoverCompatibleBackup(c *echo.Context) error {
	var request api.PassphraseRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.vault.RecoverCompatibleBackup(request.Passphrase); err != nil {
		return passwordProblem(c, err)
	}
	return h.status(c)
}

func (h PasswordHandlers) ResetUnsupported(c *echo.Context) error {
	var request api.ResetUnsupportedVaultRequest
	if err := decodeJSON(c, &request); err != nil || !request.Acknowledged {
		return problem(c, http.StatusBadRequest, "vault_reset_acknowledgement_required")
	}
	if err := h.vault.ResetUnsupported(request.Passphrase); err != nil {
		return passwordProblem(c, err)
	}
	return h.status(c)
}

// Change はマスターパスワードを変更し、ローカルのvault、同期設定、世代backupを
// 再封印する。remote snapshotは専用の同期鍵で暗号化されるため変更しない。
func (h PasswordHandlers) Change(c *echo.Context) error {
	var request api.ChangeMasterPasswordRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.vault.Change(c.Request().Context(), request.Current, request.Next); err != nil {
		return passwordProblem(c, err)
	}

	answer := api.ChangeMasterPasswordResult{}

	state, err := h.vault.State()
	if err != nil {
		return problem(c, http.StatusInternalServerError, "vault_unreadable")
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
		Exists: state.Exists, Unlocked: state.Unlocked, Aliases: aliases,
		DedicatedKeyPassphrases: dedicatedKeyPassphrases, MinPassphraseLength: &minimum,
	}
	return c.JSON(http.StatusOK, answer)
}

func (h PasswordHandlers) Lock(c *echo.Context) error {
	if err := h.vault.Lock(); err != nil {
		return passwordProblem(c, err)
	}
	return h.status(c)
}

// Eligible は alias と保存されたパスワードの間に何があるかを報告する。
func (h PasswordHandlers) Eligible(c *echo.Context) error {
	alias := c.Param("alias")
	if err := validate.Alias(alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	if h.Eligibility == nil {
		return c.JSON(http.StatusOK, api.PasswordEligibility{
			Alias: alias, Storable: true,
			Blockers: []api.Notice{}, Warnings: []api.Notice{},
		})
	}
	report, err := h.Eligibility(alias)
	if err != nil {
		return problem(c, http.StatusInternalServerError, "config_unreadable")
	}
	return c.JSON(http.StatusOK, describeEligibility(report))
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

// kindOf はパスから namespace を読み取る。namespace は 2 つあり、
// 暗黙に 3 つ目が増えることはない。未知の namespace は既定値にせず
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
		errors.Is(err, secret.ErrUnknownKind):
		return problem(c, http.StatusBadRequest, "invalid_request")
	default:
		return problem(c, http.StatusInternalServerError, "vault_failed")
	}
}

// listCredentials は名前とそれを使うものを返す。値は決して返さ
// ない。secret を読める画面は、乗っ取られたブラウザがそこから
// 読み取れる画面でもあり、選択には名前だけあれば十分だからだ。
func (h PasswordHandlers) listCredentials(c *echo.Context) error {
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
	for _, kind := range []secret.Kind{secret.KindPassword, secret.KindKeyPassphrase} {
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
				Kind: string(kind), Name: name, Uses: uses, Hosts: hosts,
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

func (h PasswordHandlers) ListCredentials(c *echo.Context) error {
	return h.listCredentials(c)
}

func (h PasswordHandlers) SetCredential(c *echo.Context) error {
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

func (h PasswordHandlers) UpdateCredential(c *echo.Context) error {
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

func (h PasswordHandlers) RevealCredential(c *echo.Context) error {
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

func (h PasswordHandlers) DeleteCredential(c *echo.Context) error {
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

func (h PasswordHandlers) AssignCredential(c *echo.Context) error {
	kind, ok := kindOf(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	var request api.AssignCredentialRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if kind == secret.KindPassword {
		if blocked, response := h.ensurePasswordStorable(c, request.Subject); blocked {
			return response
		}
		binding, response := h.passwordBinding(c, request.Subject)
		if response != nil {
			return response
		}
		if err := h.Service.AssignPasswordCredential(request.Subject, request.Name, binding); err != nil {
			return credentialProblem(c, err, nil)
		}
		return h.listCredentials(c)
	}
	if err := h.Service.AssignCredential(kind, request.Subject, request.Name); err != nil {
		return credentialProblem(c, err, nil)
	}
	return h.listCredentials(c)
}

func (h PasswordHandlers) UnassignCredential(c *echo.Context) error {
	kind, ok := kindOf(c)
	if !ok {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Service.UnassignCredential(kind, c.Param("subject")); err != nil {
		return credentialProblem(c, err, nil)
	}
	return h.listCredentials(c)
}

func (h PasswordHandlers) Store(c *echo.Context) error {
	alias := c.Param("alias")
	if err := validate.Alias(alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	var request api.StorePasswordRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if blocked, response := h.ensurePasswordStorable(c, alias); blocked {
		return response
	}
	binding, response := h.passwordBinding(c, alias)
	if response != nil {
		return response
	}
	if err := h.Service.SetBound(alias, request.Password, binding); err != nil {
		return passwordProblem(c, err)
	}
	return h.status(c)
}

func (h PasswordHandlers) passwordBinding(c *echo.Context, alias string) (string, error) {
	if h.Binding == nil {
		return "", problem(c, http.StatusInternalServerError, "config_unreadable")
	}
	binding, err := h.Binding(alias)
	if err != nil {
		return "", problem(c, http.StatusInternalServerError, "config_unreadable")
	}
	return binding, nil
}

// ensurePasswordStorable guards every route that creates a password-to-host
// relationship. Removal stays available even when current config blocks use.
func (h PasswordHandlers) ensurePasswordStorable(c *echo.Context, alias string) (bool, error) {
	if h.Eligibility == nil {
		return false, nil
	}
	report, err := h.Eligibility(alias)
	if err != nil {
		return true, problem(c, http.StatusInternalServerError, "config_unreadable")
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

func (h PasswordHandlers) Forget(c *echo.Context) error {
	alias := c.Param("alias")
	if err := validate.Alias(alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	if err := h.Service.Remove(alias); err != nil {
		return passwordProblem(c, err)
	}
	return h.status(c)
}

func passwordProblem(c *echo.Context, err error) error {
	var schema *secret.SchemaVersionError
	var migration *secret.MigrationError
	switch {
	case errors.Is(err, secret.ErrLocked):
		return problem(c, http.StatusConflict, "vault_locked")
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
	case errors.Is(err, secret.ErrStorageBusy):
		return problemDetail(c, http.StatusConflict, "vault_storage_busy", "another workspace mutation did not finish within 30 seconds")
	case errors.Is(err, fs.ErrPermission), errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return problemDetail(c, http.StatusInternalServerError, "vault_storage_permission_denied", "the operating system denied access to the app's private storage")
	case errors.Is(err, syscall.ENOSPC):
		return problemDetail(c, http.StatusInsufficientStorage, "vault_storage_full", "the app's private storage does not have enough free space")
	case errors.Is(err, syscall.EROFS):
		return problemDetail(c, http.StatusInternalServerError, "vault_storage_read_only", "the app's private storage is read-only")
	case errors.Is(err, syscall.EIO):
		return problemDetail(c, http.StatusInternalServerError, "vault_storage_io_failed", "the operating system reported an input/output failure while accessing private storage")
	default:
		return problemDetail(c, http.StatusInternalServerError, "vault_write_failed", "the encrypted vault could not be committed to app storage")
	}
}
