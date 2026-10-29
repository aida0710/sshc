package httpserver

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"sshc/internal/api"
	"sshc/internal/terminal"
	"sshc/internal/terminalworkspace"
)

func decodeAs[T any](t *testing.T, body string) T {
	t.Helper()
	var decoded T
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return decoded
}

func (f *terminalFixture) sessionWorkspace(t *testing.T, id string) string {
	t.Helper()
	_, body := f.do(t, http.MethodGet, "/api/v1/terminal/sessions", "")
	for _, session := range decodeAs[api.TerminalSessionList](t, body).Sessions {
		if session.Id == id {
			return session.WorkspaceId
		}
	}
	t.Fatalf("session %s is not listed", id)
	return ""
}

func TestTheBuiltInWorkspacesAreListedAndTheAgentsOneIsHidden(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.DefaultLimits())
	response, body := fixture.do(t, http.MethodGet, "/api/v1/terminal/workspaces", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list = %d %s", response.StatusCode, body)
	}
	list := decodeAs[api.TerminalWorkspaceList](t, body)
	if len(list.Workspaces) != 2 || !list.Workspaces[1].Hidden || !list.Workspaces[1].BuiltIn {
		t.Fatalf("workspaces = %+v", list.Workspaces)
	}
}

func TestASessionOpensInTheDefaultWorkspaceUnlessOneIsNamed(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.DefaultLimits())
	id, _ := fixture.openShell(t)
	if got := fixture.sessionWorkspace(t, id); got != terminalworkspace.DefaultID {
		t.Fatalf("workspace = %q, want default", got)
	}
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"shell","workspaceId":"agents"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("open in agents = %d %s", response.StatusCode, body)
	}
	opened := decodeAs[api.OpenTerminalSessionResponse](t, body)
	if opened.Session.WorkspaceId != terminalworkspace.AgentsID {
		t.Fatalf("opened in %q", opened.Session.WorkspaceId)
	}
	response, body = fixture.do(t, http.MethodPost, "/api/v1/terminal/sessions", `{"kind":"shell","workspaceId":"missing"}`)
	if response.StatusCode != http.StatusNotFound || decodeAs[api.Problem](t, body).Code != "terminal_workspace_not_found" {
		t.Fatalf("open in a missing workspace = %d %s", response.StatusCode, body)
	}
}

func TestDeletingAWorkspaceKeepsItsSessionsInTheDefaultOne(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.DefaultLimits())
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/workspaces", `{"name":"sshc開発"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d %s", response.StatusCode, body)
	}
	created := decodeAs[api.TerminalWorkspace](t, body)
	id, _ := fixture.openShell(t)
	response, body = fixture.do(t, http.MethodPut, "/api/v1/terminal/sessions/"+id+"/workspace", `{"workspaceId":"`+created.Id+`"}`)
	if response.StatusCode != http.StatusOK || fixture.sessionWorkspace(t, id) != created.Id {
		t.Fatalf("move = %d %s", response.StatusCode, body)
	}

	response, body = fixture.do(t, http.MethodDelete, "/api/v1/terminal/workspaces/"+created.Id, "")
	if response.StatusCode != http.StatusOK || len(decodeAs[api.TerminalWorkspaceList](t, body).Workspaces) != 2 {
		t.Fatalf("delete = %d %s", response.StatusCode, body)
	}
	if got := fixture.sessionWorkspace(t, id); got != terminalworkspace.DefaultID {
		t.Fatalf("session after delete is in %q", got)
	}
}

func TestWorkspaceRefusalsCarryTheirOwnCodes(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.DefaultLimits())
	for _, tc := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{http.MethodDelete, "/api/v1/terminal/workspaces/default", "", http.StatusConflict, "terminal_workspace_built_in"},
		{http.MethodPost, "/api/v1/terminal/workspaces", `{"name":"   "}`, http.StatusBadRequest, "invalid_terminal_workspace_name"},
		{http.MethodPatch, "/api/v1/terminal/workspaces/missing", `{"hidden":true}`, http.StatusNotFound, "terminal_workspace_not_found"},
		{http.MethodPut, "/api/v1/terminal/workspaces/missing/position", `{}`, http.StatusNotFound, "terminal_workspace_not_found"},
		{http.MethodPut, "/api/v1/terminal/workspaces/default/position", `{"beforeId":"missing"}`, http.StatusNotFound, "terminal_workspace_not_found"},
		{http.MethodPut, "/api/v1/terminal/sessions/missing/workspace", `{"workspaceId":"default"}`, http.StatusNotFound, "terminal_session_not_found"},
	} {
		response, body := fixture.do(t, tc.method, tc.path, tc.body)
		if response.StatusCode != tc.status || decodeAs[api.Problem](t, body).Code != tc.code {
			t.Errorf("%s %s = %d %s, want %d %s", tc.method, tc.path, response.StatusCode, body, tc.status, tc.code)
		}
	}
}

func TestHidingAndRenamingAWorkspaceIsKept(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.DefaultLimits())
	response, body := fixture.do(t, http.MethodPatch, "/api/v1/terminal/workspaces/default", `{"name":"メイン","hidden":true}`)
	updated := decodeAs[api.TerminalWorkspace](t, body)
	if response.StatusCode != http.StatusOK || updated.Name != "メイン" || !updated.Hidden {
		t.Fatalf("update = %d %s", response.StatusCode, body)
	}
}

func TestAMovedWorkspaceIsListedInItsNewPlace(t *testing.T) {
	fixture := newTerminalFixture(t, terminal.DefaultLimits())
	response, body := fixture.do(t, http.MethodPost, "/api/v1/terminal/workspaces", `{"name":"sshc開発"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d %s", response.StatusCode, body)
	}
	created := decodeAs[api.TerminalWorkspace](t, body)
	response, body = fixture.do(t, http.MethodPut, "/api/v1/terminal/workspaces/"+created.Id+"/position", `{"beforeId":"default"}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("move = %d %s", response.StatusCode, body)
	}
	workspaceIDs := func(list api.TerminalWorkspaceList) []string {
		listed := make([]string, 0, len(list.Workspaces))
		for _, workspace := range list.Workspaces {
			listed = append(listed, workspace.Id)
		}
		return listed
	}
	want := []string{created.Id, terminalworkspace.DefaultID, terminalworkspace.AgentsID}
	if got := workspaceIDs(decodeAs[api.TerminalWorkspaceList](t, body)); !slices.Equal(got, want) {
		t.Fatalf("answered order = %v, want %v", got, want)
	}
	_, body = fixture.do(t, http.MethodGet, "/api/v1/terminal/workspaces", "")
	if got := workspaceIDs(decodeAs[api.TerminalWorkspaceList](t, body)); !slices.Equal(got, want) {
		t.Fatalf("listed order = %v, want %v", got, want)
	}
}
