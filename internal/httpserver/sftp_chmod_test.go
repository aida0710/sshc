package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"testing"

	"github.com/labstack/echo/v5"
	"sshc/internal/api"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

type httpChmodRemote struct {
	httpMetadataRemote
	changed    []string
	failAfter  int
	lostStatus bool
}

func (remote *httpChmodRemote) CheckChmodNoFollow() error { return nil }
func (remote *httpChmodRemote) ChmodNoFollow(candidate string, mode fs.FileMode) error {
	if remote.failAfter > 0 && len(remote.changed) >= remote.failAfter {
		return fs.ErrPermission
	}
	info := remote.infos[candidate]
	if info.mode&fs.ModeSymlink != 0 {
		return sshcSFTP.ErrConflict
	}
	info.mode = info.mode.Type() | mode.Perm()
	remote.infos[candidate] = info
	remote.changed = append(remote.changed, candidate)
	if remote.lostStatus {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (remote *httpChmodRemote) ReadDir(ctx context.Context, directory string) ([]fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries := []fs.FileInfo{}
	for candidate, info := range remote.infos {
		if candidate != directory && path.Dir(candidate) == directory {
			entries = append(entries, info)
		}
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	return entries, nil
}

type chmodHTTPFixture struct {
	engine      *echo.Echo
	credentials session.Credentials
	service     *sshcSFTP.Service
	remote      *httpChmodRemote
}
type chmodHTTPRequest struct {
	method, endpoint, token string
	body                    any
}

func newChmodHTTPFixture(t *testing.T) chmodHTTPFixture {
	t.Helper()
	manager, bootstrap, err := session.NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credentials, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	remote := &httpChmodRemote{httpMetadataRemote: httpMetadataRemote{infos: map[string]remoteMetadataInfo{
		"/":           {name: "/", mode: fs.ModeDir | 0o755},
		"/a":          {name: "a", mode: 0o600, size: 1},
		"/tree":       {name: "tree", mode: fs.ModeDir | 0o700},
		"/tree/child": {name: "child", mode: 0o600, size: 1},
		"/tree/link":  {name: "link", mode: fs.ModeSymlink | 0o777, size: 2},
		"/z":          {name: "z", mode: 0o600, size: 1},
	}}}
	service := &sshcSFTP.Service{Open: func(context.Context, string) (sshcSFTP.Remote, error) { return remote, nil }}
	transfers := sshcSFTP.NewTransferManager(service, t.TempDir())
	t.Cleanup(func() { _ = transfers.Close() })
	registry := actionRegistry{}
	addSFTPActions(registry, service)
	actions := ActionHandlers{Sessions: manager, Kinds: registry}
	engine := echo.New()
	engine.Use((Security{ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost, Sessions: manager, Unlocked: alwaysUnlocked}).Middleware)
	registerActionRoutes(engine, actions)
	registerSFTPRoutes(engine, SFTPHandlers{Service: service, Transfers: transfers, Actions: actions})
	return chmodHTTPFixture{engine: engine, credentials: credentials, service: service, remote: remote}
}
func (fixture chmodHTTPFixture) send(t *testing.T, request chmodHTTPRequest) *httptest.ResponseRecorder {
	t.Helper()
	return sendKeyRequest(t, fixture.engine, fixture.credentials, request.method, request.endpoint, mustMarshal(t, request.body), request.token)
}
func (fixture chmodHTTPFixture) selection(t *testing.T, paths []string) api.SFTPChmodSelection {
	t.Helper()
	entries := make([]api.SFTPChmodEntry, len(paths))
	for index, candidate := range paths {
		entry, err := fixture.service.Stat(t.Context(), "edge", candidate)
		if err != nil {
			t.Fatal(err)
		}
		entries[index] = api.SFTPChmodEntry{Path: candidate, ExpectedRevision: entry.Revision}
	}
	return api.SFTPChmodSelection{Entries: entries, Options: api.SFTPChmodOptions{FileMode: "644", DirectoryMode: "755"}}
}
func (fixture chmodHTTPFixture) plan(t *testing.T, selection api.SFTPChmodSelection) api.SFTPChmodPlan {
	t.Helper()
	response := fixture.send(t, chmodHTTPRequest{method: http.MethodPost, endpoint: "/api/v1/sftp/edge/mode-plan", body: selection})
	if response.Code != http.StatusOK {
		t.Fatalf("plan = %d %s", response.Code, response.Body.String())
	}
	var plan api.SFTPChmodPlan
	if err := json.Unmarshal(response.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func (fixture chmodHTTPFixture) apply(t *testing.T, plan api.SFTPChmodPlan, selection api.SFTPChmodSelection) *httptest.ResponseRecorder {
	t.Helper()
	return fixture.send(t, chmodHTTPRequest{method: http.MethodPatch, endpoint: "/api/v1/sftp/edge/modes", token: plan.ActionToken,
		body: api.SFTPChmodSelectionRequest{Entries: selection.Entries, Options: selection.Options, ExpectedRevision: plan.Revision}})
}

func TestChmodHTTPBindsSelectionOptionsPlanAndSingleUseConfirmation(t *testing.T) {
	fixture := newChmodHTTPFixture(t)
	selection := fixture.selection(t, []string{"/a", "/tree"})
	selection.Options.Recursive = true
	plan := fixture.plan(t, selection)
	if plan.SelectionCount != 2 || plan.Files != 2 || plan.Directories != 1 || plan.SkippedSymlinks != 1 || len(fixture.remote.changed) != 0 {
		t.Fatalf("invalid/read-write plan: %+v", plan)
	}
	body := api.SFTPChmodSelectionRequest{Entries: selection.Entries, Options: selection.Options, ExpectedRevision: plan.Revision}
	response := fixture.send(t, chmodHTTPRequest{method: http.MethodPatch, endpoint: "/api/v1/sftp/edge/modes", body: body})
	if response.Code != http.StatusForbidden || problemCode(t, response.Body.Bytes()) != "action_token_required" {
		t.Fatalf("missing token = %d %s", response.Code, response.Body.String())
	}
	for _, change := range []func(*api.SFTPChmodSelectionRequest){
		func(body *api.SFTPChmodSelectionRequest) { body.Options.FileMode = "600" },
		func(body *api.SFTPChmodSelectionRequest) { body.Options.DirectoryMode = "700" },
		func(body *api.SFTPChmodSelectionRequest) { body.Options.Recursive = false },
		func(body *api.SFTPChmodSelectionRequest) { body.Entries = body.Entries[:1] },
		func(body *api.SFTPChmodSelectionRequest) { body.ExpectedRevision = "other-plan" },
	} {
		altered := body
		change(&altered)
		response = fixture.send(t, chmodHTTPRequest{method: http.MethodPatch, endpoint: "/api/v1/sftp/edge/modes", body: altered, token: plan.ActionToken})
		if response.Code < 400 || len(fixture.remote.changed) != 0 {
			t.Fatalf("changed confirmation accepted: %d %s", response.Code, response.Body.String())
		}
	}
	wrongPlan := fixture.plan(t, fixture.selection(t, []string{"/z"}))
	response = fixture.send(t, chmodHTTPRequest{method: http.MethodPatch, endpoint: "/api/v1/sftp/edge/modes", body: body, token: wrongPlan.ActionToken})
	if response.Code != http.StatusForbidden || len(fixture.remote.changed) != 0 {
		t.Fatalf("wrong token accepted: %d %s", response.Code, response.Body.String())
	}
	response = fixture.send(t, chmodHTTPRequest{method: http.MethodPatch, endpoint: "/api/v1/sftp/edge/modes", body: body, token: plan.ActionToken})
	if response.Code != http.StatusOK || len(fixture.remote.changed) != 3 {
		t.Fatalf("confirmed change = %d %s", response.Code, response.Body.String())
	}
	if fixture.remote.infos["/a"].mode != 0o644 || fixture.remote.infos["/tree"].mode.Perm() != 0o755 || fixture.remote.infos["/tree/link"].mode != fs.ModeSymlink|0o777 {
		t.Fatal("type-specific modes or link safety lost")
	}
	response = fixture.send(t, chmodHTTPRequest{method: http.MethodPatch, endpoint: "/api/v1/sftp/edge/modes", body: body, token: plan.ActionToken})
	if response.Code < 400 || len(fixture.remote.changed) != 3 {
		t.Fatalf("replayed change = %d %s", response.Code, response.Body.String())
	}
}

func TestChmodHTTPRefusesNestedChangesBeforeAnyMutation(t *testing.T) {
	fixture := newChmodHTTPFixture(t)
	selection := fixture.selection(t, []string{"/a", "/tree"})
	selection.Options.Recursive = true
	plan := fixture.plan(t, selection)
	fixture.remote.infos["/tree/new"] = remoteMetadataInfo{name: "new", mode: 0o600, size: 1}
	body := api.SFTPChmodSelectionRequest{Entries: selection.Entries, Options: selection.Options, ExpectedRevision: plan.Revision}
	response := fixture.send(t, chmodHTTPRequest{method: http.MethodPatch, endpoint: "/api/v1/sftp/edge/modes", body: body, token: plan.ActionToken})
	if response.Code != http.StatusConflict || len(fixture.remote.changed) != 0 {
		t.Fatalf("preflight conflict = %d %s; changed %v", response.Code, response.Body.String(), fixture.remote.changed)
	}
}

func TestChmodHTTPReportsTheAppliedCountWhenExecutionStops(t *testing.T) {
	fixture := newChmodHTTPFixture(t)
	selection := fixture.selection(t, []string{"/a", "/z"})
	plan := fixture.plan(t, selection)
	fixture.remote.failAfter = 1
	response := fixture.apply(t, plan, selection)
	var result api.SFTPChmodResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusMultiStatus || result.Applied != 1 || result.Items != 2 || result.Complete || fixture.remote.infos["/z"].mode != 0o600 {
		t.Fatalf("partial outcome = %d %+v", response.Code, result)
	}
}

func TestSingleAndRecursiveChmodHTTPUseTheCompleteConfirmationPlan(t *testing.T) {
	for _, recursive := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "recursive"}[recursive], func(t *testing.T) {
			fixture := newChmodHTTPFixture(t)
			selection := fixture.selection(t, []string{"/tree"})
			target := "edge:/tree:750"
			if recursive {
				target += ":recursive"
			}
			issued := fixture.send(t, chmodHTTPRequest{method: http.MethodPost, endpoint: "/api/v1/actions", body: api.IssueActionRequest{Kind: session.ActionSFTPChmod, Target: target}})
			if issued.Code != http.StatusCreated {
				t.Fatalf("issue = %d %s", issued.Code, issued.Body.String())
			}
			var token api.IssueActionResponse
			if err := json.Unmarshal(issued.Body.Bytes(), &token); err != nil {
				t.Fatal(err)
			}
			if recursive {
				fixture.remote.infos["/tree/new"] = remoteMetadataInfo{name: "new", mode: 0o600, size: 1}
			}
			response := fixture.send(t, chmodHTTPRequest{method: http.MethodPatch, endpoint: "/api/v1/sftp/edge/mode", token: token.Token, body: sftpChmodRequest{Path: "/tree", Mode: "750", ExpectedRevision: selection.Entries[0].ExpectedRevision, Recursive: recursive}})
			if recursive {
				if response.Code < 400 || len(fixture.remote.changed) != 0 {
					t.Fatalf("stale recursive token accepted: %d %s", response.Code, response.Body.String())
				}
				return
			}
			if response.Code != http.StatusOK || len(fixture.remote.changed) != 1 {
				t.Fatalf("single change = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestChmodHTTPReportsUncertainFirstMutationAsPartialInsteadOfUnchanged(t *testing.T) {
	fixture := newChmodHTTPFixture(t)
	selection := fixture.selection(t, []string{"/a", "/z"})
	plan := fixture.plan(t, selection)
	fixture.remote.lostStatus = true
	response := fixture.apply(t, plan, selection)
	var result api.SFTPChmodResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusMultiStatus || result.Applied != 0 || result.Complete || fixture.remote.infos["/a"].mode != 0o644 || fixture.remote.infos["/z"].mode != 0o600 {
		t.Fatalf("lost first status = %d %+v; changes %v", response.Code, result, fixture.remote.changed)
	}
}

func TestChmodHTTPConsumesConfirmationEvenWhenPermissionsAlreadyMatch(t *testing.T) {
	fixture := newChmodHTTPFixture(t)
	selection := fixture.selection(t, []string{"/a"})
	selection.Options.FileMode = "600"
	plan := fixture.plan(t, selection)
	response := fixture.apply(t, plan, selection)
	if response.Code != http.StatusOK || len(fixture.remote.changed) != 1 {
		t.Fatalf("confirmed unchanged mode = %d %s", response.Code, response.Body.String())
	}
	// The metadata revision stays equal, so only token consumption can refuse this replay.
	response = fixture.apply(t, plan, selection)
	if response.Code != http.StatusForbidden || problemCode(t, response.Body.Bytes()) != "action_token_invalid" || len(fixture.remote.changed) != 1 {
		t.Fatalf("replayed unchanged mode = %d %s", response.Code, response.Body.String())
	}
}
