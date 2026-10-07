package httpserver

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	"sshc/internal/session"
	sshcSFTP "sshc/internal/sftp"
)

func (h SFTPHandlers) CreateSymlink(c *echo.Context) error {
	var body api.SFTPCreateSymlinkRequest
	if err := decodeJSON(c, &body); err != nil || body.Target == "" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	alias := c.Param("alias")
	unlock, err := h.Transfers.LockOperation(alias, body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer unlock()
	target := symlinkActionTarget(alias, body.Path, body.Target)
	if allowed, response := h.Actions.consume(c, session.ActionSFTPCreateSymlink, target); !allowed {
		return response
	}
	entry, err := h.Service.CreateSymlink(c.Request().Context(), alias, sshcSFTP.SymlinkChange{Path: body.Path, Target: body.Target})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusCreated, describeSFTPEntry(entry))
}

func (h SFTPHandlers) ChangeSymlink(c *echo.Context) error {
	var body api.SFTPChangeSymlinkRequest
	if err := decodeJSON(c, &body); err != nil || body.Target == "" || body.ExpectedRevision == "" {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	alias := c.Param("alias")
	unlock, err := h.Transfers.LockOperation(alias, body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer unlock()
	if allowed, response := h.Actions.consume(c, session.ActionSFTPSymlink, symlinkActionTarget(alias, body.Path, body.Target)); !allowed {
		return response
	}
	entry, err := h.Service.ChangeSymlink(c.Request().Context(), alias, sshcSFTP.SymlinkChange{Path: body.Path, Target: body.Target, ExpectedRevision: body.ExpectedRevision})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeSFTPEntry(entry))
}

func (h SFTPHandlers) ChangeOwnership(c *echo.Context) error {
	var body sftpOwnershipRequest
	if err := decodeJSON(c, &body); err != nil || body.ExpectedRevision == "" || !validSFTPOwnerID(body.UID) || !validSFTPOwnerID(body.GID) {
		return problem(c, http.StatusBadRequest, "invalid_request")
	}
	alias := c.Param("alias")
	unlock, err := h.Transfers.LockOperation(alias, body.Path)
	if err != nil {
		return sftpProblem(c, err)
	}
	defer unlock()
	target := fmt.Sprintf("%s:%s:%d:%d", alias, body.Path, *body.UID, *body.GID)
	if allowed, response := h.Actions.consume(c, session.ActionSFTPOwnership, target); !allowed {
		return response
	}
	entry, err := h.Service.ChangeOwnership(c.Request().Context(), alias, sshcSFTP.OwnershipChange{Path: body.Path, UID: uint32(*body.UID), GID: uint32(*body.GID), ExpectedRevision: body.ExpectedRevision})
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, describeSFTPEntry(entry))
}

func (h SFTPHandlers) FilesystemSpace(c *echo.Context) error {
	space, err := h.Service.FilesystemSpace(c.Request().Context(), c.Param("alias"), c.QueryParam("path"))
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.SFTPFilesystemSpace{Path: space.Path, AvailableBytes: strconv.FormatUint(space.AvailableBytes, 10), TotalBytes: strconv.FormatUint(space.TotalBytes, 10)})
}

func symlinkActionTarget(alias, path, target string) string {
	return alias + ":" + path + ":" + base64.RawURLEncoding.EncodeToString([]byte(target))
}

func metadataActionPath(target string, suffixCount int) (string, string, error) {
	alias, remainder, found := strings.Cut(target, ":")
	if !found {
		return "", "", sshcSFTP.ErrInvalidPath
	}
	for range suffixCount {
		separator := strings.LastIndexByte(remainder, ':')
		if separator <= 0 {
			return "", "", sshcSFTP.ErrInvalidPath
		}
		remainder = remainder[:separator]
	}
	return alias, remainder, nil
}

func addSFTPMetadataActions(registry actionRegistry, service *sshcSFTP.Service) {
	for _, kind := range []string{session.ActionSFTPCreateSymlink, session.ActionSFTPSymlink, session.ActionSFTPOwnership} {
		registry[kind] = actionKind{fail: sftpProblem, evidence: func(ctx context.Context, target string) (string, error) {
			suffixCount := 1
			if kind == session.ActionSFTPOwnership {
				suffixCount = 2
			}
			alias, remotePath, err := metadataActionPath(target, suffixCount)
			if err != nil {
				return "", err
			}
			entry, err := service.Stat(ctx, alias, remotePath)
			if kind == session.ActionSFTPCreateSymlink {
				if errors.Is(err, fs.ErrNotExist) {
					return "absent", nil
				}
				if err == nil {
					return "", sshcSFTP.ErrAlreadyExists
				}
			}
			if err != nil {
				return "", err
			}
			if kind == session.ActionSFTPSymlink && entry.Type != sshcSFTP.EntrySymlink {
				return "", sshcSFTP.ErrNotSymlink
			}
			if kind == session.ActionSFTPOwnership {
				if entry.Type != sshcSFTP.EntryFile && entry.Type != sshcSFTP.EntryDirectory {
					return "", sshcSFTP.ErrNotRegularFile
				}
				if entry.Ownership == nil {
					return "", sshcSFTP.ErrOwnershipUnavailable
				}
				return fmt.Sprintf("%s:%s:%d:%d", entry.Type, entry.Revision, entry.Ownership.UID, entry.Ownership.GID), nil
			}
			return entry.Revision, nil
		}}
	}
}

// Pointers distinguish omitted IDs from the valid numeric ID 0.
type sftpOwnershipRequest struct {
	Path             string `json:"path"`
	UID              *int64 `json:"uid"`
	GID              *int64 `json:"gid"`
	ExpectedRevision string `json:"expectedRevision"`
}

// SFTP v3 owner IDs are unsigned 32-bit integers.
const maxSFTPOwnerID = int64(1<<32 - 1)

func validSFTPOwnerID(value *int64) bool {
	return value != nil && *value >= 0 && *value <= maxSFTPOwnerID
}
