package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	tracked := map[string]string{"get_12345678": "archive.tar.gz"}
	jobs := map[string]sftpCLIProgressJob{
		"get_12345678": {
			ID: "get_12345678",
			DownloadParts: []sftpCLIDownloadPart{
				{Index: 1, TransferredBytes: 16 << 20, TotalBytes: 32 << 20},
				{Index: 0, TransferredBytes: 32 << 20, TotalBytes: 32 << 20},
				{Index: 3, TransferredBytes: 0, TotalBytes: 16 << 20},
				{Index: 2, TransferredBytes: 8 << 20, TotalBytes: 32 << 20},
			},
		},
	}
	lines := sftpProgressLines(tracked, jobs)
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

func TestSFTPDownloadProgressAcceptsEverySupportedConnection(t *testing.T) {
	parts := make([]sftpCLIDownloadPart, sftpcore.MaxLargeFileParallelism)
	for index := range parts {
		parts[index] = sftpCLIDownloadPart{Index: index, TotalBytes: 1}
	}
	if !validSFTPDownloadParts(parts) {
		t.Fatalf("%d download progress parts were rejected", len(parts))
	}
	parts = append(parts, sftpCLIDownloadPart{Index: sftpcore.MaxLargeFileParallelism, TotalBytes: 1})
	if validSFTPDownloadParts(parts) {
		t.Fatalf("%d download progress parts were accepted", len(parts))
	}
}

func TestSFTPDownloadPollsConnectionProgressWhileEnginePreparesFile(t *testing.T) {
	var progressRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
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
	display := &sftpCLIProgressDisplay{
		engine: engine, output: &output, tracked: make(map[string]string), jobs: make(map[string]sftpCLIProgressJob),
	}
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
	settings := sftpCLITransferQueue{
		MaxConcurrent: 3, ClearCompletedAfterSeconds: 300, ProcessingStopped: true,
		LargeFileThresholdBytes: 100 << 20, LargeFileParallelism: 4, LargeFileChunkBytes: 32 << 20,
		Jobs: []json.RawMessage{},
	}
	var updates int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/sftp/transfers":
			writeTestJSON(response, http.StatusOK, settings)
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/sftp/transfers/settings":
			updates++
			if err := json.NewDecoder(request.Body).Decode(&settings); err != nil {
				t.Errorf("decode settings: %v", err)
			}
			settings.Jobs = []json.RawMessage{}
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
		settings.LargeFileThresholdBytes != 73<<20 || settings.LargeFileParallelism != 7 || settings.LargeFileChunkBytes != 41<<20 {
		t.Fatalf("settings=%+v updates=%d", settings, updates)
	}
	if got := stdout.String(); got != "split-size  73 MiB\nsplit-jobs  7\nchunk-size  41 MiB\n" {
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
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
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
	err := sftpDownloadFile(context.Background(), testSFTPEngine(server), "server-a", "batch_12345678", sftpCLIFile{
		Source: "/remote/file.txt", Destination: destination, Size: 3,
	}, 50, 6, 512, nil)
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

func TestSFTPUploadStreamsChunksAndCarriesOverwritePolicy(t *testing.T) {
	var uploaded strings.Builder
	var overwrite bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
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
	err := sftpUploadFile(context.Background(), testSFTPEngine(server), "server-a", "batch_12345678", sftpCLIFile{
		Source: source, Destination: "/remote/file.txt", Size: 7,
	}, true, 0, 0, 0)
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

func TestValidSFTPUploadRangesRejectsUntrustedEngineProgress(t *testing.T) {
	const chunk = int64(8 << 20)
	tests := []struct {
		name        string
		ranges      []sftpCLIUploadRange
		transferred int64
		valid       bool
	}{
		{"empty", nil, 0, true},
		{"coalesced prefix", []sftpCLIUploadRange{{Offset: 0, Size: 2 * chunk}}, 2 * chunk, true},
		{"last partial", []sftpCLIUploadRange{{Offset: 2 * chunk, Size: chunk / 2}}, chunk / 2, true},
		{"overlap", []sftpCLIUploadRange{{Offset: 0, Size: 2 * chunk}, {Offset: chunk, Size: chunk}}, 3 * chunk, false},
		{"unaligned start", []sftpCLIUploadRange{{Offset: 1, Size: chunk}}, chunk, false},
		{"progress mismatch", []sftpCLIUploadRange{{Offset: 0, Size: chunk}}, 0, false},
		{"past total", []sftpCLIUploadRange{{Offset: 2 * chunk, Size: chunk}}, chunk, false},
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
