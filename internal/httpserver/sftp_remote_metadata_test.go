package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"sshc/internal/api"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

type remoteMetadataInfo struct {
	name  string
	mode  fs.FileMode
	size  int64
	owner sshcSFTP.Ownership
}

func (info remoteMetadataInfo) Name() string                          { return info.name }
func (info remoteMetadataInfo) Mode() fs.FileMode                     { return info.mode }
func (info remoteMetadataInfo) Size() int64                           { return info.size }
func (info remoteMetadataInfo) ModTime() time.Time                    { return time.Unix(1, 0) }
func (info remoteMetadataInfo) IsDir() bool                           { return info.mode.IsDir() }
func (info remoteMetadataInfo) Sys() any                              { return nil }
func (info remoteMetadataInfo) Ownership() (sshcSFTP.Ownership, bool) { return info.owner, true }

type httpMetadataRemote struct {
	sshcSFTP.Remote
	infos       map[string]remoteMetadataInfo
	targets     map[string]string
	mutations   int
	mutationErr error
}

func (remote *httpMetadataRemote) Close() error { return nil }
func (remote *httpMetadataRemote) Lstat(candidate string) (fs.FileInfo, error) {
	info, exists := remote.infos[candidate]
	if !exists {
		return nil, fs.ErrNotExist
	}
	return info, nil
}
func (remote *httpMetadataRemote) ReadLink(candidate string) (string, error) {
	return remote.targets[candidate], nil
}
func (remote *httpMetadataRemote) Symlink(target, candidate string) error {
	if remote.mutationErr != nil {
		return remote.mutationErr
	}
	if _, exists := remote.infos[candidate]; exists {
		return fs.ErrExist
	}
	remote.infos[candidate] = remoteMetadataInfo{name: path.Base(candidate), mode: fs.ModeSymlink | 0o777, size: int64(len(target))}
	remote.targets[candidate] = target
	remote.mutations++
	return nil
}
func (remote *httpMetadataRemote) ReplaceSymlink(temporary, candidate string) error {
	if remote.mutationErr != nil {
		return remote.mutationErr
	}
	remote.infos[candidate] = remote.infos[temporary]
	remote.targets[candidate] = remote.targets[temporary]
	delete(remote.infos, temporary)
	delete(remote.targets, temporary)
	remote.mutations++
	return nil
}
func (remote *httpMetadataRemote) Remove(candidate string) error {
	delete(remote.infos, candidate)
	delete(remote.targets, candidate)
	return nil
}
func (remote *httpMetadataRemote) Chown(candidate string, uid, gid uint32) error {
	if remote.mutationErr != nil {
		return remote.mutationErr
	}
	info := remote.infos[candidate]
	info.owner = sshcSFTP.Ownership{UID: uid, GID: gid}
	remote.infos[candidate] = info
	remote.mutations++
	return nil
}

func TestMetadataHTTPRequiresConfirmationBindsParametersAndConsumesOnlyOnce(t *testing.T) {
	for _, operation := range []string{"create", "retarget", "ownership"} {
		t.Run(operation, func(t *testing.T) {
			manager, bootstrap, err := session.NewManager(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			credentials, _, err := manager.BootstrapForSession(bootstrap, "")
			if err != nil {
				t.Fatal(err)
			}
			remote := &httpMetadataRemote{infos: map[string]remoteMetadataInfo{
				"/file": {name: "file", mode: 0o644, size: 4, owner: sshcSFTP.Ownership{UID: 1000, GID: 1000}},
				"/link": {name: "link", mode: fs.ModeSymlink | 0o777, size: 5},
			}, targets: map[string]string{"/link": "/file"}}
			service := &sshcSFTP.Service{Open: func(context.Context, string) (sshcSFTP.Remote, error) { return remote, nil }}
			registry := actionRegistry{}
			addSFTPActions(registry, service)
			actions := ActionHandlers{Sessions: manager, Kinds: registry}
			transfers := sshcSFTP.NewTransferManager(service, t.TempDir())
			defer transfers.Close()
			engine := echo.New()
			engine.Use((Security{ExpectedHost: keyTestHost, ExpectedOrigin: "http://" + keyTestHost, Sessions: manager, Unlocked: alwaysUnlocked}).Middleware)
			registerActionRoutes(engine, actions)
			registerSFTPRoutes(engine, SFTPHandlers{Service: service, Transfers: transfers, Actions: actions})
			revisionPath := "/link"
			if operation == "ownership" {
				revisionPath = "/file"
			}
			entry, err := service.Stat(t.Context(), "edge", revisionPath)
			if err != nil {
				t.Fatal(err)
			}
			method, endpoint, kind, target := http.MethodPatch, "/api/v1/sftp/edge/symlink", session.ActionSFTPSymlink, symlinkActionTarget("edge", "/link", "relative:リンク")
			body := mustMarshal(t, api.SFTPChangeSymlinkRequest{Path: "/link", Target: "relative:リンク", ExpectedRevision: entry.Revision})
			if operation == "create" {
				method = http.MethodPost
				kind, target = session.ActionSFTPCreateSymlink, symlinkActionTarget("edge", "/new", "relative:リンク")
				body = mustMarshal(t, api.SFTPCreateSymlinkRequest{Path: "/new", Target: "relative:リンク"})
			} else if operation == "ownership" {
				endpoint, kind, target = "/api/v1/sftp/edge/ownership", session.ActionSFTPOwnership, "edge:/file:42:43"
				body = mustMarshal(t, api.SFTPOwnershipRequest{Path: "/file", Uid: 42, Gid: 43, ExpectedRevision: entry.Revision})
			}
			response := sendKeyRequest(t, engine, credentials, method, endpoint, body, "")
			if response.Code != http.StatusForbidden || problemCode(t, response.Body.Bytes()) != "action_token_required" || remote.mutations != 0 {
				t.Fatalf("missing confirmation = %d %s", response.Code, response.Body.String())
			}
			issue := func(target string) string {
				t.Helper()
				issued := sendKeyRequest(t, engine, credentials, http.MethodPost, "/api/v1/actions", mustMarshal(t, api.IssueActionRequest{Kind: kind, Target: target}), "")
				if issued.Code != http.StatusCreated {
					t.Fatalf("issue = %d %s", issued.Code, issued.Body.String())
				}
				var token api.IssueActionResponse
				if err := json.Unmarshal(issued.Body.Bytes(), &token); err != nil {
					t.Fatal(err)
				}
				return token.Token
			}
			wrong := issue(target + "x")
			response = sendKeyRequest(t, engine, credentials, method, endpoint, body, wrong)
			if response.Code != http.StatusForbidden || remote.mutations != 0 {
				t.Fatalf("wrong target allowed: %d %s", response.Code, response.Body.String())
			}
			stale := issue(target)
			originalFile := remote.infos["/file"]
			switch operation {
			case "create":
				remote.infos["/new"] = originalFile
			case "retarget":
				remote.targets["/link"] = "/else"
			case "ownership":
				changed := originalFile
				changed.owner.UID++
				remote.infos["/file"] = changed
			}
			response = sendKeyRequest(t, engine, credentials, method, endpoint, body, stale)
			if response.Code < 400 || remote.mutations != 0 {
				t.Fatalf("changed evidence allowed: %d %s", response.Code, response.Body.String())
			}
			delete(remote.infos, "/new")
			remote.targets["/link"] = "/file"
			remote.infos["/file"] = originalFile
			denied := issue(target)
			remote.mutationErr = fs.ErrPermission
			response = sendKeyRequest(t, engine, credentials, method, endpoint, body, denied)
			if response.Code != http.StatusForbidden || problemCode(t, response.Body.Bytes()) != "sftp_permission_denied" || remote.mutations != 0 {
				t.Fatalf("permission refusal = %d %s", response.Code, response.Body.String())
			}
			remote.mutationErr = nil
			token := issue(target)
			response = sendKeyRequest(t, engine, credentials, method, endpoint, body, token)
			if response.Code != http.StatusOK && response.Code != http.StatusCreated {
				t.Fatalf("confirmed = %d %s", response.Code, response.Body.String())
			}
			count := remote.mutations
			response = sendKeyRequest(t, engine, credentials, method, endpoint, body, token)
			if response.Code < 400 || remote.mutations != count {
				t.Fatalf("replayed token = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestOwnershipHTTPRejectsMissingAndInvalidIDsBeforeMutation(t *testing.T) {
	engine := echo.New()
	registerSFTPRoutes(engine, SFTPHandlers{})
	for _, body := range []string{
		`{"path":"/file","expectedRevision":"revision","gid":1}`,
		`{"path":"/file","expectedRevision":"revision","uid":1}`,
		`{"path":"/file","expectedRevision":"revision","uid":-1,"gid":1}`,
		`{"path":"/file","expectedRevision":"revision","uid":4294967296,"gid":1}`,
		`{"path":"/file","expectedRevision":"revision","uid":null,"gid":1}`,
	} {
		response := sendKeyRequest(t, engine, session.Credentials{}, http.MethodPatch, "/api/v1/sftp/edge/ownership", []byte(body), "")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid IDs = %d %s", response.Code, response.Body.String())
		}
	}
}

func TestMetadataProblemCodesDistinguishUnsupportedMissingAttributesAndPermissions(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{sshcSFTP.ErrUnsupportedOperation, 501, "sftp_unsupported_operation"},
		{sshcSFTP.ErrMetadataUnavailable, 501, "sftp_metadata_unavailable"},
		{sshcSFTP.ErrOwnershipUnavailable, 501, "sftp_ownership_unavailable"},
		{fs.ErrPermission, 403, "sftp_permission_denied"},
		{sshcSFTP.ErrInvalidSpace, 502, "sftp_invalid_space"},
	} {
		engine := echo.New()
		engine.GET("/problem", func(c *echo.Context) error { return sftpProblem(c, errors.Join(test.err)) })
		response := sendKeyRequest(t, engine, session.Credentials{}, http.MethodGet, "/problem", nil, "")
		if response.Code != test.status || problemCode(t, response.Body.Bytes()) != test.code {
			t.Fatalf("problem = %d %s", response.Code, response.Body.String())
		}
	}
}
