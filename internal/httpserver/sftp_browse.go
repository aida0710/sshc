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

// Search は、あるディレクトリ配下の名前一致を返す。歩き切れなかったときは
// truncated がそう言う。
func (h SFTPHandlers) Search(c *echo.Context) error {
	found, err := h.Service.Search(c.Request().Context(), c.Param("alias"), c.QueryParam("path"), c.QueryParam("query"))
	if err != nil {
		return sftpProblem(c, err)
	}
	described := make([]api.SFTPEntry, 0, len(found.Entries))
	for _, entry := range found.Entries {
		described = append(described, describeSFTPEntry(entry))
	}
	return c.JSON(http.StatusOK, sftpSearchResponse{
		Path: found.Path, Query: found.Query, Truncated: found.Truncated, Entries: described,
	})
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
		c.Request().Context(), c.QueryParam("leftAlias"), c.QueryParam("leftPath"),
		c.QueryParam("rightAlias"), c.QueryParam("rightPath"),
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
		entries = append(entries, entry)
	}
	return c.JSON(http.StatusOK, api.SFTPDirectoryComparison{
		LeftPath: comparison.LeftPath, RightPath: comparison.RightPath, Entries: entries,
	})
}
