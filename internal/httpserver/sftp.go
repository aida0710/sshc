package httpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/labstack/echo/v5"

	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

type SFTPHandlers struct {
	Service   *sshcSFTP.Service
	Transfers *sshcSFTP.TransferManager
	Actions   ActionHandlers
}

func registerSFTPRoutes(engine *echo.Echo, handlers SFTPHandlers) {
	engine.GET("/api/v1/sftp/transfers", handlers.ListTransfers)
	engine.POST("/api/v1/sftp/transfers", handlers.CreateTransfer)
	engine.DELETE("/api/v1/sftp/transfers/finished", handlers.ClearFinishedTransfers)
	engine.DELETE("/api/v1/sftp/transfers/:id", handlers.RemoveTransfer)
	engine.PUT("/api/v1/sftp/transfers/settings", handlers.UpdateTransferSettings)
	engine.POST("/api/v1/sftp/transfers/:id/queue-position", handlers.MoveTransfer)
	engine.POST("/api/v1/sftp/transfers/:id/actions", handlers.UpdateTransfer)
	engine.POST("/api/v1/sftp/transfers/:id/download-checkpoint", handlers.CheckpointDownload)
	engine.GET("/api/v1/sftp/compare", handlers.CompareDirectories)
	engine.GET("/api/v1/sftp/local/entries", handlers.ListLocal)
	engine.GET("/api/v1/sftp/:alias/entries", handlers.List)
	engine.POST("/api/v1/sftp/:alias/entries", handlers.CreateEntry)
	engine.GET("/api/v1/sftp/:alias/stats", handlers.DirectoryStats)
	engine.GET("/api/v1/sftp/:alias/preview", handlers.Preview)
	engine.GET("/api/v1/sftp/:alias/search", handlers.Search)
	engine.GET("/api/v1/sftp/:alias/text", handlers.ReadText)
	engine.PUT("/api/v1/sftp/:alias/text", handlers.SaveText)
	engine.GET("/api/v1/sftp/:alias/download", handlers.Download)
	engine.GET("/api/v1/sftp/:alias/archive", handlers.DownloadArchive)
	engine.POST("/api/v1/sftp/:alias/uploads/:id", handlers.StartUpload)
	engine.PATCH("/api/v1/sftp/:alias/uploads/:id", handlers.AppendUpload)
	engine.POST("/api/v1/sftp/:alias/uploads/:id/complete", handlers.CompleteUpload)
	engine.DELETE("/api/v1/sftp/:alias/uploads/:id", handlers.CancelUpload)
	engine.PATCH("/api/v1/sftp/:alias/entry", handlers.Rename)
	engine.PATCH("/api/v1/sftp/:alias/mode", handlers.Chmod)
}

func addSFTPActions(registry actionRegistry, service *sshcSFTP.Service) {
	registry[session.ActionSFTPDelete] = actionKind{
		evidence: func(ctx context.Context, target string) (string, error) {
			alias, remotePath, ok := strings.Cut(target, ":")
			if !ok {
				return "", sshcSFTP.ErrInvalidPath
			}
			entry, err := service.Stat(ctx, alias, remotePath)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s:%s:%d", entry.Type, entry.Revision, entry.Size), nil
		},
		fail: sftpProblem,
	}
	registry[session.ActionSFTPChmod] = actionKind{
		evidence: func(ctx context.Context, target string) (string, error) {
			alias, remainder, ok := strings.Cut(target, ":")
			if !ok {
				return "", sshcSFTP.ErrInvalidPath
			}
			remainder = strings.TrimSuffix(remainder, ":recursive")
			separator := strings.LastIndexByte(remainder, ':')
			if separator <= 0 || separator == len(remainder)-1 {
				return "", sshcSFTP.ErrInvalidPath
			}
			entry, err := service.Stat(ctx, alias, remainder[:separator])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s:%s:%s", entry.Type, entry.Revision, entry.Mode.Perm()), nil
		},
		fail: sftpProblem,
	}
}
