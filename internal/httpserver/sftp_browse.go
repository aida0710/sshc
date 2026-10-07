package httpserver

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"sshc/internal/api"
	sshcSFTP "sshc/internal/sftp"
)

func (h SFTPHandlers) ListLocal(c *echo.Context) error {
	listing, err := sshcSFTP.ListLocal(c.QueryParam("path"))
	if err != nil {
		return sftpProblem(c, err)
	}
	entries := make([]api.SFTPEntry, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		entries = append(entries, describeSFTPEntry(entry))
	}
	return c.JSON(http.StatusOK, api.SFTPLocalListing{Path: listing.Path, Home: listing.Home, Entries: entries})
}

func (h SFTPHandlers) List(c *echo.Context) error {
	remotePath := c.QueryParam("path")
	listing, err := h.Service.ListDirectory(c.Request().Context(), c.Param("alias"), remotePath)
	if err != nil {
		return sftpProblem(c, err)
	}
	described := make([]api.SFTPEntry, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		described = append(described, describeSFTPEntry(entry))
	}
	return c.JSON(http.StatusOK, SFTPListing{Path: listing.Path, Entries: described})
}

// Search は、名前または内容の一致を返す。部分結果の理由は domain が決める。
func (h SFTPHandlers) Search(c *echo.Context) error {
	found, err := h.Service.Search(c.Request().Context(), sshcSFTP.SearchOptions{
		Alias: c.Param("alias"), Path: c.QueryParam("path"), Query: c.QueryParam("query"), Mode: sshcSFTP.SearchMode(c.QueryParam("mode")),
	})
	if err != nil {
		return sftpProblem(c, err)
	}
	described := make([]api.SFTPEntry, 0, len(found.Entries))
	for _, entry := range found.Entries {
		described = append(described, describeSFTPEntry(entry))
	}
	response := sftpSearchResponse{
		Path: found.Path, Query: found.Query, Truncated: found.Truncated, Entries: described,
	}
	if found.Mode == sshcSFTP.SearchContent {
		matches := make([]sftpContentMatchResponse, 0, len(found.Matches))
		for _, match := range found.Matches {
			matches = append(matches, sftpContentMatchResponse{Entry: describeSFTPEntry(match.Entry), Line: match.Line, Snippet: match.Snippet})
		}
		omissions := make([]sftpSearchOmissionResponse, 0, len(found.Omissions))
		for _, omission := range found.Omissions {
			omissions = append(omissions, sftpSearchOmissionResponse{Reason: omission.Reason, Count: omission.Count})
		}
		response.Matches, response.Omissions, response.BytesRead = &matches, &omissions, &found.BytesRead
	}
	return c.JSON(http.StatusOK, response)
}

func (h SFTPHandlers) DirectoryStats(c *echo.Context) error {
	stats, err := h.Service.DirectoryStats(c.Request().Context(), c.Param("alias"), c.QueryParam("path"))
	if err != nil {
		return sftpProblem(c, err)
	}
	return c.JSON(http.StatusOK, api.SFTPDirectoryStats{
		Path: stats.Path, Bytes: stats.Bytes, Files: stats.Files,
		Directories: stats.Directories, Truncated: stats.Truncated,
	})
}

func (h SFTPHandlers) CompareDirectories(c *echo.Context) error {
	comparison, err := h.Service.CompareDirectories(
		c.Request().Context(), sshcSFTP.CompareOptions{
			Left:  sshcSFTP.ComparisonLocation{Alias: c.QueryParam("leftAlias"), Path: c.QueryParam("leftPath")},
			Right: sshcSFTP.ComparisonLocation{Alias: c.QueryParam("rightAlias"), Path: c.QueryParam("rightPath")},
			Mode:  sshcSFTP.ComparisonMode(c.QueryParam("mode")),
		},
	)
	if err != nil {
		return sftpProblem(c, err)
	}
	entries := make([]api.SFTPDirectoryDifference, 0, len(comparison.Entries))
	for _, difference := range comparison.Entries {
		entry := api.SFTPDirectoryDifference{
			RelativePath: difference.RelativePath,
			Status:       api.SFTPDirectoryDifferenceStatus(difference.Status),
		}
		if difference.Left != nil {
			described := describeSFTPEntry(*difference.Left)
			entry.Left = &described
		}
		if difference.Right != nil {
			described := describeSFTPEntry(*difference.Right)
			entry.Right = &described
		}
		if difference.Omission != "" {
			entry.Omission = &difference.Omission
		}
		entries = append(entries, entry)
	}
	response := api.SFTPDirectoryComparison{
		LeftPath: comparison.LeftPath, RightPath: comparison.RightPath, Entries: entries,
	}
	if comparison.Mode == sshcSFTP.ComparisonContent {
		mode := api.SFTPComparisonMode(comparison.Mode)
		response.Mode, response.BytesRead, response.Truncated = &mode, &comparison.BytesRead, &comparison.Truncated
	}
	return c.JSON(http.StatusOK, response)
}
