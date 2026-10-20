package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"sshc/internal/api"
	"sshc/internal/connectionattempt"
	sshcSFTP "sshc/internal/sftp"
)

const attemptTestID = "6f1c2b9e-0d4a-4c1e-9a77-3b2f5d8e1a40"

// sftpAttemptRequests は、接続がいつも failure で失敗する SFTP の口へ要求を送る関数を返す。
// opened には接続を試みた回数を数える。
func sftpAttemptRequests(t *testing.T, failure error, opened *int) func(method, target string) (int, []byte) {
	t.Helper()
	handlers := SFTPHandlers{
		Service: &sshcSFTP.Service{Open: func(context.Context, string) (sshcSFTP.Remote, error) {
			*opened++
			return nil, failure
		}},
		Attempts: connectionattempt.NewRegistry(),
	}
	engine, credentials := sftpHTTPEngine(t, handlers)
	return func(method, target string) (int, []byte) {
		response := sendKeyRequest(t, engine, credentials, method, target, nil, "")
		return response.Code, response.Body.Bytes()
	}
}

func TestAListingThatNamesAnAttemptLeavesTheReasonItFailedToBeRead(t *testing.T) {
	opened := 0
	send := sftpAttemptRequests(t, errors.New("dial tcp 192.0.2.10:22: connect: connection refused"), &opened)

	if status, body := send(http.MethodGet, "/api/v1/sftp/edge/entries?path=/work&attempt="+attemptTestID); status == http.StatusOK {
		t.Fatalf("listing = %d %s, want a failure", status, body)
	}
	status, body := send(http.MethodGet, "/api/v1/sftp/edge/connection-attempts/"+attemptTestID)
	if status != http.StatusOK {
		t.Fatalf("attempt = %d %s", status, body)
	}
	var attempt api.SFTPConnectionAttempt
	if err := json.Unmarshal(body, &attempt); err != nil {
		t.Fatal(err)
	}
	if !attempt.Finished || len(attempt.Log) != 1 || attempt.Log[0] != "sshc: dial tcp 192.0.2.10:22: connect: connection refused" {
		t.Errorf("attempt = %+v", attempt)
	}
	// Another host's tab cannot read it.
	if status, _ := send(http.MethodGet, "/api/v1/sftp/db/connection-attempts/"+attemptTestID); status != http.StatusNotFound {
		t.Errorf("another alias read the attempt: %d", status)
	}
}

func TestAnAttemptNotBegunYetIsNotFound(t *testing.T) {
	opened := 0
	send := sftpAttemptRequests(t, errors.New("unused"), &opened)
	status, body := send(http.MethodGet, "/api/v1/sftp/edge/connection-attempts/"+attemptTestID)
	var problem api.Problem
	_ = json.Unmarshal(body, &problem)
	if status != http.StatusNotFound || problem.Code != "sftp_connection_attempt_not_found" {
		t.Errorf("unknown attempt = %d %s", status, body)
	}
}

func TestAListingWithAMalformedAttemptIsRefusedBeforeConnecting(t *testing.T) {
	opened := 0
	send := sftpAttemptRequests(t, errors.New("unused"), &opened)
	status, body := send(http.MethodGet, "/api/v1/sftp/edge/entries?attempt=short")
	var problem api.Problem
	_ = json.Unmarshal(body, &problem)
	if status != http.StatusBadRequest || problem.Code != "invalid_request" || opened != 0 {
		t.Errorf("malformed attempt = %d %s, opened %d times", status, body, opened)
	}
}
