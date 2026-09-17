package httpserver

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/remotesync"
)

// What the bucket holds (live object, history, diffs) and the exclusion
// rules that decide what a snapshot carries.

func exclusionsResponse(view remotesync.ExclusionView) api.SyncExclusions {
	response := api.SyncExclusions{
		Document: view.Document, UsingDefaults: view.UsingDefaults,
		Candidates: make([]api.SyncExclusionCandidate, 0, len(view.Candidates)),
	}
	for _, candidate := range view.Candidates {
		response.Candidates = append(response.Candidates, api.SyncExclusionCandidate{
			Path: candidate.Path, Ignored: candidate.Ignored,
		})
	}
	return response
}

func (h SyncHandlers) Exclusions(c *echo.Context) error {
	view, err := h.Service.Exclusions()
	if err != nil {
		return syncProblem(c, err)
	}
	return c.JSON(http.StatusOK, exclusionsResponse(view))
}

func (h SyncHandlers) SaveExclusions(c *echo.Context) error {
	var request api.SyncExclusionsRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	view, err := h.Service.SaveExclusions(request.Document)
	if err != nil {
		if errors.Is(err, remotesync.ErrInvalidIgnoreRules) {
			return problem(c, http.StatusBadRequest, "sync_ignore_invalid")
		}
		return syncProblem(c, err)
	}
	if h.Auto != nil {
		h.Auto.NotifyLocalChange()
	}
	return c.JSON(http.StatusOK, exclusionsResponse(view))
}

func (h SyncHandlers) Bucket(c *echo.Context) error {
	h.restore()
	view, err := h.Service.BucketStatus(c.Request().Context())
	if err != nil {
		return syncProblem(c, err)
	}
	response := api.SyncBucketStatus{
		CheckedAt: view.CheckedAt, History: make([]api.SyncBucketObject, 0, len(view.History)),
		HistoryTruncated: view.HistoryTruncated, LocalIsLive: view.LocalIsLive,
	}
	if view.Live != nil {
		response.Live = &api.SyncBucketObject{
			Key: view.Live.Key, Size: view.Live.Size, LastModified: syncStringPointer(view.Live.LastModified),
		}
	}
	for _, item := range view.History {
		response.History = append(response.History, api.SyncBucketObject{
			Key: item.Key, Size: item.Size, LastModified: syncStringPointer(item.LastModified),
		})
	}
	return c.JSON(http.StatusOK, response)
}

func (h SyncHandlers) History(c *echo.Context) error {
	h.restore()
	key, ok, err := h.sealingKey(c)
	if !ok {
		return err
	}
	view, err := h.Service.History(c.Request().Context(), key)
	if err != nil {
		return syncProblem(c, err)
	}
	response := api.SyncHistory{
		CheckedAt: view.CheckedAt, HeadRevision: view.HeadRevision,
		HistoryTruncated: view.HistoryTruncated, DownloadTruncated: view.DownloadTruncated,
		DownloadedBytes: view.DownloadedBytes,
		Skipped:         view.Skipped,
		Revisions:       make([]api.SyncHistoryRevision, 0, len(view.Revisions)),
	}
	for _, revision := range view.Revisions {
		response.Revisions = append(response.Revisions, api.SyncHistoryRevision{
			Key: revision.Key, Revision: revision.Revision,
			ParentRevision: syncStringPointer(revision.ParentRevision),
			Message:        revision.Message,
			CreatedAt:      revision.CreatedAt, Origin: revision.Origin,
			FileCount: revision.FileCount, Size: revision.Size,
			LastModified: syncStringPointer(revision.LastModified),
			Relation:     api.SyncHistoryRelation(revision.Relation),
		})
	}
	return c.JSON(http.StatusOK, response)
}

func (h SyncHandlers) HistoryDiff(c *echo.Context) error {
	h.restore()
	var request api.SyncHistoryDiffRequest
	if err := decodeJSON(c, &request); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if len(request.Key) == 0 || len(request.Key) > 1024 {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	key, ok, err := h.sealingKey(c)
	if !ok {
		return err
	}
	diff, err := h.Service.DiffHistory(c.Request().Context(), key, request.Key)
	if err != nil {
		return syncProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.SyncHistoryDiff{
		FromRevision: diff.FromRevision, ToRevision: diff.ToRevision,
		Added: diff.Added, Modified: diff.Modified, Removed: diff.Removed,
		DownloadedBytes: diff.DownloadedBytes,
	})
}
