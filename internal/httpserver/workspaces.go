package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/workspace"
)

type WorkspaceHandlers struct{ Service *workspace.Service }

type workspaceDefinition struct {
	Name          string         `json:"name"`
	Layout        workspace.Node `json:"layout"`
	FocusedPaneID string         `json:"focusedPaneId"`
}

type workspaceListResponse struct {
	Workspaces []workspace.Workspace `json:"workspaces"`
}

func registerWorkspaceRoutes(engine *echo.Echo, handlers WorkspaceHandlers) {
	engine.GET("/api/v1/workspaces", handlers.List)
	engine.POST("/api/v1/workspaces", handlers.Create)
	engine.GET("/api/v1/workspaces/:id", handlers.Get)
	engine.PUT("/api/v1/workspaces/:id", handlers.Update)
	engine.DELETE("/api/v1/workspaces/:id", handlers.Delete)
	engine.POST("/api/v1/workspaces/:id/restore", handlers.Restore)
}

func workspaceProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, workspace.ErrNotFound):
		return problem(c, http.StatusNotFound, "workspace_not_found")
	case errors.Is(err, workspace.ErrLimit):
		return problem(c, http.StatusConflict, "workspace_limit")
	case errors.Is(err, workspace.ErrInvalidWorkspace):
		return problem(c, http.StatusBadRequest, "invalid_workspace")
	case errors.Is(err, workspace.ErrUnsupportedSchema):
		return problem(c, http.StatusConflict, "workspace_newer_schema")
	default:
		return problem(c, http.StatusInternalServerError, "workspace_failed")
	}
}

func definition(value workspaceDefinition) workspace.Definition {
	return workspace.Definition{Name: value.Name, Layout: value.Layout, FocusedPaneID: value.FocusedPaneID}
}

func (h WorkspaceHandlers) List(c *echo.Context) error {
	items, err := h.Service.List()
	if err != nil {
		return workspaceProblem(c, err)
	}
	return c.JSON(http.StatusOK, workspaceListResponse{Workspaces: items})
}

func (h WorkspaceHandlers) Get(c *echo.Context) error {
	item, err := h.Service.Get(c.Param("id"))
	if err != nil {
		return workspaceProblem(c, err)
	}
	return c.JSON(http.StatusOK, item)
}

func (h WorkspaceHandlers) Create(c *echo.Context) error {
	var request workspaceDefinition
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	item, err := h.Service.Create(definition(request))
	if err != nil {
		return workspaceProblem(c, err)
	}
	return c.JSON(http.StatusCreated, item)
}

func (h WorkspaceHandlers) Update(c *echo.Context) error {
	var request workspaceDefinition
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	item, err := h.Service.Update(c.Param("id"), definition(request))
	if err != nil {
		return workspaceProblem(c, err)
	}
	return c.JSON(http.StatusOK, item)
}

func (h WorkspaceHandlers) Delete(c *echo.Context) error {
	if err := h.Service.Delete(c.Param("id")); err != nil {
		return workspaceProblem(c, err)
	}
	return c.JSON(http.StatusOK, changedResponse{Changed: true})
}

func (h WorkspaceHandlers) Restore(c *echo.Context) error {
	plan, err := h.Service.Restore(c.Param("id"))
	if err != nil {
		return workspaceProblem(c, err)
	}
	return c.JSON(http.StatusOK, plan)
}
