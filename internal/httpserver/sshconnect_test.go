package httpserver

import (
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"sshc/internal/keys"
	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
	"sshc/internal/vpn"
)

func TestConnectProblemNamesFailuresThatNeedUserAction(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{sshclient.ErrHostKeyUnknown, "host_key_unknown"},
		{fmt.Errorf("ssh: handshake failed: %w", sshclient.ErrHostKeyDeclined), "host_key_declined"},
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
	if notice := reconnectStopNotice("vpn_route_disconnected"); notice != "VPNルートが切断されたため、自動再接続を停止しました。" {
		t.Fatalf("notice = %q", notice)
	}
	if notice := reconnectStopNotice("host_key_changed"); notice != "" {
		t.Fatalf("既定の文を使う理由に、専用の文を返した: %q", notice)
	}
}

// refusedDial は、閉じたポートへ実際に繋いだ失敗を返す。
func refusedDial(t *testing.T) error {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	conn, err := net.Dial("tcp", address)
	if err == nil {
		_ = conn.Close()
		t.Fatal("the closed port took the connection")
	}
	return err
}

func TestAConnectionFailureThatMayPassByItselfKeepsItsReasonAndIsTriedAgain(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{refusedDial(t), "connect_refused"},
		{&net.DNSError{Err: "server misbehaving", IsTemporary: true}, "connect_no_network"},
		{fmt.Errorf("ssh: handshake failed: %w", io.EOF), "connect_closed"},
		{errors.New("something else"), terminal.ProblemReconnectFailed},
	}
	for _, testCase := range cases {
		retry, problem := reconnectError(testCase.err)
		if !retry || problem != testCase.want {
			t.Errorf("reconnectError(%v) = %v, %q, want true, %q", testCase.err, retry, problem, testCase.want)
		}
	}
}

// 画面は始まりと終わりの差で「開いてすぐ終わった」を見る。秒に丸めると、同じ秒の中で終わったものがみな該当する。
func TestASessionsTimesKeepTheirMilliseconds(t *testing.T) {
	started := time.Date(2026, 11, 4, 1, 2, 3, 100_000_000, time.UTC)
	described := describeSession(terminal.View{
		Started: started,
		Exited:  &terminal.ExitInfo{At: started.Add(800 * time.Millisecond)},
	})
	if described.StartedAt != "2026-11-04T01:02:03.100Z" || described.Exited.At != "2026-11-04T01:02:03.900Z" {
		t.Errorf("startedAt = %q, exited at = %q", described.StartedAt, described.Exited.At)
	}
}
