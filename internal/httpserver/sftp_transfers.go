package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"sshc/internal/application"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

func (h SFTPHandlers) ListTransfers(c *echo.Context) error {
	return h.respondTransferQueue(c)
}

func (h SFTPHandlers) respondTransferQueue(c *echo.Context) error {
	queue, err := h.describeTransferQueue()
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, queue)
}

func (h SFTPHandlers) describeTransferQueue() (SFTPTransferJobList, error) {
	jobs, err := h.Transfers.ListJobs()
	if err != nil {
		return SFTPTransferJobList{}, err
	}
	described := make([]SFTPTransferJob, 0, len(jobs))
	for _, job := range jobs {
		described = append(described, describeTransferJob(job))
	}
	return SFTPTransferJobList{
		MaxConcurrent:              h.Transfers.MaxConcurrent(),
		LargeFileThresholdBytes:    h.Transfers.LargeFileThreshold(),
		LargeFileParallelism:       h.Transfers.LargeFileParallelism(),
		LargeFileChunkBytes:        h.Transfers.LargeFileChunkBytes(),
		SpeedLimitBytesPerSecond:   h.Transfers.SpeedLimitBytesPerSecond(),
		AutoReconnect:              h.Transfers.AutoReconnect(),
		MaxReconnectAttempts:       h.Transfers.MaxReconnectAttempts(),
		ExcludePatterns:            h.Transfers.ExcludePatterns(),
		ClearCompletedAfterSeconds: int(h.Transfers.ClearCompletedAfter() / time.Second),
		ProcessingStopped:          h.Transfers.ProcessingStopped(),
		Jobs:                       described,
	}, nil
}

// UpdateTransferSettings は、engine が持つ転送キューの設定を差し替える。
// 転送は engine の資源なので、設定も browser ごとではなく engine にひとつだけ
// 置く。metadata.json への保存は TransferManager が適用の前に行う
// （newTransferManager を参照）。
func (h SFTPHandlers) UpdateTransferSettings(c *echo.Context) error {
	var body sftpTransferSettingsRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	patterns := body.ExcludePatterns
	if patterns == nil {
		patterns = h.Transfers.ExcludePatterns()
	}
	settings := engineTransferSettings(application.FileTransferSettings{
		MaxConcurrent:              body.MaxConcurrent,
		ClearCompletedAfterSeconds: body.ClearCompletedAfterSeconds,
		ProcessingStopped:          body.ProcessingStopped,
		LargeFileThresholdBytes:    body.LargeFileThresholdBytes,
		LargeFileParallelism:       body.LargeFileParallelism,
		LargeFileChunkBytes:        body.LargeFileChunkBytes,
		SpeedLimitBytesPerSecond:   body.SpeedLimitBytesPerSecond,
		AutoReconnect:              body.AutoReconnect,
		MaxReconnectAttempts:       body.MaxReconnectAttempts,
		ExcludePatterns:            patterns,
	})
	if err := h.Transfers.SetTransferSettings(settings); err != nil {
		if errors.Is(err, sshcSFTP.ErrInvalidTransfer) {
			return sftpProblem(c, err)
		}
		// 範囲内の設定が断られるのは、metadata.json へ保存できなかったときである。
		return serviceProblem(c, err)
	}
	return h.respondTransferQueue(c)
}

// MoveTransfer は、待機中の job を待機列の中で入れ替える。
func (h SFTPHandlers) MoveTransfer(c *echo.Context) error {
	var body sftpTransferQueueMoveRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if err := h.Transfers.MoveQueuedJob(c.Param("id"), body.Move); err != nil {
		return sftpProblem(c, err)
	}
	return h.respondTransferQueue(c)
}

func (h SFTPHandlers) CreateTransfer(c *echo.Context) error {
	var body sftpCreateTransferJobRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	if body.Direction == sshcSFTP.TransferRemote && body.Operation == sshcSFTP.RemoteDelete {
		if allowed, response := h.Actions.consume(c, session.ActionSFTPDelete, body.Alias+":"+body.RemotePath); !allowed {
			return response
		}
	}
	job, err := h.Transfers.CreateJob(sshcSFTP.CreateTransferJob{
		ID: body.ID, BatchID: body.BatchID, BatchName: body.BatchName, BatchKind: body.BatchKind, Alias: body.Alias,
		SourceAlias: body.SourceAlias, SourcePath: body.SourcePath, Operation: body.Operation,
		Overwrite: body.Overwrite,
		Direction: body.Direction, Kind: body.Kind,
		Name: body.Name, RemotePath: body.RemotePath, TotalBytes: body.TotalBytes, LastModified: body.LastModified,
		LargeFileThresholdBytes: body.LargeFileThresholdBytes, LargeFileParallelism: body.LargeFileParallelism,
		LargeFileChunkBytes: body.LargeFileChunkBytes,
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, describeTransferJob(job))
}

func (h SFTPHandlers) ClearFinishedTransfers(c *echo.Context) error {
	if _, err := h.Transfers.ClearFinished(); err != nil {
		return sftpProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h SFTPHandlers) RemoveTransfer(c *echo.Context) error {
	if err := h.Transfers.RemoveJob(c.Param("id")); err != nil {
		return sftpProblem(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h SFTPHandlers) UpdateTransfer(c *echo.Context) error {
	var body sftpTransferJobActionRequest
	if err := decodeJSON(c, &body); err != nil {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	job, err := h.Transfers.UpdateJobFromClient(c.Param("id"), sshcSFTP.UpdateTransferJob{
		Action: body.Action, TransferredBytes: body.TransferredBytes,
		TotalBytes: body.TotalBytes, Problem: body.Problem, ResetProgress: body.ResetProgress,
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeTransferJob(job))
}
