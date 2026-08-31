package remotesync

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"testing"
)

func TestFailureDetailKeepsAStableSafeCause(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "timeout", err: context.DeadlineExceeded, want: "bucket_timeout"},
		{name: "dns", err: &net.DNSError{Err: "not found", Name: "invalid.example"}, want: "bucket_dns_failed"},
		{name: "tls", err: x509.UnknownAuthorityError{}, want: "bucket_tls_failed"},
		{name: "network", err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}, want: "bucket_unreachable"},
		{name: "authentication", err: ErrAuthenticationFailed, want: "bucket_authentication_failed"},
		{name: "access", err: ErrAccessDenied, want: "bucket_access_denied"},
		{name: "rate limit", err: ErrRateLimited, want: "bucket_rate_limited"},
		{name: "service", err: ErrServiceUnavailable, want: "bucket_unavailable"},
		{name: "short download", err: io.ErrUnexpectedEOF, want: "snapshot_download_incomplete"},
		{name: "cost", err: ErrCostRefused, want: "snapshot_cost_refused"},
		{name: "remote size", err: ErrObjectTooLarge, want: "snapshot_too_large"},
		{name: "snapshot size", err: ErrSnapshotTooLarge, want: "snapshot_too_large"},
		{name: "unknown internal", err: errors.New("private implementation detail"), want: "sync_internal_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := failureDetail(test.err); got != test.want {
				t.Fatalf("failureDetail() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCredentialFailuresWaitForConfigurationChange(t *testing.T) {
	for _, err := range []error{ErrAuthenticationFailed, ErrAccessDenied} {
		if !deterministicSyncFailure(err) {
			t.Errorf("deterministicSyncFailure(%v) = false, want true", err)
		}
	}
	for _, err := range []error{ErrRateLimited, ErrServiceUnavailable} {
		if deterministicSyncFailure(err) {
			t.Errorf("deterministicSyncFailure(%v) = true, want false", err)
		}
	}
}
