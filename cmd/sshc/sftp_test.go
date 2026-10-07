package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/api"
	"sshc/internal/httpserver"
	sftpcore "sshc/internal/sftp"
)

func TestSFTPFileWorkersBoundParallelFiles(t *testing.T) {
	files := []sftpCLIFile{{Source: "one"}, {Source: "two"}, {Source: "three"}, {Source: "four"}}
	started := make(chan struct{}, len(files))
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	finished := make(chan error, 1)
	go func() {
		finished <- runSFTPFileWorkers(t.Context(), files, 3, func(context.Context, sftpCLIFile) error {
			current := active.Add(1)
			for observed := maximum.Load(); current > observed && !maximum.CompareAndSwap(observed, current); observed = maximum.Load() {
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
			return nil
		})
	}()
	for range 3 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("three workers did not start")
		}
	}
	select {
	case <-started:
		t.Fatal("a fourth file started before a worker was available")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 3 {
		t.Fatalf("maximum active files = %d, want 3", maximum.Load())
	}
}

func TestSFTPProgressUsesOneLinePerDownloadConnection(t *testing.T) {
	state := sftpCLIProgressState{files: 1, active: []sftpCLIActiveDownload{{id: "get_12345678", name: "archive.tar.gz"}}}
	state.record(httpserver.SFTPTransferJob{
		ID: "get_12345678",
		DownloadParts: []httpserver.SFTPDownloadPartProgress{
			{Index: 1, TransferredBytes: 16 << 20, TotalBytes: 32 << 20},
			{Index: 0, TransferredBytes: 32 << 20, TotalBytes: 32 << 20},
			{Index: 3, TransferredBytes: 0, TotalBytes: 16 << 20},
			{Index: 2, TransferredBytes: 8 << 20, TotalBytes: 32 << 20},
		},
	})
	lines := state.lines()
	if len(lines) != 4 {
		t.Fatalf("progress lines = %d, want 4: %q", len(lines), lines)
	}
	for index, line := range lines {
		want := fmt.Sprintf("connection %d ", index+1)
		if !strings.HasPrefix(line, want) || !strings.HasSuffix(line, "archive.tar.gz") {
			t.Errorf("line %d = %q", index, line)
		}
	}
	if !strings.Contains(lines[0], "100%") || !strings.Contains(lines[1], " 50%") ||
		!strings.Contains(lines[2], " 25%") || !strings.Contains(lines[3], "  0%") {
		t.Fatalf("progress percentages = %q", lines)
	}
}

func TestSFTPProgressDrawsOnlyDownloadsInProgressAndCountsFinishedOnes(t *testing.T) {
	var output strings.Builder
	display := &sftpCLIProgressDisplay{output: &output, state: sftpCLIProgressState{files: 3}}
	display.track("get_b", "first.txt")
	display.track("get_a", "second.txt")
	display.mu.Lock()
	display.state.record(httpserver.SFTPTransferJob{ID: "get_b", DownloadParts: []httpserver.SFTPDownloadPartProgress{{Index: 0, TransferredBytes: 1, TotalBytes: 2}}})
	lines := display.state.lines()
	display.mu.Unlock()
	if len(lines) != 3 || lines[0] != "finished   0 / 3 files" ||
		!strings.HasPrefix(lines[1], "connection 1 ") || !strings.HasSuffix(lines[1], "first.txt") ||
		lines[2] != "preparing  second.txt" {
		t.Fatalf("lines before a download finished = %q", lines)
	}

	display.untrack("get_b")
	display.mu.Lock()
	display.state.record(httpserver.SFTPTransferJob{ID: "get_b", DownloadParts: []httpserver.SFTPDownloadPartProgress{{Index: 0, TransferredBytes: 2, TotalBytes: 2}}})
	lines = display.state.lines()
	display.mu.Unlock()
	if len(lines) != 2 || lines[0] != "finished   1 / 3 files" || lines[1] != "preparing  second.txt" {
		t.Fatalf("lines after a download finished = %q", lines)
	}
}

func TestSFTPProgressClearsTheLinesOfAShorterDrawing(t *testing.T) {
	var output strings.Builder
	display := &sftpCLIProgressDisplay{output: &output}
	display.renderLocked([]string{"one", "two", "three"})
	output.Reset()
	display.renderLocked([]string{"one"})
	if got := output.String(); got != "\x1b[3A\r\x1b[2Kone\n\r\x1b[J" {
		t.Fatalf("shorter drawing = %q", got)
	}
	output.Reset()
	display.renderLocked(nil)
	if got := output.String(); got != "\x1b[1A\r\x1b[J" || display.lines != 0 {
		t.Fatalf("empty drawing = %q, lines=%d", got, display.lines)
	}
}

func TestSFTPDownloadProgressAcceptsEverySupportedConnection(t *testing.T) {
	parts := make([]httpserver.SFTPDownloadPartProgress, sftpcore.MaxLargeFileParallelism)
	for index := range parts {
		parts[index] = httpserver.SFTPDownloadPartProgress{Index: index, TotalBytes: 1}
	}
	if !validSFTPDownloadParts(parts) {
		t.Fatalf("%d download progress parts were rejected", len(parts))
	}
	parts = append(parts, httpserver.SFTPDownloadPartProgress{Index: sftpcore.MaxLargeFileParallelism, TotalBytes: 1})
	if validSFTPDownloadParts(parts) {
		t.Fatalf("%d download progress parts were accepted", len(parts))
	}
}

func TestSFTPDownloadPollsConnectionProgressWhileEnginePreparesFile(t *testing.T) {
	var progressRequests atomic.Int32
	server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/sftp/transfers":
			progressRequests.Add(1)
			writeTestJSON(response, http.StatusOK, map[string]any{
				"maxConcurrent": 2, "clearCompletedAfterSeconds": 0, "processingStopped": false,
				"largeFileThresholdBytes": 100 << 20, "largeFileParallelism": 4, "largeFileChunkBytes": 32 << 20,
				"jobs": []map[string]any{{
					"id": "get_12345678",
					"downloadParts": []map[string]any{
						{"index": 0, "transferredBytes": 8 << 20, "totalBytes": 32 << 20},
						{"index": 1, "transferredBytes": 16 << 20, "totalBytes": 32 << 20},
					},
				}},
			})
		case "/download":
			time.Sleep(180 * time.Millisecond)
			response.Header().Set("ETag", `"revision"`)
			response.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s", request.URL.Path)
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	engine := testSFTPEngine(server)
	var output strings.Builder
	display := &sftpCLIProgressDisplay{engine: engine, output: &output, state: sftpCLIProgressState{files: 1}}
	display.track("get_12345678", "large.bin")
	response, err := waitForSFTPDownloadResponse(t.Context(), engine, "/download", display)
	if err != nil {
		t.Fatal(err)
	}
	discardEngineResponse(response)
	if progressRequests.Load() < 2 {
		t.Fatalf("progress requests = %d, want at least 2", progressRequests.Load())
	}
	if rendered := output.String(); !strings.Contains(rendered, "connection 1") || !strings.Contains(rendered, "connection 2") {
		t.Fatalf("rendered progress = %q", rendered)
	}
}

func testSFTPEngine(server *httptest.Server) *engineAPI {
	return &engineAPI{
		origin: server.URL, csrf: "csrf", cookie: http.Cookie{Name: httpserver.SessionCookie, Value: "session"}, client: server.Client(),
	}
}

func TestSFTPSettingsShowsAndPersistsSplitDefaults(t *testing.T) {
	settings := httpserver.SFTPTransferJobList{
		MaxConcurrent: 3, ClearCompletedAfterSeconds: 300, ProcessingStopped: true,
		LargeFileThresholdBytes: 100 << 20, LargeFileParallelism: 4, LargeFileChunkBytes: 32 << 20,
		SpeedLimitBytesPerSecond: 2048 << 10, AutoReconnect: true, MaxReconnectAttempts: 3,
		Jobs: []httpserver.SFTPTransferJob{},
	}
	var updates int
	server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/sftp/transfers":
			writeTestJSON(response, http.StatusOK, settings)
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/sftp/transfers/settings":
			updates++
			if err := json.NewDecoder(request.Body).Decode(&settings); err != nil {
				t.Errorf("decode settings: %v", err)
			}
			settings.Jobs = []httpserver.SFTPTransferJob{}
			writeTestJSON(response, http.StatusOK, settings)
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.String())
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	var stdout, stderr strings.Builder
	called := sftpInvocation{Action: sftpSettings, SplitSizeMiB: 73, SplitJobs: 7, ChunkSizeMiB: 41}
	if code := runSFTPSettings(context.Background(), testSFTPEngine(server), called, &stdout, &stderr); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	if updates != 1 || settings.MaxConcurrent != 3 || settings.ClearCompletedAfterSeconds != 300 || !settings.ProcessingStopped ||
		settings.LargeFileThresholdBytes != 73<<20 || settings.LargeFileParallelism != 7 || settings.LargeFileChunkBytes != 41<<20 ||
		settings.SpeedLimitBytesPerSecond != 2048<<10 || !settings.AutoReconnect || settings.MaxReconnectAttempts != 3 {
		t.Fatalf("settings=%+v updates=%d", settings, updates)
	}
	if got := stdout.String(); got != "split-size  73 MiB\nsplit-jobs  7\nchunk-size  41 MiB\nspeed-limit  2048 KiB/s (0: unlimited)\nauto-reconnect  true\nreconnect-attempts  3\n" {
		t.Fatalf("stdout=%q", got)
	}

	stdout.Reset()
	called = sftpInvocation{Action: sftpSettings, JSON: true}
	if code := runSFTPSettings(context.Background(), testSFTPEngine(server), called, &stdout, &stderr); code != 0 {
		t.Fatalf("json code=%d stderr=%q", code, stderr.String())
	}
	if updates != 1 || !strings.Contains(stdout.String(), `"splitSizeMiB":73`) || !strings.Contains(stdout.String(), `"splitJobs":7`) ||
		!strings.Contains(stdout.String(), `"chunkSizeMiB":41`) {
		t.Fatalf("json stdout=%q updates=%d", stdout.String(), updates)
	}
}

func TestSFTPDownloadUsesRemotePathAndPublishesAtomically(t *testing.T) {
	var createdRemotePath string
	var checkpointOffset float64
	var splitThreshold, splitJobs, chunkBytes float64
	server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/sftp/transfers":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			createdRemotePath, _ = body["remotePath"].(string)
			splitThreshold, _ = body["largeFileThresholdBytes"].(float64)
			splitJobs, _ = body["largeFileParallelism"].(float64)
			chunkBytes, _ = body["largeFileChunkBytes"].(float64)
			writeTestJSON(response, http.StatusCreated, map[string]any{})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/actions"):
			writeTestJSON(response, http.StatusOK, map[string]any{})
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/download"):
			if request.URL.Query().Get("path") != "/remote/file.txt" {
				t.Errorf("download path = %q", request.URL.Query().Get("path"))
			}
			response.Header().Set("ETag", `"revision-one"`)
			response.Header().Set("Content-Length", "3")
			_, _ = io.WriteString(response, "new")
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/download-checkpoint"):
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			checkpointOffset, _ = body["offset"].(float64)
			writeTestJSON(response, http.StatusOK, map[string]any{})
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.String())
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "file.txt")
	batch := sftpTransferBatch{
		engine: testSFTPEngine(server), alias: "server-a", batchID: "batch_12345678",
		splitSizeMiB: 50, splitJobs: 6, chunkSizeMiB: 512,
	}
	err := sftpDownloadFile(context.Background(), batch, sftpCLIFile{
		Source: "/remote/file.txt", Destination: destination, Size: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(destination)
	if err != nil || string(contents) != "new" {
		t.Fatalf("downloaded = %q, %v", contents, err)
	}
	if createdRemotePath != "/remote/file.txt" || checkpointOffset != 3 || splitThreshold != 50<<20 || splitJobs != 6 || chunkBytes != 512<<20 {
		t.Fatalf("job remotePath=%q checkpoint=%v split=%v/%v chunk=%v", createdRemotePath, checkpointOffset, splitThreshold, splitJobs, chunkBytes)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(destination), ".sshc-sftp-*"))
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestSFTPDownloadRefusesAFileThatGrewAfterPlanning(t *testing.T) {
	server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/sftp/transfers":
			writeTestJSON(response, http.StatusCreated, map[string]any{})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/actions"):
			writeTestJSON(response, http.StatusOK, map[string]any{})
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/download"):
			response.Header().Set("ETag", `"revision-grew"`)
			_, _ = io.WriteString(response, "much larger than planned")
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.String())
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	directory := t.TempDir()
	destination := filepath.Join(directory, "file.txt")
	batch := sftpTransferBatch{engine: testSFTPEngine(server), alias: "server-a", batchID: "batch_12345678"}
	err := sftpDownloadFile(context.Background(), batch, sftpCLIFile{
		Source: "/remote/file.txt", Destination: destination, Size: 3,
	})
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("download error = %v, want %v", err, io.ErrUnexpectedEOF)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination exists after refused download: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(directory, ".sshc-sftp-*"))
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestSFTPRecursiveGetPlanHonorsTreeBudgets(t *testing.T) {
	tests := []struct {
		name     string
		budget   sftpCLIRecursiveBudget
		listings map[string][]api.SFTPEntry
		wantCall int32
	}{
		{
			name:   "entry count",
			budget: sftpCLIRecursiveBudget{maxDepth: 64, maxEntries: 2, maxBytes: 100, entries: 1},
			listings: map[string][]api.SFTPEntry{
				"/root": {
					{Name: "one", Path: "/root/one", Type: "file", Size: 1},
					{Name: "two", Path: "/root/two", Type: "file", Size: 1},
				},
			},
			wantCall: 1,
		},
		{
			name:   "total bytes",
			budget: sftpCLIRecursiveBudget{maxDepth: 64, maxEntries: 10, maxBytes: 10, entries: 1},
			listings: map[string][]api.SFTPEntry{
				"/root": {
					{Name: "one", Path: "/root/one", Type: "file", Size: 6},
					{Name: "two", Path: "/root/two", Type: "file", Size: 5},
				},
			},
			wantCall: 1,
		},
		{
			name:   "depth",
			budget: sftpCLIRecursiveBudget{maxDepth: 1, maxEntries: 10, maxBytes: 100, entries: 1},
			listings: map[string][]api.SFTPEntry{
				"/root":        {{Name: "nested", Path: "/root/nested", Type: "directory"}},
				"/root/nested": {{Name: "file", Path: "/root/nested/file", Type: "file", Size: 1}},
			},
			wantCall: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				remotePath := request.URL.Query().Get("path")
				entries, exists := test.listings[remotePath]
				if !exists {
					t.Errorf("unexpected listing path %q", remotePath)
					response.WriteHeader(http.StatusNotFound)
					return
				}
				writeTestJSON(response, http.StatusOK, httpserver.SFTPListing{Path: remotePath, Entries: entries})
			}))
			defer server.Close()

			plan := sftpCLIPlan{}
			budget := test.budget
			err := walkRemoteGetPlan(t.Context(), newRemoteListings(testSFTPEngine(server), "server-a"), "/root", t.TempDir(), 0, &budget, &plan)
			if !errors.Is(err, errSFTPRecursiveLimit) {
				t.Fatalf("walk error = %v, want %v", err, errSFTPRecursiveLimit)
			}
			if got := calls.Load(); got != test.wantCall {
				t.Fatalf("listing calls = %d, want %d", got, test.wantCall)
			}
		})
	}
}

func TestSFTPRecursiveGetPlanRejectsInvalidSizeAndPath(t *testing.T) {
	tests := []api.SFTPEntry{
		{Name: "negative", Path: "/root/negative", Type: "file", Size: -1},
		{Name: "escaped", Path: "/elsewhere/escaped", Type: "file", Size: 1},
	}
	for _, entry := range tests {
		t.Run(entry.Name, func(t *testing.T) {
			server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				writeTestJSON(response, http.StatusOK, httpserver.SFTPListing{Path: "/root", Entries: []api.SFTPEntry{entry}})
			}))
			defer server.Close()

			plan := sftpCLIPlan{}
			budget := sftpCLIRecursiveBudget{maxDepth: 64, maxEntries: 10, maxBytes: 100, entries: 1}
			err := walkRemoteGetPlan(t.Context(), newRemoteListings(testSFTPEngine(server), "server-a"), "/root", t.TempDir(), 0, &budget, &plan)
			if !errors.Is(err, errEngineInvalidResponse) {
				t.Fatalf("walk error = %v, want %v", err, errEngineInvalidResponse)
			}
		})
	}
}

// sftp get writes under the destination itself, without the os.Root the
// engine's get goes through, so it must refuse the same names up front.
func TestSFTPRecursiveGetPlanRefusesNamesTheEngineWouldNotWriteLocally(t *testing.T) {
	names := []string{"..", `a\b`, "a\x00b"}
	if runtime.GOOS == "windows" {
		names = append(names, "CON", "a:b", "a::$DATA")
	}
	for _, name := range names {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			entry := api.SFTPEntry{Name: name, Path: "/root/" + name, Type: "file", Size: 1}
			server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				writeTestJSON(response, http.StatusOK, httpserver.SFTPListing{Path: "/root", Entries: []api.SFTPEntry{entry}})
			}))
			defer server.Close()

			plan := sftpCLIPlan{}
			budget := sftpCLIRecursiveBudget{maxDepth: 64, maxEntries: 10, maxBytes: 100, entries: 1}
			err := walkRemoteGetPlan(t.Context(), newRemoteListings(testSFTPEngine(server), "server-a"), "/root", t.TempDir(), 0, &budget, &plan)
			if !errors.Is(err, errSFTPLocalName) {
				t.Fatalf("walk error = %v, want %v", err, errSFTPLocalName)
			}
			if len(plan.Files) != 0 || len(plan.Directories) != 0 {
				t.Fatalf("planned a write for a refused name: %+v", plan)
			}
		})
	}
}

func TestSFTPUploadStreamsChunksAndCarriesOverwritePolicy(t *testing.T) {
	var uploaded strings.Builder
	var overwrite bool
	server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/sftp/transfers":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			overwrite, _ = body["overwrite"].(bool)
			writeTestJSON(response, http.StatusCreated, map[string]any{})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/actions"):
			writeTestJSON(response, http.StatusOK, map[string]any{})
		case request.Method == http.MethodPost && strings.Contains(request.URL.Path, "/uploads/") && !strings.HasSuffix(request.URL.Path, "/complete"):
			writeTestJSON(response, http.StatusOK, map[string]any{
				"id": "put_12345678", "path": "/remote/file.txt", "offset": 0, "size": 7, "expectedRevision": "missing:rev",
			})
		case request.Method == http.MethodPatch && strings.Contains(request.URL.Path, "/uploads/"):
			contents, _ := io.ReadAll(request.Body)
			uploaded.Write(contents)
			writeTestJSON(response, http.StatusOK, map[string]any{
				"id": "put_12345678", "path": "/remote/file.txt", "offset": len(contents), "size": 7, "expectedRevision": "",
			})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/complete"):
			writeTestJSON(response, http.StatusCreated, map[string]any{"path": "/remote/file.txt", "bytes": 7, "revision": "revision-two"})
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.String())
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	source := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(source, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	batch := sftpTransferBatch{engine: testSFTPEngine(server), alias: "server-a", batchID: "batch_12345678", overwrite: true}
	err := sftpUploadFile(context.Background(), batch, sftpCLIFile{
		Source: source, Destination: "/remote/file.txt", Size: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if uploaded.String() != "payload" || !overwrite {
		t.Fatalf("uploaded=%q overwrite=%v", uploaded.String(), overwrite)
	}
}

func TestSFTPFingerprintMatchesTheBrowserTreeHash(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "fingerprint")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got, err := sftpFingerprint(context.Background(), file, 3)
	if err != nil {
		t.Fatal(err)
	}
	const want = "tree-sha256:e13dee54bfc1b26042c1fec4b1d8ef2054b22fa1ab1263adce665eb013f829d5"
	if got != want {
		t.Fatalf("fingerprint = %q, want %q", got, want)
	}
}

func TestSFTPFingerprintIsTheEngineFingerprintWhenReadsComeShort(t *testing.T) {
	// Several engine chunks long, and read from a pipe, which returns less
	// than a chunk per read, like some network and FUSE file systems do.
	contents := bytes.Repeat([]byte("0123456789abcdef"), (3<<20)/16)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	go func() {
		defer writer.Close()
		const pieceBytes = 100 << 10
		for start := 0; start < len(contents); start += pieceBytes {
			if _, err := writer.Write(contents[start:min(start+pieceBytes, len(contents))]); err != nil {
				return
			}
		}
	}()
	got, err := sftpFingerprint(t.Context(), reader, int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := sftpcore.SourceFingerprint(t.Context(), bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("fingerprint = %q, want the engine's %q", got, want)
	}
}

func TestSFTPFingerprintRefusesASourceThatChangedSizeAfterThePlan(t *testing.T) {
	for name, plannedSize := range map[string]int64{"grew": 3, "shrank": 5} {
		t.Run(name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "fingerprint")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if _, err := file.WriteString("abcd"); err != nil {
				t.Fatal(err)
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			if _, err := sftpFingerprint(t.Context(), file, plannedSize); !errors.Is(err, errLocalSourceChanged) {
				t.Fatalf("fingerprint of 4 bytes planned as %d = %v, want %v", plannedSize, err, errLocalSourceChanged)
			}
		})
	}
}

func TestValidSFTPUploadRangesRejectsUntrustedEngineProgress(t *testing.T) {
	const chunk = int64(8 << 20)
	tests := []struct {
		name        string
		ranges      []sftpcore.UploadRange
		transferred int64
		valid       bool
	}{
		{"empty", nil, 0, true},
		{"coalesced prefix", []sftpcore.UploadRange{{Offset: 0, Size: 2 * chunk}}, 2 * chunk, true},
		{"last partial", []sftpcore.UploadRange{{Offset: 2 * chunk, Size: chunk / 2}}, chunk / 2, true},
		{"overlap", []sftpcore.UploadRange{{Offset: 0, Size: 2 * chunk}, {Offset: chunk, Size: chunk}}, 3 * chunk, false},
		{"unaligned start", []sftpcore.UploadRange{{Offset: 1, Size: chunk}}, chunk, false},
		{"progress mismatch", []sftpcore.UploadRange{{Offset: 0, Size: chunk}}, 0, false},
		{"past total", []sftpcore.UploadRange{{Offset: 2 * chunk, Size: chunk}}, chunk, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validSFTPUploadRanges(test.ranges, test.transferred, 2*chunk+chunk/2, chunk); got != test.valid {
				t.Fatalf("validSFTPUploadRanges() = %v", got)
			}
		})
	}
}

func writeTestJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func TestSFTPWithoutARunningEngineReportsEngineNotRunning(t *testing.T) {
	for _, action := range []sftpAction{sftpPut, sftpGet, sftpSettings} {
		// The local source is missing too, so a stopped engine must not be
		// mistaken for it.
		called := sftpInvocation{
			Action: action, Alias: "web", JSON: true,
			Source: filepath.Join(t.TempDir(), "missing.txt"), Destination: "/srv/missing.txt",
		}
		var stdout, stderr strings.Builder
		environment := commandEnvironment{stateDir: t.TempDir(), client: &http.Client{}, stdout: &stdout, stderr: &stderr}
		if code := runSFTP(context.Background(), called, environment); code != 1 || stderr.Len() != 0 {
			t.Fatalf("action %d: code=%d stdout=%q stderr=%q", action, code, stdout.String(), stderr.String())
		}
		var envelope commandEnvelope
		if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Failure == nil || envelope.Failure.Kind != "engine_not_running" || !envelope.Failure.Retryable {
			t.Fatalf("action %d: failure envelope = %+v", action, envelope)
		}

		called.JSON = false
		stdout.Reset()
		runSFTP(context.Background(), called, environment)
		if !strings.Contains(stderr.String(), "sshc is not running") {
			t.Fatalf("action %d: stderr=%q", action, stderr.String())
		}
	}
}

func TestSFTPPutOfAMissingLocalSourceReportsLocalNotFound(t *testing.T) {
	server := engineTestServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		t.Errorf("unexpected request: %s %s", request.Method, request.URL.String())
		response.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	missing := filepath.Join(t.TempDir(), "missing.txt")
	called := sftpInvocation{Action: sftpPut, Alias: "web", Source: missing, Destination: "/srv/missing.txt"}
	_, err := buildSFTPPutPlan(context.Background(), testSFTPEngine(server), called)

	var stdout, stderr strings.Builder
	if code := finishSFTPFailure(true, err, &stdout, &stderr); code != 1 {
		t.Fatalf("code=%d", code)
	}
	var envelope commandEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Failure == nil || envelope.Failure.Kind != "local_not_found" || envelope.Failure.Retryable {
		t.Fatalf("failure envelope = %+v", envelope)
	}
	finishSFTPFailure(false, err, &stdout, &stderr)
	if !strings.Contains(stderr.String(), "the local source does not exist") || !strings.Contains(stderr.String(), missing) {
		t.Fatalf("stderr=%q", stderr.String())
	}
}
