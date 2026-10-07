package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"sshc/internal/httpserver"
)

func TestSFTPRecoverySettingsDistinguishOmittedValuesFromExplicitZero(t *testing.T) {
	for _, args := range [][]string{
		{"sshc", "sftp", "settings"},
		{"sshc", "sftp", "settings", "--speed-limit", "0", "--reconnect-attempts", "0"},
		{"sshc", "sftp", "settings", "--speed-limit", "2048", "--reconnect-attempts", "3"},
	} {
		called, err := parseInvocation(args)
		if err != nil {
			t.Fatal(err)
		}
		if len(args) == 3 {
			if called.SFTP.SpeedLimitKiB != nil || called.SFTP.ReconnectAttempts != nil {
				t.Fatal("omission should preserve the stored defaults")
			}
			continue
		}
		if called.SFTP.SpeedLimitKiB == nil || called.SFTP.ReconnectAttempts == nil {
			t.Fatal("explicit settings were lost")
		}
	}
	for _, args := range [][]string{
		{"sshc", "sftp", "settings", "--speed-limit", "-1"},
		{"sshc", "sftp", "settings", "--reconnect-attempts", "11"},
		{"sshc", "sftp", "get", "host", "/source", "target", "--speed-limit", "100"},
	} {
		if _, err := parseInvocation(args); err == nil {
			t.Fatalf("unsafe or misplaced setting accepted: %v", args)
		}
	}
}

func TestSFTPRecoverySettingsPersistExplicitValuesAndPreserveOtherSettings(t *testing.T) {
	stored := httpserver.SFTPTransferJobList{MaxConcurrent: 3, ProcessingStopped: true, ClearCompletedAfterSeconds: 300,
		LargeFileThresholdBytes: 100 << 20, LargeFileParallelism: 4, LargeFileChunkBytes: 32 << 20, Jobs: []httpserver.SFTPTransferJob{}}
	updates := 0
	engine := &engineAPI{origin: "http://engine.invalid", client: &http.Client{Transport: engineAPIRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPut {
			if request.URL.Path != "/api/v1/sftp/transfers/settings" {
				t.Fatalf("unexpected mutation: %s", request.URL.Path)
			}
			if err := json.NewDecoder(request.Body).Decode(&stored); err != nil {
				t.Fatal(err)
			}
			updates++
		}
		contents, err := json.Marshal(stored)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(contents))), Request: request}, nil
	})}}
	for _, values := range []struct {
		speed    string
		attempts string
		wantRate int64
		wantAuto bool
	}{
		{speed: "2048", attempts: "3", wantRate: 2048 << 10, wantAuto: true},
		{speed: "0", attempts: "0", wantRate: 0, wantAuto: false},
	} {
		called, err := parseInvocation([]string{"sshc", "sftp", "settings", "--speed-limit", values.speed, "--reconnect-attempts", values.attempts})
		if err != nil {
			t.Fatal(err)
		}
		var stdout, stderr strings.Builder
		if code := runSFTPSettings(t.Context(), engine, *called.SFTP, &stdout, &stderr); code != 0 {
			t.Fatalf("settings refused: %d, %s", code, stderr.String())
		}
		if stored.SpeedLimitBytesPerSecond != values.wantRate || stored.AutoReconnect != values.wantAuto || !stored.ProcessingStopped ||
			stored.MaxConcurrent != 3 || stored.ClearCompletedAfterSeconds != 300 || stored.LargeFileParallelism != 4 || stored.LargeFileThresholdBytes != 100<<20 || stored.LargeFileChunkBytes != 32<<20 {
			t.Fatalf("persisted settings = %+v", stored)
		}
	}
	if updates != 2 || stored.MaxReconnectAttempts != 0 {
		t.Fatalf("updates = %d, attempts = %d", updates, stored.MaxReconnectAttempts)
	}
}
