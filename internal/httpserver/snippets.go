package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/session"
	"sshc/internal/snippets"
)

type SnippetHandlers struct {
	Service     *snippets.Service
	Actions     ActionHandlers
	BaseContext context.Context
}

type snippetDraft struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Command     string              `json:"command"`
	Variables   []snippets.Variable `json:"variables"`
}

type snippetLibrary struct {
	Snippets []snippets.Snippet `json:"snippets"`
	Startup  []snippets.Startup `json:"startup"`
}

func registerSnippetRoutes(engine *echo.Echo, handlers SnippetHandlers) {
	engine.GET("/api/v1/snippets", handlers.List)
	engine.POST("/api/v1/snippets", handlers.Create)
	engine.PUT("/api/v1/snippets/:id", handlers.Update)
	engine.DELETE("/api/v1/snippets/:id", handlers.Delete)
	engine.PUT("/api/v1/snippets/startup/:alias", handlers.SetStartup)
	engine.POST("/api/v1/snippets/preview", handlers.Preview)
	engine.POST("/api/v1/snippets/jobs", handlers.Start)
	engine.GET("/api/v1/snippets/jobs/:id", handlers.Job)
	engine.DELETE("/api/v1/snippets/jobs/:id", handlers.Cancel)
}

func snippetProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, snippets.ErrUnknownSnippet), errors.Is(err, snippets.ErrUnknownJob), errors.Is(err, snippets.ErrNoStartup):
		return problem(c, http.StatusNotFound, "snippet_not_found")
	case errors.Is(err, snippets.ErrPreviewChanged):
		return problem(c, http.StatusConflict, "snippet_preview_changed")
	case errors.Is(err, snippets.ErrTooManyJobs):
		return problem(c, http.StatusTooManyRequests, "snippet_jobs_full")
	case errors.Is(err, snippets.ErrJobFinished):
		return problem(c, http.StatusConflict, "snippet_job_finished")
	case errors.Is(err, snippets.ErrInvalidSnippet), errors.Is(err, snippets.ErrInvalidVariable), errors.Is(err, snippets.ErrUnknownVariable),
		errors.Is(err, snippets.ErrMissingVariable), errors.Is(err, snippets.ErrMalformedTemplate), errors.Is(err, snippets.ErrInvalidTarget),
		errors.Is(err, snippets.ErrDuplicateTarget), errors.Is(err, snippets.ErrSecretStartup):
		return problem(c, http.StatusBadRequest, "invalid_snippet")
	default:
		return problem(c, http.StatusInternalServerError, "snippet_failed")
	}
}

func toSnippetDraft(request snippetDraft) snippets.Draft {
	return snippets.Draft{Name: request.Name, Description: request.Description, Command: request.Command, Variables: request.Variables}
}

func (h SnippetHandlers) library() (snippetLibrary, error) {
	items, err := h.Service.List()
	if err != nil {
		return snippetLibrary{}, err
	}
	startup, err := h.Service.Startup()
	return snippetLibrary{Snippets: items, Startup: startup}, err
}

func (h SnippetHandlers) List(c *echo.Context) error {
	library, err := h.library()
	if err != nil {
		return snippetProblem(c, err)
	}
	return c.JSON(http.StatusOK, library)
}

func (h SnippetHandlers) Create(c *echo.Context) error {
	var request snippetDraft
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	created, err := h.Service.Create(toSnippetDraft(request))
	if err != nil {
		return snippetProblem(c, err)
	}
	return c.JSON(http.StatusCreated, created)
}

func (h SnippetHandlers) Update(c *echo.Context) error {
	var request snippetDraft
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	updated, err := h.Service.Update(c.Param("id"), toSnippetDraft(request))
	if err != nil {
		return snippetProblem(c, err)
	}
	return c.JSON(http.StatusOK, updated)
}

func (h SnippetHandlers) Delete(c *echo.Context) error {
	if err := h.Service.Delete(c.Param("id")); err != nil {
		return snippetProblem(c, err)
	}
	return c.JSON(http.StatusOK, map[string]bool{"changed": true})
}

func (h SnippetHandlers) SetStartup(c *echo.Context) error {
	var request struct {
		SnippetID string            `json:"snippetId"`
		Inputs    map[string]string `json:"inputs"`
	}
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Service.SetStartup(c.Param("alias"), request.SnippetID, request.Inputs); err != nil {
		return snippetProblem(c, err)
	}
	library, err := h.library()
	if err != nil {
		return snippetProblem(c, err)
	}
	return c.JSON(http.StatusOK, library)
}

func (h SnippetHandlers) Preview(c *echo.Context) error {
	var request snippets.PreviewRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	preview, err := h.Service.Preview(request)
	if err != nil {
		return snippetProblem(c, err)
	}
	issued, err := h.Actions.issueEvidence(c, session.ActionSnippetExecute, preview.SnippetID, preview.Evidence)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, struct {
		snippets.Preview
		ActionToken     string `json:"actionToken"`
		ActionExpiresAt string `json:"actionExpiresAt"`
	}{Preview: preview, ActionToken: issued.Token, ActionExpiresAt: issued.ExpiresAt})
}

func (h SnippetHandlers) Start(c *echo.Context) error {
	var request snippets.ExecuteRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	preview, err := h.Service.Preview(request.PreviewRequest)
	if err != nil {
		return snippetProblem(c, err)
	}
	if allowed, response := h.Actions.consumeEvidence(c, session.ActionSnippetExecute, preview.SnippetID, preview.Evidence); !allowed {
		return response
	}
	parent := h.BaseContext
	if parent == nil {
		parent = context.Background()
	}
	job, err := h.Service.Start(parent, request)
	if err != nil {
		return snippetProblem(c, err)
	}
	return c.JSON(http.StatusAccepted, job)
}

func (h SnippetHandlers) Job(c *echo.Context) error {
	job, err := h.Service.Job(c.Param("id"))
	if err != nil {
		return snippetProblem(c, err)
	}
	return c.JSON(http.StatusOK, job)
}

func (h SnippetHandlers) Cancel(c *echo.Context) error {
	if err := h.Service.Cancel(c.Param("id")); err != nil {
		return snippetProblem(c, err)
	}
	job, err := h.Service.Job(c.Param("id"))
	if err != nil {
		return snippetProblem(c, err)
	}
	return c.JSON(http.StatusAccepted, job)
}
