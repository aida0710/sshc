package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	sshcSFTP "sshc/internal/sftp"
)

func TestDownloadOffsetAcceptsOnlySingleOpenEndedRange(t *testing.T) {
	tests := []struct {
		header string
		size   int64
		want   int64
		ranged bool
		valid  bool
	}{
		{header: "", size: 10, want: 0, valid: true},
		{header: "bytes=4-", size: 10, want: 4, ranged: true, valid: true},
		{header: "bytes=10-", size: 10},
		{header: "bytes=-4", size: 10},
		{header: "bytes=1-2", size: 10},
		{header: "bytes=1-,4-", size: 10},
	}
	for _, test := range tests {
		offset, ranged, err := downloadOffset(test.header, test.size)
		if (err == nil) != test.valid || offset != test.want || ranged != test.ranged {
			t.Errorf("downloadOffset(%q, %d) = %d, %v, %v", test.header, test.size, offset, ranged, err)
		}
	}
}

func TestTransferTooLargeUsesAFileTransferProblemCode(t *testing.T) {
	engine := echo.New()
	engine.GET("/too-large", func(c *echo.Context) error {
		return sftpProblem(c, sshcSFTP.ErrTransferTooLarge)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/too-large", nil))
	if response.Code != http.StatusRequestEntityTooLarge ||
		!bytes.Contains(response.Body.Bytes(), []byte(`"code":"sftp_transfer_too_large"`)) {
		t.Fatalf("transfer too large = %d: %s", response.Code, response.Body.String())
	}
}

func TestTransferManagerHTTPContractAndSharedLimit(t *testing.T) {
	manager := sshcSFTP.NewTransferManager(nil)
	manager.ConfigureJobs(1, nil)
	engine := echo.New()
	registerSFTPRoutes(engine, SFTPHandlers{Transfers: manager})

	create := func(id string) map[string]any {
		body, err := json.Marshal(map[string]any{
			"id": id, "batchId": "batch_http001", "batchName": "HTTP batch", "batchKind": "file",
			"alias": "edge", "direction": "upload", "kind": "file", "name": id + ".bin",
			"remotePath": "/" + id + ".bin", "totalBytes": 10, "lastModified": 123,
		})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sftp/transfers", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("create %s = %d: %s", id, response.Code, response.Body.String())
		}
		var decoded map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	create("transfer_http01")
	create("transfer_http02")

	action := func(id, action string, transferred *int64) *httptest.ResponseRecorder {
		payload := map[string]any{"action": action}
		if transferred != nil {
			payload["transferredBytes"] = *transferred
		}
		body, _ := json.Marshal(payload)
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/sftp/transfers/"+id+"/actions", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(response, request)
		return response
	}
	if response := action("transfer_http01", "start", nil); response.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", response.Code, response.Body.String())
	}
	if response := action("transfer_http02", "start", nil); response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte("sftp_transfer_limit")) {
		t.Fatalf("limit = %d: %s", response.Code, response.Body.String())
	}
	settings := httptest.NewRecorder()
	settingsRequest := httptest.NewRequest(http.MethodPut, "/api/v1/sftp/transfers/settings", bytes.NewBufferString(
		`{"maxConcurrent":3,"clearCompletedAfterSeconds":300,"processingStopped":false,"largeFileThresholdBytes":52428800,"largeFileParallelism":6,"largeFileChunkBytes":536870912}`,
	))
	settingsRequest.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(settings, settingsRequest)
	if settings.Code != http.StatusOK {
		t.Fatalf("update settings = %d: %s", settings.Code, settings.Body.String())
	}
	missingFingerprint := httptest.NewRecorder()
	engine.ServeHTTP(missingFingerprint, httptest.NewRequest(http.MethodPost,
		"/api/v1/sftp/edge/uploads/transfer_http01", bytes.NewBufferString(`{"path":"/transfer_http01.bin","size":10}`)))
	if missingFingerprint.Code != http.StatusBadRequest {
		t.Fatalf("upload without source fingerprint = %d: %s", missingFingerprint.Code, missingFingerprint.Body.String())
	}
	progress := int64(4)
	if response := action("transfer_http01", "progress", &progress); response.Code != http.StatusConflict {
		t.Fatalf("client-authored upload progress = %d: %s", response.Code, response.Body.String())
	}

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/sftp/transfers", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", response.Code, response.Body.String())
	}
	var listed struct {
		MaxConcurrent           int   `json:"maxConcurrent"`
		LargeFileThresholdBytes int64 `json:"largeFileThresholdBytes"`
		LargeFileParallelism    int   `json:"largeFileParallelism"`
		LargeFileChunkBytes     int64 `json:"largeFileChunkBytes"`
		Jobs                    []struct {
			ID               string `json:"id"`
			BatchName        string `json:"batchName"`
			Status           string `json:"status"`
			TransferredBytes int64  `json:"transferredBytes"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.MaxConcurrent != 3 || listed.LargeFileThresholdBytes != 50<<20 || listed.LargeFileParallelism != 6 || listed.LargeFileChunkBytes != 512<<20 ||
		len(listed.Jobs) != 2 || listed.Jobs[0].BatchName != "HTTP batch" || listed.Jobs[0].Status != "running" || listed.Jobs[0].TransferredBytes != 0 {
		t.Fatalf("listed = %+v", listed)
	}
	if _, err := manager.UpdateJob("transfer_http01", sshcSFTP.UpdateTransferJob{Action: sshcSFTP.TransferCancelAction}); err != nil {
		t.Fatalf("cancel transfer before clear: %v", err)
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/sftp/transfers/finished", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("clear finished = %d: %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/sftp/transfers", nil))
	var remaining struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &remaining) != nil || len(remaining.Jobs) != 1 || remaining.Jobs[0].ID != "transfer_http02" {
		t.Fatalf("remaining after clear = %d: %s", response.Code, response.Body.String())
	}
}
