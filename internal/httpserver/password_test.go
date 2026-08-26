package httpserver

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

const testPassphrase = "correct horse battery staple"

func passwordEngine(t *testing.T) (*echo.Echo, *secret.Service) {
	engine, service, _ := passwordEngineIn(t)
	return engine, service
}

// passwordEngineIn は home も返す。API の返答ではなく実際に書かれた
// ものを読むテストのためである。
func passwordEngineIn(t *testing.T) (*echo.Echo, *secret.Service, string) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	service := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)

	engine := echo.New()
	registerPasswordRoutes(engine, PasswordHandlers{
		Service: service, Binding: fixedPasswordBinding,
	})
	return engine, service, home
}

func passwordEngineWithKeyHosts(
	t *testing.T,
	keyHosts func([]string) (map[string][]string, error),
) (*echo.Echo, *secret.Service, string) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	service := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	engine := echo.New()
	registerPasswordRoutes(engine, PasswordHandlers{
		Service: service, Binding: fixedPasswordBinding,
		KeyHosts: keyHosts,
	})
	return engine, service, home
}

func storeDedicatedKeyPassphrase(t *testing.T, service *secret.Service, home, relativePath, passphrase string) {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	_, err = service.WithConnectionSecretsMutation(secret.ConnectionSecretsMutation{
		KeyPassphrase: &secret.KeyPassphraseMutation{RelativePath: relativePath, Passphrase: passphrase},
	}, func(change storage.Change) (storage.Result, error) {
		return manager.Commit(storage.Request{Operation: "test.key-passphrase", Changes: []storage.Change{change}})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func send(t *testing.T, engine *echo.Echo, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set(echo.HeaderContentType, "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func TestNoPasswordRouteEverReturnsAPassword(t *testing.T) {
	// 腐らせてはならないアサーションである。このファイルの全ルートが
	// 走査対象であり、書き込みを行うものも含め、どの body にも
	// 保存された値が含まれてはならない。
	engine, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetBound("bastion", "hunter2", testPasswordBinding); err != nil {
		t.Fatal(err)
	}

	responses := []*httptest.ResponseRecorder{
		send(t, engine, http.MethodGet, "/api/v1/passwords", "", nil),
		send(t, engine, http.MethodPut, "/api/v1/passwords/bastion", `{"password":"hunter2"}`, nil),
		send(t, engine, http.MethodPost, "/api/v1/passwords/unlock", `{"passphrase":"`+testPassphrase+`"}`, nil),
		send(t, engine, http.MethodPost, "/api/v1/passwords/lock", "", nil),
		send(t, engine, http.MethodDelete, "/api/v1/passwords/bastion", "", nil),
	}
	for index, response := range responses {
		if strings.Contains(response.Body.String(), "hunter2") {
			t.Errorf("response %d contains the stored password: %s", index, response.Body.String())
		}
		if strings.Contains(response.Body.String(), testPassphrase) {
			t.Errorf("response %d contains the vault passphrase", index)
		}
	}
}

func TestStatusReportsWhichHostsHaveAPasswordAndNothingElse(t *testing.T) {
	engine, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetBound("bastion", "hunter2", testPasswordBinding); err != nil {
		t.Fatal(err)
	}

	response := send(t, engine, http.MethodGet, "/api/v1/passwords", "", nil)
	var status struct {
		Exists   bool     `json:"exists"`
		Unlocked bool     `json:"unlocked"`
		Aliases  []string `json:"aliases"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode: %v (%s)", err, response.Body.String())
	}
	if !status.Exists || !status.Unlocked || len(status.Aliases) != 1 || status.Aliases[0] != "bastion" {
		t.Errorf("status = %#v", status)
	}
}

func TestPasswordVaultStatusAlwaysIncludesDedicatedKeyPassphrasePaths(t *testing.T) {
	engine, _ := passwordEngine(t)
	response := send(t, engine, http.MethodGet, "/api/v1/passwords", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var status map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	value, ok := status["dedicatedKeyPassphrases"]
	if !ok || string(value) != "[]" {
		t.Fatalf("locked status dedicatedKeyPassphrases = %s, present %t", value, ok)
	}
}

func TestStoringRefusesWhileTheVaultIsLocked(t *testing.T) {
	engine, _ := passwordEngine(t)

	response := send(t, engine, http.MethodPut, "/api/v1/passwords/bastion", `{"password":"hunter2"}`, nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("code = %d, want 409", response.Code)
	}
}

func TestInitialiseRefusesAShortPassphraseAndDoesNotCreateAVault(t *testing.T) {
	engine, service := passwordEngine(t)

	response := send(t, engine, http.MethodPost, "/api/v1/passwords/initialise", `{"passphrase":"short"}`, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", response.Code)
	}
	exists, err := service.Exists()
	if err != nil || exists {
		t.Errorf("a vault was created: %v, %v", exists, err)
	}
}

func TestStoreRefusesAPasswordTheHostWouldNeverBeOffered(t *testing.T) {
	// interface はこのフィールドを無効化するが、interface は差し替え
	// 可能でもこちら側はそうではない。blocker があれば保存された
	// パスワードは決して使われず、保存しても無駄に secret をディスクに置くだけになる。
	engine, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	registerPasswordRoutes(engine, PasswordHandlers{
		Service: service, Binding: fixedPasswordBinding,
		Eligibility: func(alias string) (application.PasswordEligibility, error) {
			return application.PasswordEligibility{
				Alias:    alias,
				Storable: false,
				Blockers: []application.Notice{{Code: application.BlockerPasswordAuthenticationOff}},
			}, nil
		},
	})

	recorder := send(t, engine, http.MethodPut, "/api/v1/passwords/bastion",
		`{"password":"hunter2"}`, nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("PUT = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Code     string   `json:"code"`
		Blockers []string `json:"blockers"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "password_not_storable" {
		t.Errorf("code = %q", body.Code)
	}
	// 理由は拒否と共に運ばれる。理由のない 409 だけでは、ユーザーは
	// 設定が下した判断を vault のせいだと思って探してしまう。
	if len(body.Blockers) != 1 || body.Blockers[0] != application.BlockerPasswordAuthenticationOff {
		t.Errorf("blockers = %#v", body.Blockers)
	}
	// そして何も保存されなかった。
	if service.Has("bastion") {
		t.Error("the vault holds a password for a host that refused one")
	}
}

func TestPasswordWritesFailClosedWithoutAuthenticationBinding(t *testing.T) {
	home := t.TempDir()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	service := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office", "shared"); err != nil {
		t.Fatal(err)
	}
	engine := echo.New()
	registerPasswordRoutes(engine, PasswordHandlers{Service: service})

	for name, response := range map[string]*httptest.ResponseRecorder{
		"dedicated": send(t, engine, http.MethodPut, "/api/v1/passwords/edge", `{"password":"secret"}`, nil),
		"saved": send(t, engine, http.MethodPut, credentialPath("password", "/assign"),
			`{"subject":"edge","name":"office"}`, nil),
	} {
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "config_unreadable") {
			t.Errorf("%s password write = %d: %s", name, response.Code, response.Body.String())
		}
	}
	if service.Has("edge") {
		t.Fatal("password write without an authentication binding changed the vault")
	}
}

func TestAssignCredentialRefusesAPasswordForADirectKeyButNotAKeyPassphrase(t *testing.T) {
	_, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office", "shared"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindKeyPassphrase, "keys", "phrase"); err != nil {
		t.Fatal(err)
	}
	engine := echo.New()
	registerPasswordRoutes(engine, PasswordHandlers{
		Service: service, Binding: fixedPasswordBinding,
		Eligibility: func(alias string) (application.PasswordEligibility, error) {
			return application.PasswordEligibility{
				Alias: alias, Storable: false,
				Blockers: []application.Notice{{Code: application.BlockerIdentityFileConfigured}},
			}, nil
		},
	})

	password := send(t, engine, http.MethodPut, credentialPath("password", "/assign"),
		`{"subject":"bastion","name":"office"}`, nil)
	if password.Code != http.StatusConflict || service.BoundPasswordFor("bastion", testPasswordBinding) != "" {
		t.Fatalf("password assignment = %d: %s", password.Code, password.Body.String())
	}
	keyPhrase := send(t, engine, http.MethodPut, credentialPath("key_passphrase", "/assign"),
		`{"subject":"id_ed25519","name":"keys"}`, nil)
	if keyPhrase.Code != http.StatusOK {
		t.Fatalf("key passphrase assignment = %d: %s", keyPhrase.Code, keyPhrase.Body.String())
	}
}

func TestEligibilityIsReadableAndCarriesTheWarnings(t *testing.T) {
	engine, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	registerPasswordRoutes(engine, PasswordHandlers{
		Service: service, Binding: fixedPasswordBinding,
		Eligibility: func(alias string) (application.PasswordEligibility, error) {
			return application.PasswordEligibility{
				Alias: alias, Storable: true,
				Warnings: []application.Notice{{Code: application.WarnHostKeyUnknown, Detail: "203.0.113.10"}},
			}, nil
		},
	})

	recorder := send(t, engine, http.MethodGet, "/api/v1/passwords/bastion/eligibility", "", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", recorder.Code, recorder.Body.String())
	}
	var report api.PasswordEligibility
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.Storable || len(report.Warnings) != 1 {
		t.Fatalf("report = %#v", report)
	}
	if report.Warnings[0].Code != application.WarnHostKeyUnknown {
		t.Errorf("warning = %#v", report.Warnings[0])
	}
}

func credentialPath(kind, rest string) string {
	return "/api/v1/credentials/" + kind + rest
}

// 腐らせてはならない走査を、credential を運ぶルートにも広げたもの
// である。書き込みを行うものも含め、すべてに問い合わせ、
// どの body にも値が含まれてはならない。
func TestNoCredentialRouteEverReturnsASecret(t *testing.T) {
	engine, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}

	responses := []*httptest.ResponseRecorder{
		send(t, engine, http.MethodPut, credentialPath("password", "/office"), `{"secret":"hunter2"}`, nil),
		send(t, engine, http.MethodPut, credentialPath("key_passphrase", "/build"), `{"secret":"phrase-2"}`, nil),
		send(t, engine, http.MethodGet, "/api/v1/credentials", "", nil),
		send(t, engine, http.MethodPut, credentialPath("password", "/assign"), `{"subject":"web-1","name":"office"}`, nil),
		send(t, engine, http.MethodGet, "/api/v1/credentials", "", nil),
		send(t, engine, http.MethodDelete, credentialPath("password", "/assign/web-1"), "", nil),
		send(t, engine, http.MethodDelete, credentialPath("password", "/office"), "", nil),
	}
	for index, response := range responses {
		for _, absent := range []string{"hunter2", "phrase-2", testPassphrase} {
			if strings.Contains(response.Body.String(), absent) {
				t.Errorf("response %d carries %q: %s", index, absent, response.Body.String())
			}
		}
	}
}

func TestCredentialsListNamesAndUses(t *testing.T) {
	engine, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	send(t, engine, http.MethodPut, credentialPath("password", "/office"), `{"secret":"hunter2"}`, nil)
	send(t, engine, http.MethodPut, credentialPath("password", "/assign"), `{"subject":"web-1","name":"office"}`, nil)
	send(t, engine, http.MethodPut, credentialPath("password", "/assign"), `{"subject":"web-2","name":"office"}`, nil)

	response := send(t, engine, http.MethodGet, "/api/v1/credentials", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{"office", "web-1", "web-2"} {
		if !strings.Contains(body, want) {
			t.Errorf("list does not carry %q: %s", want, body)
		}
	}
}

func TestCredentialsListIncludesNamedAndDedicatedHostUsage(t *testing.T) {
	engine, service, home := passwordEngineWithKeyHosts(t, func(paths []string) (map[string][]string, error) {
		if !slices.Equal(paths, []string{"keys/id_a", "keys/id_b", "keys/id_owned"}) {
			t.Fatalf("key host paths = %#v", paths)
		}
		return map[string][]string{
			"keys/id_a":     {"build-a"},
			"keys/id_b":     {"build-a", "build-b"},
			"keys/id_owned": {"deploy"},
		}, nil
	})
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	for _, credential := range []struct {
		kind  secret.Kind
		name  string
		value string
	}{
		{secret.KindPassword, "office", "account-secret-value"},
		{secret.KindKeyPassphrase, "team-phrase", "shared-key-passphrase-value"},
	} {
		if err := service.SetCredential(credential.kind, credential.name, credential.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.AssignPasswordCredential("web-1", "office", testPasswordBinding); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"keys/id_a", "keys/id_b"} {
		if err := service.AssignCredential(secret.KindKeyPassphrase, key, "team-phrase"); err != nil {
			t.Fatal(err)
		}
	}
	storeDedicatedKeyPassphrase(t, service, home, "keys/id_owned", "owned-key-passphrase-value")

	response := send(t, engine, http.MethodGet, "/api/v1/credentials", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", response.Code, response.Body.String())
	}
	var answer api.CredentialList
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if !answer.KeyHostUsageComplete {
		t.Fatal("key host usage unexpectedly incomplete")
	}
	byName := map[string]api.Credential{}
	for _, credential := range answer.Credentials {
		byName[credential.Name] = credential
	}
	if office := byName["office"]; !slices.Equal(office.Uses, []string{"web-1"}) || !slices.Equal(office.Hosts, []string{"web-1"}) {
		t.Fatalf("office = %#v", office)
	}
	if team := byName["team-phrase"]; !slices.Equal(team.Uses, []string{"keys/id_a", "keys/id_b"}) ||
		!slices.Equal(team.Hosts, []string{"build-a", "build-b"}) {
		t.Fatalf("team phrase = %#v", team)
	}
	if len(answer.DedicatedKeyPassphrases) != 1 || answer.DedicatedKeyPassphrases[0].Key != "keys/id_owned" ||
		!slices.Equal(answer.DedicatedKeyPassphrases[0].Hosts, []string{"deploy"}) {
		t.Fatalf("dedicated = %#v", answer.DedicatedKeyPassphrases)
	}
	for _, absent := range []string{
		"account-secret-value", "shared-key-passphrase-value", "owned-key-passphrase-value", testPassphrase,
	} {
		if strings.Contains(response.Body.String(), absent) {
			t.Errorf("credential list contains secret %q: %s", absent, response.Body.String())
		}
	}
}

func TestCredentialMutationRemainsSuccessfulWhenKeyHostUsageIsUnavailable(t *testing.T) {
	engine, service, _ := passwordEngineWithKeyHosts(t, func([]string) (map[string][]string, error) {
		return nil, errors.New("broken include graph")
	})
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindKeyPassphrase, "team", "key-secret"); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignCredential(secret.KindKeyPassphrase, "keys/id_team", "team"); err != nil {
		t.Fatal(err)
	}

	response := send(t, engine, http.MethodPut, credentialPath("password", "/office"), `{"secret":"account-secret"}`, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("stored credential reported as failed: %d %s", response.Code, response.Body.String())
	}
	var answer api.CredentialList
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.KeyHostUsageComplete {
		t.Fatal("failed key-host projection reported complete")
	}
	found := false
	for _, credential := range answer.Credentials {
		found = found || credential.Kind == string(secret.KindPassword) && credential.Name == "office"
	}
	if !found {
		t.Fatalf("successful credential is absent: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "account-secret") || strings.Contains(response.Body.String(), "key-secret") {
		t.Fatalf("response contains a secret: %s", response.Body.String())
	}
}

// 2 台のマシンがまだ指している名前を削除すれば、後でどこか別の
// 場所で両方が壊れる。拒否はどれが使っているかを教える。
func TestDeletingACredentialInUseIsRefusedWithItsUses(t *testing.T) {
	engine, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	send(t, engine, http.MethodPut, credentialPath("password", "/office"), `{"secret":"hunter2"}`, nil)
	send(t, engine, http.MethodPut, credentialPath("password", "/assign"), `{"subject":"web-1","name":"office"}`, nil)

	response := send(t, engine, http.MethodDelete, credentialPath("password", "/office"), "", nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("delete of a used credential = %d, want 409: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "web-1") {
		t.Errorf("the refusal does not say what uses it: %s", response.Body.String())
	}
}

// ブラウザが実際に到達する境界における、その分離である。
func TestAHostCannotBePointedAtAKeyPassphraseThroughTheAPI(t *testing.T) {
	engine, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	send(t, engine, http.MethodPut, credentialPath("key_passphrase", "/build"), `{"secret":"phrase"}`, nil)

	response := send(t, engine, http.MethodPut, credentialPath("password", "/assign"), `{"subject":"web-1","name":"build"}`, nil)
	if response.Code == http.StatusOK {
		t.Error("a host was pointed at a key passphrase through the API")
	}
}

func TestAnUnknownCredentialKindIsRefused(t *testing.T) {
	engine, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}

	if code := send(t, engine, http.MethodPut, credentialPath("wallet", "/x"), `{"secret":"y"}`, nil).Code; code != http.StatusBadRequest {
		t.Errorf("an unknown kind = %d, want 400", code)
	}
}

func TestEveryCredentialRouteRefusesALockedVault(t *testing.T) {
	engine, service := passwordEngine(t)
	if err := service.Initialise(testPassphrase); err != nil {
		t.Fatal(err)
	}
	service.Lock()

	for _, call := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/credentials", ""},
		{http.MethodPut, credentialPath("password", "/office"), `{"secret":"x"}`},
		{http.MethodDelete, credentialPath("password", "/office"), ""},
		{http.MethodPut, credentialPath("password", "/assign"), `{"subject":"web-1","name":"office"}`},
		{http.MethodDelete, credentialPath("password", "/assign/web-1"), ""},
	} {
		if code := send(t, engine, call.method, call.path, call.body, nil).Code; code != http.StatusConflict {
			t.Errorf("%s %s while locked = %d, want 409", call.method, call.path, code)
		}
	}
}

// 端から端までの、仕組み全体である。
//
// 1 つの名前の下に secret が 1 つ、それを指すホストが 2 つ、そして
// ホストも secret も指定しないディスク上のファイルがある。これが
// secret に名前を付けて得たものだ。以前は 2 台の同じパスワードは
// 2 つのコピーであり、変更は 2 箇所の編集で、同じものと分かる術がなかった。
func TestOneNamedSecretServesTwoHostsAndTheFileNamesNeither(t *testing.T) {
	engine, _, home := passwordEngineIn(t)

	if code := send(t, engine, http.MethodPost, "/api/v1/passwords/initialise",
		`{"passphrase":"`+testPassphrase+`"}`, nil).Code; code != http.StatusOK {
		t.Fatalf("initialise = %d", code)
	}
	if code := send(t, engine, http.MethodPut, "/api/v1/credentials/password/office-vm",
		`{"secret":"hunter2"}`, nil).Code; code != http.StatusOK {
		t.Fatalf("store = %d", code)
	}
	for _, alias := range []string{"web-1", "web-2"} {
		body := `{"subject":"` + alias + `","name":"office-vm"}`
		if code := send(t, engine, http.MethodPut, "/api/v1/credentials/password/assign", body, nil).Code; code != http.StatusOK {
			t.Fatalf("assign %s = %d", alias, code)
		}
	}

	listed := send(t, engine, http.MethodGet, "/api/v1/credentials", "", nil)
	var answer api.CredentialList
	if err := json.Unmarshal(listed.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Credentials) != 1 || len(answer.Credentials[0].Uses) != 2 {
		t.Fatalf("the list does not say one name serves two hosts: %s", listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), "hunter2") {
		t.Error("the list carries the secret itself")
	}

	sealed, err := os.ReadFile(filepath.Join(home, ".ssh", filepath.FromSlash(secret.WorkspacePath)))
	if err != nil {
		t.Fatalf("the vault was not written: %v", err)
	}
	for _, absent := range []string{"hunter2", "office-vm", "web-1", "web-2"} {
		if strings.Contains(string(sealed), absent) {
			t.Errorf("the sealed file contains %q in clear", absent)
		}
	}

	// 同じワークスペースに対する 2 つ目の service は、このファイル形式が
	// 存在する理由そのものである、application の次回の実行である。
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	reopened := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := reopened.Unlock(testPassphrase); err != nil {
		t.Fatalf("Unlock = %v", err)
	}
	for _, alias := range []string{"web-1", "web-2"} {
		if got := reopened.BoundPasswordFor(alias, testPasswordBinding); got != "hunter2" {
			t.Errorf("PasswordFor(%q) = %q, want the one secret both point at", alias, got)
		}
	}
}

// マスターパスワード変更はローカル暗号化だけを変更し、remote同期状態を応答へ
// 混ぜない。remote snapshotは専用の同期鍵で暗号化されている。
func TestChangingTheMasterPasswordReturnsTheLocalVaultState(t *testing.T) {
	engine, service, _ := passwordEngineIn(t)
	if code := send(t, engine, http.MethodPost, "/api/v1/passwords/initialise",
		`{"passphrase":"`+testPassphrase+`"}`, nil).Code; code != http.StatusOK {
		t.Fatal("initialise")
	}
	_ = service

	recorder := send(t, engine, http.MethodPost, "/api/v1/passwords/change",
		`{"current":"`+testPassphrase+`","next":"a different master password"}`, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("change = %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer api.ChangeMasterPasswordResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if !answer.Vault.Exists || !answer.Vault.Unlocked {
		t.Fatalf("vault status = %+v", answer.Vault)
	}

	// そして今動くのは新しい方である。
	if code := send(t, engine, http.MethodPost, "/api/v1/passwords/unlock",
		`{"passphrase":"`+testPassphrase+`"}`, nil).Code; code != http.StatusForbidden {
		t.Error("the old master password still unlocks")
	}
	if code := send(t, engine, http.MethodPost, "/api/v1/passwords/unlock",
		`{"passphrase":"a different master password"}`, nil).Code; code != http.StatusOK {
		t.Error("the new master password does not unlock")
	}
}

func TestPasswordProblemClassifiesStorageFailuresWithoutExposingPaths(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "permission", err: &os.PathError{Op: "open", Path: "/data/user/0/private", Err: syscall.EACCES}, status: http.StatusInternalServerError, code: "vault_storage_permission_denied"},
		{name: "full", err: &os.PathError{Op: "write", Path: "/data/user/0/private", Err: syscall.ENOSPC}, status: http.StatusInsufficientStorage, code: "vault_storage_full"},
		{name: "read-only", err: &os.PathError{Op: "rename", Path: "/data/user/0/private", Err: syscall.EROFS}, status: http.StatusInternalServerError, code: "vault_storage_read_only"},
		{name: "busy", err: secret.ErrStorageBusy, status: http.StatusConflict, code: "vault_storage_busy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := echo.New()
			engine.GET("/", func(c *echo.Context) error { return passwordProblem(c, test.err) })
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.status, recorder.Body.String())
			}
			var answer api.Problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
				t.Fatal(err)
			}
			if answer.Code != test.code {
				t.Errorf("code = %q, want %q", answer.Code, test.code)
			}
			if strings.Contains(recorder.Body.String(), "/data/user") {
				t.Errorf("response exposed a private path: %s", recorder.Body.String())
			}
		})
	}
}

func TestPasswordProblemReportsTheExactVaultSchemaDirection(t *testing.T) {
	for _, test := range []struct {
		name  string
		found int
		code  string
	}{
		{name: "older", found: 3, code: "vault_schema_older"},
		{name: "newer", found: 5, code: "vault_schema_newer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := echo.New()
			engine.GET("/", func(c *echo.Context) error {
				return passwordProblem(c, &secret.SchemaVersionError{Found: test.found, Supported: 4})
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
			}
			var answer api.Problem
			if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
				t.Fatal(err)
			}
			if answer.Code != test.code || answer.CurrentVersion == nil || *answer.CurrentVersion != test.found ||
				answer.RequiredVersion == nil || *answer.RequiredVersion != 4 {
				t.Fatalf("problem = %#v", answer)
			}
		})
	}
}

func TestPasswordProblemReportsTheExactFailingMigrationWithoutItsCause(t *testing.T) {
	engine := echo.New()
	engine.GET("/", func(c *echo.Context) error {
		return passwordProblem(c, &secret.MigrationError{
			From: 4, To: 5, Cause: errors.New("private migration detail"),
		})
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var answer api.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Code != "vault_migration_failed" || answer.CurrentVersion == nil || *answer.CurrentVersion != 4 ||
		answer.RequiredVersion == nil || *answer.RequiredVersion != 5 {
		t.Fatalf("problem = %#v", answer)
	}
	if strings.Contains(recorder.Body.String(), "private migration detail") {
		t.Fatalf("problem exposed the internal migration cause: %s", recorder.Body.String())
	}
}

func TestResetUnsupportedVaultRequiresAnExplicitAcknowledgement(t *testing.T) {
	engine, _ := passwordEngine(t)
	for _, body := range []string{
		`{"passphrase":"` + testPassphrase + `"}`,
		`{"passphrase":"` + testPassphrase + `","acknowledged":false}`,
	} {
		response := send(t, engine, http.MethodPost, "/api/v1/passwords/reset-unsupported", body, nil)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "vault_reset_acknowledgement_required") {
			t.Fatalf("reset without acknowledgement = %d: %s", response.Code, response.Body.String())
		}
	}
}
