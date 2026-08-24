package httpserver

import (
	"fmt"
	"testing"

	"sshc/internal/keys"
	"sshc/internal/sshclient"
)

func TestConnectProblemNamesFailuresThatNeedUserAction(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{sshclient.ErrHostKeyUnknown, "host_key_unknown"},
		{sshclient.ErrHostKeyChanged, "host_key_changed"},
		{sshclient.ErrHostKeyRevoked, "host_key_revoked"},
		{sshclient.ErrNoIdentity, "identity_unavailable"},
		{sshclient.ErrNoAuthMethod, "authentication_unavailable"},
		{sshclient.ErrPromptAborted, "authentication_cancelled"},
		{keys.ErrPassphraseRequired, "key_passphrase_required"},
		{keys.ErrWrongPassphrase, "key_passphrase_required"},
	} {
		t.Run(test.code, func(t *testing.T) {
			code, named := connectProblem(fmt.Errorf("wrapped: %w", test.err))
			if !named || code != test.code {
				t.Fatalf("connectProblem = %q/%v, want %q/true", code, named, test.code)
			}
		})
	}
}
