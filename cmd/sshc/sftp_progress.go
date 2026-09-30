package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"sshc/internal/httpserver"

	"golang.org/x/term"
)

// sftpCLIProgressDisplay draws the downloads in progress on a terminal. A
// finished download leaves the drawing, so a recursive get of many files
// redraws only the files its workers are transferring.
type sftpCLIProgressDisplay struct {
	engine    *engineAPI
	output    io.Writer
	refreshMu sync.Mutex
	mu        sync.Mutex
	state     sftpCLIProgressState
	lines     int
}

// sftpCLIProgressState is what one drawing shows.
type sftpCLIProgressState struct {
	// active lists the downloads in progress in the order they started, so a
	// line keeps its place while its file transfers.
	active []sftpCLIActiveDownload
	jobs   map[string]httpserver.SFTPTransferJob
	// files is how many files this get transfers. With more than one, a first
	// line counts the finished ones.
	files    int
	finished int
}

type sftpCLIActiveDownload struct {
	id   string
	name string
}

func newSFTPCLIProgressDisplay(engine *engineAPI, output io.Writer, files int) *sftpCLIProgressDisplay {
	if engine == nil {
		return nil
	}
	file, ok := output.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return nil
	}
	return &sftpCLIProgressDisplay{engine: engine, output: output, state: sftpCLIProgressState{files: files}}
}

func (display *sftpCLIProgressDisplay) track(id, name string) {
	display.mu.Lock()
	display.state.active = append(display.state.active, sftpCLIActiveDownload{id: id, name: name})
	display.mu.Unlock()
}

// untrack takes a download that succeeded, failed or was canceled out of the
// drawing and counts it as finished.
func (display *sftpCLIProgressDisplay) untrack(id string) {
	display.mu.Lock()
	defer display.mu.Unlock()
	display.state.active = slices.DeleteFunc(display.state.active, func(download sftpCLIActiveDownload) bool {
		return download.id == id
	})
	delete(display.state.jobs, id)
	display.state.finished++
	display.renderLocked(display.state.lines())
}

// sftpProgressRequestTimeout は、進捗を 1 回尋ねる上限である。応答しない engine の
// 前で表示の更新を長く止めない。取りこぼした分は次の更新で取り直せる。
const sftpProgressRequestTimeout = 500 * time.Millisecond

func (display *sftpCLIProgressDisplay) refresh(ctx context.Context) {
	if display == nil {
		return
	}
	display.refreshMu.Lock()
	defer display.refreshMu.Unlock()
	requestContext, cancel := context.WithTimeout(ctx, sftpProgressRequestTimeout)
	defer cancel()
	var queue httpserver.SFTPTransferJobList
	if err := display.engine.sendJSON(requestContext, http.MethodGet, "/api/v1/sftp/transfers", nil, &queue); err != nil {
		return
	}
	display.mu.Lock()
	defer display.mu.Unlock()
	for _, job := range queue.Jobs {
		if job.ID == "" || !validSFTPDownloadParts(job.DownloadParts) {
			continue
		}
		display.state.record(job)
	}
	display.renderLocked(display.state.lines())
}

// renderLocked redraws over the previous drawing. Lines left over from a
// longer previous drawing are cleared.
func (display *sftpCLIProgressDisplay) renderLocked(lines []string) {
	if display.lines > 0 {
		fmt.Fprintf(display.output, "\x1b[%dA", display.lines)
	}
	for _, line := range lines {
		fmt.Fprintf(display.output, "\r\x1b[2K%s\n", line)
	}
	if len(lines) < display.lines {
		fmt.Fprint(display.output, "\r\x1b[J")
	}
	display.lines = len(lines)
}

// record keeps the engine's progress for a download still in progress. A job
// reported after its download finished is ignored.
func (state *sftpCLIProgressState) record(job httpserver.SFTPTransferJob) {
	if !slices.ContainsFunc(state.active, func(download sftpCLIActiveDownload) bool { return download.id == job.ID }) {
		return
	}
	if state.jobs == nil {
		state.jobs = make(map[string]httpserver.SFTPTransferJob)
	}
	state.jobs[job.ID] = job
}

func (state sftpCLIProgressState) lines() []string {
	lines := make([]string, 0, len(state.active)+1)
	if state.files > 1 {
		lines = append(lines, fmt.Sprintf("finished   %d / %d files", state.finished, state.files))
	}
	for _, download := range state.active {
		job, found := state.jobs[download.id]
		if !found || len(job.DownloadParts) == 0 {
			lines = append(lines, fmt.Sprintf("preparing  %s", download.name))
			continue
		}
		parts := append([]httpserver.SFTPDownloadPartProgress(nil), job.DownloadParts...)
		sort.Slice(parts, func(i, j int) bool { return parts[i].Index < parts[j].Index })
		for _, part := range parts {
			lines = append(lines, formatSFTPProgressLine(part, download.name))
		}
	}
	return lines
}

func formatSFTPProgressLine(part httpserver.SFTPDownloadPartProgress, name string) string {
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
