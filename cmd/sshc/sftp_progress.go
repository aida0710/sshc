package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

type sftpCLIProgressJob struct {
	ID            string                `json:"id"`
	DownloadParts []sftpCLIDownloadPart `json:"downloadParts"`
}

type sftpCLIProgressDisplay struct {
	engine    *engineAPI
	output    io.Writer
	refreshMu sync.Mutex
	mu        sync.Mutex
	tracked   map[string]string
	jobs      map[string]sftpCLIProgressJob
	lines     int
}

func newSFTPCLIProgressDisplay(engine *engineAPI, output io.Writer, jsonOutput bool) *sftpCLIProgressDisplay {
	if jsonOutput || engine == nil {
		return nil
	}
	file, ok := output.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return nil
	}
	return &sftpCLIProgressDisplay{
		engine: engine, output: output, tracked: make(map[string]string), jobs: make(map[string]sftpCLIProgressJob),
	}
}

func (display *sftpCLIProgressDisplay) track(id, name string) {
	display.mu.Lock()
	display.tracked[id] = name
	display.mu.Unlock()
}

func (display *sftpCLIProgressDisplay) refresh(ctx context.Context) {
	if display == nil {
		return
	}
	display.refreshMu.Lock()
	defer display.refreshMu.Unlock()
	requestContext, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	var queue sftpCLITransferQueue
	if err := display.engine.sendJSON(requestContext, http.MethodGet, "/api/v1/sftp/transfers", nil, &queue); err != nil {
		return
	}
	display.mu.Lock()
	for _, encoded := range queue.Jobs {
		var job sftpCLIProgressJob
		if json.Unmarshal(encoded, &job) != nil || job.ID == "" || !validSFTPDownloadParts(job.DownloadParts) {
			continue
		}
		if _, tracked := display.tracked[job.ID]; tracked {
			display.jobs[job.ID] = job
		}
	}
	lines := sftpProgressLines(display.tracked, display.jobs)
	display.renderLocked(lines)
	display.mu.Unlock()
}

func (display *sftpCLIProgressDisplay) renderLocked(lines []string) {
	if len(lines) == 0 {
		return
	}
	if display.lines > 0 {
		fmt.Fprintf(display.output, "\x1b[%dA", display.lines)
	}
	for _, line := range lines {
		fmt.Fprintf(display.output, "\r\x1b[2K%s\n", line)
	}
	display.lines = len(lines)
}

func sftpProgressLines(tracked map[string]string, jobs map[string]sftpCLIProgressJob) []string {
	ids := make([]string, 0, len(tracked))
	for id := range tracked {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lines := make([]string, 0)
	for _, id := range ids {
		job, found := jobs[id]
		if !found || len(job.DownloadParts) == 0 {
			lines = append(lines, fmt.Sprintf("preparing  %s", tracked[id]))
			continue
		}
		parts := append([]sftpCLIDownloadPart(nil), job.DownloadParts...)
		sort.Slice(parts, func(i, j int) bool { return parts[i].Index < parts[j].Index })
		for _, part := range parts {
			lines = append(lines, formatSFTPProgressLine(part, tracked[id]))
		}
	}
	return lines
}

func formatSFTPProgressLine(part sftpCLIDownloadPart, name string) string {
	const width = 24
	percent := 0
	if part.TotalBytes > 0 {
		percent = int(min(int64(100), part.TransferredBytes*100/part.TotalBytes))
	}
	filled := percent * width / 100
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	return fmt.Sprintf("connection %d [%s] %3d%%  %s / %s  %s",
		part.Index+1, bar, percent, sftpProgressBytes(part.TransferredBytes), sftpProgressBytes(part.TotalBytes), compactSFTPProgressName(name))
}

func compactSFTPProgressName(name string) string {
	const maximum = 28
	characters := []rune(name)
	if len(characters) <= maximum {
		return name
	}
	return "…" + string(characters[len(characters)-maximum+1:])
}

func sftpProgressBytes(value int64) string {
	const (
		kib = int64(1 << 10)
		mib = int64(1 << 20)
		gib = int64(1 << 30)
	)
	switch {
	case value >= gib:
		return fmt.Sprintf("%.1f GiB", float64(value)/float64(gib))
	case value >= mib:
		return fmt.Sprintf("%.1f MiB", float64(value)/float64(mib))
	case value >= kib:
		return fmt.Sprintf("%.1f KiB", float64(value)/float64(kib))
	default:
		return fmt.Sprintf("%d B", value)
	}
}
