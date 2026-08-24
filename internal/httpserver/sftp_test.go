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

func TestTransferManagerHTTPContractAndSharedLimit(t *testing.T) {
	manager := sshcSFTP.NewTransferManager(nil)
	manager.ConfigureJobs(1, nil)
	engine := echo.New()
	registerSFTPRoutes(engine, SFTPHandlers{Transfers: manager})

	create := func(id string) map[string]any {
		body, err := json.Marshal(map[string]any{
			"id": id, "batchId": "batch_http001", "alias": "edge", "direction": "upload",
			"kind": "file", "name": id + ".bin", "remotePath": "/" + id + ".bin", "totalBytes": 10,
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
	progress := int64(4)
	if response := action("transfer_http01", "progress", &progress); response.Code != http.StatusOK {
		t.Fatalf("progress = %d: %s", response.Code, response.Body.String())
	}

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/sftp/transfers", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", response.Code, response.Body.String())
	}
	var listed struct {
		MaxConcurrent int `json:"maxConcurrent"`
		Jobs          []struct {
			ID               string `json:"id"`
			Status           string `json:"status"`
			TransferredBytes int64  `json:"transferredBytes"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.MaxConcurrent != 1 || len(listed.Jobs) != 2 || listed.Jobs[0].Status != "running" || listed.Jobs[0].TransferredBytes != 4 {
		t.Fatalf("listed = %+v", listed)
	}
}
