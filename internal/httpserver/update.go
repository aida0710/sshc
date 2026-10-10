package httpserver

import (
	"errors"
	"net/http"
	"runtime"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/releasecheck"
	"sshc/internal/selfupdate"
	"sshc/internal/session"
)

// maxUpdateTargetLength matches the release tag bound in api/openapi.yaml.
const maxUpdateTargetLength = 128

// maxUpdateRequestBody bounds the JSON envelope around a single release tag.
const maxUpdateRequestBody = 1024

type UpdateHandlers struct {
	Current string
	Checker *releasecheck.Checker
	Service *selfupdate.Service
	Actions ActionHandlers
}

func registerUpdateRoutes(engine *echo.Echo, handlers *UpdateHandlers) {
	engine.GET("/api/v1/update", handlers.Check)
	engine.POST("/api/v1/update/preview", handlers.Preview)
	engine.POST("/api/v1/update", handlers.Start)
}

func (h *UpdateHandlers) Check(c *echo.Context) error {
	status := api.UpdateStatus{Current: h.Current, Available: false}
	canUpdate := false
	updating := false
	status.CanUpdate = &canUpdate
	reason := "update_unavailable"
	if runtime.GOOS == "android" || runtime.GOOS == "windows" {
		reason = "update_" + runtime.GOOS + "_unsupported"
	}
	if h.Service != nil {
		job, err := h.Service.Status()
		if err != nil {
			return updateProblem(c, err)
		}
		if job.ID != "" {
			wire := updateJobResponse(job)
			status.Job = &wire
		}
		if job.Active() {
			updating = true
			status.Latest = &job.Target
			status.Available = releasecheck.Newer(h.Current, job.Target)
			manager := api.UpdateStatusManager(job.Plan.Installation.Manager)
			status.Manager = &manager
			reason = "update_in_progress"
		} else if job.State == selfupdate.JobRestartRequired {
			reason = "update_restart_failed"
		} else if installation, err := h.Service.Inspect(c.Request().Context()); err != nil {
			reason = "update_unavailable"
			var failure selfupdate.Failure
			if errors.As(err, &failure) {
				reason = string(failure)
			}
		} else {
			manager := api.UpdateStatusManager(installation.Manager)
			status.Manager, reason = &manager, ""
			canUpdate = true
		}
	}
	status.Reason = &reason
	// Progress is device-local; polling an active job must not depend on GitHub.
	if h.Checker != nil && !updating {
		latest, err := h.Checker.Latest(c.Request().Context())
		if err == nil {
			status.Latest, status.PageUrl = &latest.Version, &latest.PageURL
			status.Available = releasecheck.Newer(h.Current, latest.Version)
		} else if !errors.Is(err, releasecheck.ErrNoRelease) && status.Job == nil {
			return problem(c, http.StatusBadGateway, "update_check_failed")
		}
	}
	canUpdate = canUpdate && status.Available
	return c.JSON(http.StatusOK, status)
}

func (h *UpdateHandlers) prepare(c *echo.Context) (selfupdate.Plan, error) {
	var body api.UpdateRequest
	if err := decodeJSONWithin(c, maxUpdateRequestBody, &body); err != nil || len(body.Target) > maxUpdateTargetLength {
		return selfupdate.Plan{}, selfupdate.Failure("invalid_request")
	}
	if h.Service == nil {
		return selfupdate.Plan{}, selfupdate.ErrUnavailable
	}
	return h.Service.Prepare(c.Request().Context(), body.Target)
}

func (h *UpdateHandlers) Preview(c *echo.Context) error {
	plan, err := h.prepare(c)
	if err != nil {
		return updateProblem(c, err)
	}
	token, allowed, response := h.Actions.issueEvidence(c, session.ActionUpdate, plan.Target, plan.Evidence())
	if !allowed {
		return response
	}
	return c.JSON(http.StatusOK, api.UpdatePreview{Current: plan.Current, Target: plan.Target,
		Manager: api.UpdatePreviewManager(plan.Installation.Manager), ActionToken: token.Token, ActionExpiresAt: token.ExpiresAt})
}

func (h *UpdateHandlers) Start(c *echo.Context) error {
	plan, err := h.prepare(c)
	if err != nil {
		return updateProblem(c, err)
	}
	allowed, response := h.Actions.consumeEvidence(c, session.ActionUpdate, plan.Target, plan.Evidence())
	if !allowed {
		return response
	}
	job, err := h.Service.Start(plan)
	if err != nil {
		return updateProblem(c, err)
	}
	err = c.JSON(http.StatusAccepted, updateJobResponse(job))
	if err == nil {
		err = http.NewResponseController(c.Response()).Flush()
	}
	h.Service.ResponseSent(job.ID, err)
	return err
}

func updateJobResponse(job selfupdate.Job) api.UpdateJob {
	response := api.UpdateJob{Id: job.ID, Target: job.Target, State: api.UpdateJobState(job.State), Problem: job.Problem}
	if job.InstalledVersion != "" {
		response.InstalledVersion = &job.InstalledVersion
	}
	return response
}

func updateProblem(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, selfupdate.Failure("invalid_request")):
		return problem(c, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, selfupdate.ErrState):
		return problem(c, http.StatusInternalServerError, "update_state_failed")
	case errors.Is(err, selfupdate.Failure("update_check_failed")):
		return problem(c, http.StatusBadGateway, "update_check_failed")
	case errors.Is(err, selfupdate.ErrPermission):
		return problem(c, http.StatusForbidden, "update_permission_denied")
	default:
		var failure selfupdate.Failure
		if !errors.As(err, &failure) {
			return problem(c, http.StatusConflict, "update_unavailable")
		}
		return problem(c, http.StatusConflict, string(failure))
	}
}
