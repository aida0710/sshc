package httpserver

import (
	"fmt"
	"testing"

	"sshc/internal/keys"
	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/vpn"
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
		{fmt.Errorf("ssh: handshake failed: %w", sshclient.ErrAuthenticationRejected), "authentication_rejected"},
		{sshclient.ErrVPNWithProxyCommand, "route_misconfigured"},
		{sshclient.ErrVPNThroughJump, "route_misconfigured"},
		{sshclient.ErrProxyCommandThroughJump, "route_misconfigured"},
		{sshclient.ErrProxyAuthenticationRequired, "proxy_authentication_required"},
		{sshclient.ErrPromptAborted, "authentication_cancelled"},
		{keys.ErrPassphraseRequired, "key_passphrase_required"},
		{keys.ErrWrongPassphrase, "key_passphrase_required"},
		{&sshclient.ExplainedError{Sentence: "切断されています。", Err: vpn.ErrRouteDisconnected}, "vpn_route_disconnected"},
		{&sshclient.KnownHostsSymlinkError{Path: "/home/me/.ssh/known_hosts", Err: knownhosts.ErrSymlinkPath}, "known_hosts_symlink"},
	} {
		t.Run(test.code, func(t *testing.T) {
			code, named := connectProblem(fmt.Errorf("wrapped: %w", test.err))
			if !named || code != test.code {
				t.Fatalf("connectProblem = %q/%v, want %q/true", code, named, test.code)
			}
		})
	}
}

// 利用者が切断した VPN 経路で再接続を止めたときは、設定を直すよう促す既定の文では
// なく、切断されたことを書く。
func TestTheReconnectStopNoticeSaysTheVPNRouteWasDisconnected(t *testing.T) {
	if notice := reconnectStopNotice("vpn_route_disconnected"); notice != "VPN経路が切断されたため、自動再接続を停止しました。" {
		t.Fatalf("notice = %q", notice)
	}
	if notice := reconnectStopNotice("host_key_changed"); notice != "" {
		t.Fatalf("既定の文を使う理由に、専用の文を返した: %q", notice)
	}
}
