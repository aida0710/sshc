package httpserver

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"sync"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/application"
	"sshc/internal/platform"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
)

// PasswordHandlers は vault とヘルパーを提供する。
type PasswordHandlers struct {
	Service *secret.Service
	vault   *vaultOperations
	// KeyHosts projects saved key subjects through the current SSH configuration.
	// It returns relationships only; the vault values never cross this boundary.
	KeyHosts func(relativePaths []string) (map[string][]string, error)
	// Eligibility は alias と保存されたパスワードの間に何があるかを答える。
	// これが注入されているのは、答えが設定グラフと known_hosts から
	// 来るためで、そのどちらについても vault は何も知らない。nil の
	// 関数は何もチェックしないことを意味し、これはこの仕組みができる
	// 前に vault がしていたことである。
	Eligibility func(alias string) (application.PasswordEligibility, error)
	// ResealSnapshot は新しいマスターパスワードでワークスペースを再度 push し、
	// bucket の最新スナップショットが古いパスワードでしか開けないままにはしない。
	// これが注入されているのは、スナップショットの行き先が object store に
	// 属する事柄だからで、nil はこのマシンに更新すべき bucket がないことを意味する。
	ResealSnapshot func(ctx context.Context, passphrase string) error
}

var errVaultUnavailable = errors.New("vault service unavailable")

// vaultOperations は browser と CLI が共有する vault operation の順序境界である。
// local rekey の commit から remote reseal の完了までを一つとして扱うため、
// 次世代の change や lock が途中へ入らない。
type vaultOperations struct {
	mu             sync.Mutex
	service        *secret.Service
	resealSnapshot func(context.Context, string) error
}

func newVaultOperations(
	service *secret.Service,
	resealSnapshot func(context.Context, string) error,
) *vaultOperations {
	return &vaultOperations{service: service, resealSnapshot: resealSnapshot}
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
) (masterPasswordChangeResult, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return masterPasswordChangeResult{}, err
	}
	if v.service == nil {
		return masterPasswordChangeResult{}, errVaultUnavailable
	}
	state, err := v.service.State()
	if err != nil {
		return masterPasswordChangeResult{}, err
	}
	if !state.Unlocked {
		return masterPasswordChangeResult{}, secret.ErrLocked
	}
	return changeMasterPassword(ctx, v.service, v.resealSnapshot, current, next)
}

type masterPasswordChangeResult struct {
	SnapshotResealed bool    `json:"snapshotResealed"`
	SnapshotProblem  *string `json:"snapshotProblem,omitempty"`
}

// changeMasterPassword は local rekey と remote snapshot の再封印を一つの
// application operation にする。HTTP の応答規約は browser と CLI で異なるため、
// ここでは commit 済みかと remote の結果だけを返す。
func changeMasterPassword(
	ctx context.Context,
	service *secret.Service,
	resealSnapshot func(context.Context, string) error,
	current string,
	next string,
) (masterPasswordChangeResult, error) {
	if err := service.ChangeMasterPassword(current, next); err != nil {
		return masterPasswordChangeResult{}, err
	}

	result := masterPasswordChangeResult{SnapshotResealed: true}
	if resealSnapshot != nil {
		if err := resealSnapshot(ctx, next); err != nil {
			if errors.Is(err, remotesync.ErrNotConfigured) {
				return masterPasswordChangeResult{SnapshotResealed: false}, nil
			}
			reason := snapshotProblemCode(err)
			result.SnapshotResealed, result.SnapshotProblem = false, &reason
		}
	} else {
		result.SnapshotResealed = false
	}
	return result, nil
}

// snapshotProblemCode は bucket が更新されなかった理由を、sync 画面が
// すでに使っている語彙と同じ言葉で名付ける。
func snapshotProblemCode(err error) string {
	switch {
	case errors.Is(err, remotesync.ErrRemoteMoved):
		return "sync_remote_moved"
	case errors.Is(err, remotesync.ErrPushRefused):
		return "sync_push_refused"
	default:
		return "sync_failed"
	}
}

func registerPasswordRoutes(engine *echo.Echo, handlers PasswordHandlers) {
	if handlers.vault == nil {
		handlers.vault = newVaultOperations(handlers.Service, handlers.ResealSnapshot)
	}
	engine.GET("/api/v1/passwords", handlers.Status)
	engine.POST("/api/v1/passwords/initialise", handlers.Initialise)
	engine.POST("/api/v1/passwords/unlock", handlers.Unlock)
	engine.POST("/api/v1/passwords/change", handlers.Change)
	engine.POST("/api/v1/passwords/lock", handlers.Lock)
	engine.GET("/api/v1/passwords/:alias/eligibility", handlers.Eligible)
	engine.PUT("/api/v1/passwords/:alias", handlers.Store)
	engine.DELETE("/api/v1/passwords/:alias", handlers.Forget)
	engine.GET("/api/v1/credentials", handlers.ListCredentials)
	engine.PUT("/api/v1/credentials/:kind/assign", handlers.AssignCredential)
	engine.DELETE("/api/v1/credentials/:kind/assign/:subject", handlers.UnassignCredential)
	engine.PUT("/api/v1/credentials/:kind/:name", handlers.SetCredential)
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
	return c.JSON(http.StatusOK, api.PasswordVaultStatus{
		Exists:                  state.Exists,
		Unlocked:                state.Unlocked,
		Aliases:                 aliases,
		DedicatedKeyPassphrases: dedicatedKeyPassphrases,
		MinPassphraseLength:     &minimum,
	})
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

// Change はマスターパスワードを置き換え、それが保持していたものを再封印する。
//
// bucket の最新スナップショットは同じパスワードで封印されているため、
// ここで再び push する — vault にはできない。スナップショットの行き先は
// object store に属する事柄であり、secret パッケージはそれを import
// していないからだ。その脇の日付付きコピーはわざと手を付けず残してある。それらは
// 履歴であり、全部の再封印は bucket 全体のダウンロードと再アップロードを意味する。
//
// push が失敗しても変更は元に戻らない。ローカル側はすでに完了して
// おり、そう伝える方が取り繕うより役に立つ。応答には bucket が
// 更新されたかどうかと、されなかった理由が含まれる。
func (h PasswordHandlers) Change(c *echo.Context) error {
	var request api.ChangeMasterPasswordRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	changed, err := h.vault.Change(c.Request().Context(), request.Current, request.Next)
	if err != nil {
		return passwordProblem(c, err)
	}

	answer := api.ChangeMasterPasswordResult{
		SnapshotResealed: changed.SnapshotResealed,
		SnapshotProblem:  changed.SnapshotProblem,
	}

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
	if err := platform.ValidateAlias(alias); err != nil {
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
// 黙って 3 つ目が増えることはない。未知の namespace は既定値にせず
// ここで拒否する。既定値にすると、typo が黙って namespace を選ぶことになるからだ。
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
	case errors.Is(err, secret.ErrUnsafeName), errors.Is(err, secret.ErrEmptySecret),
		errors.Is(err, secret.ErrUnknownKind):
		return problem(c, http.StatusBadRequest, "invalid_request")
	default:
		return problem(c, http.StatusInternalServerError, "vault_failed")
	}
}

// listCredentials は名前とそれを使うものを答える。値は決して返さ
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
	if err := platform.ValidateAlias(alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	var request api.StorePasswordRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if blocked, response := h.ensurePasswordStorable(c, alias); blocked {
		return response
	}
	if err := h.Service.Set(alias, request.Password); err != nil {
		return passwordProblem(c, err)
	}
	return h.status(c)
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
	if err := platform.ValidateAlias(alias); err != nil {
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	}
	if err := h.Service.Remove(alias); err != nil {
		return passwordProblem(c, err)
	}
	return h.status(c)
}

func passwordProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, secret.ErrLocked):
		return problem(c, http.StatusConflict, "vault_locked")
	case errors.Is(err, secret.ErrAlreadyExists):
		return problem(c, http.StatusConflict, "vault_already_exists")
	case errors.Is(err, secret.ErrNoVault):
		return problem(c, http.StatusNotFound, "vault_missing")
	case errors.Is(err, secret.ErrWrongPassphrase):
		return problem(c, http.StatusForbidden, "wrong_passphrase")
	case errors.Is(err, secret.ErrUnsupportedVersion):
		return problem(c, http.StatusConflict, "vault_too_new")
	case errors.Is(err, secret.ErrCostRefused):
		return problem(c, http.StatusConflict, "vault_cost_refused")
	case errors.Is(err, secret.ErrWeakPassphrase):
		return problem(c, http.StatusBadRequest, "passphrase_too_short")
	case errors.Is(err, secret.ErrEmptySecret):
		return problem(c, http.StatusBadRequest, "password_empty")
	case errors.Is(err, secret.ErrUnsafeName):
		return problem(c, http.StatusBadRequest, "unsafe_alias")
	default:
		return problem(c, http.StatusInternalServerError, "vault_write_failed")
	}
}
