package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/terminal"
	"sshc/internal/terminalworkspace"
)

func registerTerminalWorkspaceRoutes(engine *echo.Echo, handlers TerminalHandlers) {
	engine.GET("/api/v1/terminal/workspaces", handlers.ListWorkspaces)
	engine.POST("/api/v1/terminal/workspaces", handlers.CreateWorkspace)
	engine.PATCH("/api/v1/terminal/workspaces/:id", handlers.UpdateWorkspace)
	engine.DELETE("/api/v1/terminal/workspaces/:id", handlers.DeleteWorkspace)
	engine.PUT("/api/v1/terminal/workspaces/:id/position", handlers.MoveWorkspace)
	engine.PUT("/api/v1/terminal/sessions/:id/workspace", handlers.MoveSession)
}

func describeWorkspace(workspace terminalworkspace.Workspace) api.TerminalWorkspace {
	return api.TerminalWorkspace{
		Id: workspace.ID, Name: workspace.Name, Hidden: workspace.Hidden, BuiltIn: workspace.BuiltIn(),
	}
}

func (h TerminalHandlers) workspaceList() (api.TerminalWorkspaceList, error) {
	workspaces, err := h.Workspaces.List()
	if err != nil {
		return api.TerminalWorkspaceList{}, err
	}
	described := make([]api.TerminalWorkspace, 0, len(workspaces))
	for _, workspace := range workspaces {
		described = append(described, describeWorkspace(workspace))
	}
	return api.TerminalWorkspaceList{Workspaces: described, MaxWorkspaces: terminalworkspace.MaxWorkspaces}, nil
}

// openingWorkspace は、開くセッションを入れるワークスペースを決める。指定が無ければ
// default に入れる。指定されたワークスペースが無ければ断る。
func (h TerminalHandlers) openingWorkspace(requested *string) (string, error) {
	if requested == nil {
		return terminalworkspace.DefaultID, nil
	}
	if err := h.requireWorkspace(*requested); err != nil {
		return "", err
	}
	return *requested, nil
}

func (h TerminalHandlers) requireWorkspace(id string) error {
	if h.Workspaces == nil {
		if id == terminalworkspace.DefaultID {
			return nil
		}
		return terminalworkspace.ErrNotFound
	}
	exists, err := h.Workspaces.Exists(id)
	if err != nil {
		return err
	}
	if !exists {
		return terminalworkspace.ErrNotFound
	}
	return nil
}

func (h TerminalHandlers) ListWorkspaces(c *echo.Context) error {
	list, err := h.workspaceList()
	if err != nil {
		return terminalWorkspaceProblem(c, err)
	}
	return c.JSON(http.StatusOK, list)
}

func (h TerminalHandlers) CreateWorkspace(c *echo.Context) error {
	var request api.CreateTerminalWorkspaceRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	created, err := h.Workspaces.Create(request.Name, request.Hidden != nil && *request.Hidden)
	if err != nil {
		return terminalWorkspaceProblem(c, err)
	}
	return c.JSON(http.StatusCreated, describeWorkspace(created))
}

func (h TerminalHandlers) UpdateWorkspace(c *echo.Context) error {
	id, ok := identifierParam(c, "id")
	if !ok {
		return problem(c, http.StatusNotFound, "terminal_workspace_not_found")
	}
	var request api.UpdateTerminalWorkspaceRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	updated, err := h.Workspaces.Update(id, terminalworkspace.Change{Name: request.Name, Hidden: request.Hidden})
	if err != nil {
		return terminalWorkspaceProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeWorkspace(updated))
}

// DeleteWorkspace は、ワークスペースを消す。中のセッションは閉じずに default へ移す。
func (h TerminalHandlers) DeleteWorkspace(c *echo.Context) error {
	id, ok := identifierParam(c, "id")
	if !ok {
		return problem(c, http.StatusNotFound, "terminal_workspace_not_found")
	}
	if err := h.Workspaces.Delete(id, h.Registry.MoveWorkspaceSessions); err != nil {
		return terminalWorkspaceProblem(c, err)
	}
	list, err := h.workspaceList()
	if err != nil {
		return terminalWorkspaceProblem(c, err)
	}
	return c.JSON(http.StatusOK, list)
}

// MoveWorkspace は、ワークスペースを beforeId の前へ、無ければ末尾へ移し、新しい並びを返す。
func (h TerminalHandlers) MoveWorkspace(c *echo.Context) error {
	id, ok := identifierParam(c, "id")
	if !ok {
		return problem(c, http.StatusNotFound, "terminal_workspace_not_found")
	}
	var request api.MoveTerminalWorkspaceRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	beforeID := ""
	if request.BeforeId != nil {
		beforeID = *request.BeforeId
	}
	if err := h.Workspaces.Move(id, beforeID); err != nil {
		return terminalWorkspaceProblem(c, err)
	}
	list, err := h.workspaceList()
	if err != nil {
		return terminalWorkspaceProblem(c, err)
	}
	return c.JSON(http.StatusOK, list)
}

func (h TerminalHandlers) MoveSession(c *echo.Context) error {
	id, ok := sessionIDParam(c)
	if !ok {
		return problem(c, http.StatusNotFound, "terminal_session_not_found")
	}
	var request api.MoveTerminalSessionRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.requireWorkspace(request.WorkspaceId); err != nil {
		return terminalWorkspaceProblem(c, err)
	}
	if err := h.Registry.MoveToWorkspace(id, request.WorkspaceId); err != nil {
		if errors.Is(err, terminal.ErrNotFound) {
			return problem(c, http.StatusNotFound, "terminal_session_not_found")
		}
		return unexpectedProblem(c, "terminal_workspace_failed", err)
	}
	return c.JSON(http.StatusOK, h.list())
}

func terminalWorkspaceProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, terminalworkspace.ErrNotFound):
		return problem(c, http.StatusNotFound, "terminal_workspace_not_found")
	case errors.Is(err, terminalworkspace.ErrInvalidName):
		return problem(c, http.StatusBadRequest, "invalid_terminal_workspace_name")
	case errors.Is(err, terminalworkspace.ErrLimit):
		return problem(c, http.StatusConflict, "terminal_workspace_limit")
	case errors.Is(err, terminalworkspace.ErrBuiltIn):
		return problem(c, http.StatusConflict, "terminal_workspace_built_in")
	case errors.Is(err, terminalworkspace.ErrUnsupportedVersion):
		return problem(c, http.StatusConflict, "terminal_workspace_newer_schema")
	}
	return unexpectedProblem(c, "terminal_workspace_failed", err)
}
