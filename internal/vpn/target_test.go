package vpn

import (
	"testing"
)

// connect が書いた理由の語を、そのまま繋げなかった理由にする。
func TestAConnectFailureWordBecomesTheReason(t *testing.T) {
	watch := newConnectWatch()
	_, _ = watch.Write([]byte("sshc-vpn-failure: target_unre"))
	_, _ = watch.Write([]byte("solved\n"))

	if reason := watch.failureReason(); reason != FailureTargetUnresolved {
		t.Fatalf("reason = %q", reason)
	}
}

// 知らない語は読まない。画面と CLI が訳せない語を返さない。
func TestAnUnknownConnectFailureWordIsNotRepeated(t *testing.T) {
	watch := newConnectWatch()
	_, _ = watch.Write([]byte("sshc-vpn-failure: something_new\n"))

	if reason := watch.failureReason(); reason != FailureTargetUnreachable {
		t.Fatalf("reason = %q", reason)
	}
}

// socat が繋ぎに行って諦めたなら、接続先へ届かなかった。
func TestSocatGivingUpMeansTheDestinationWasUnreachable(t *testing.T) {
	watch := newConnectWatch()
	_, _ = watch.Write([]byte("2026/09/24 10:00:00 socat[42] E connect(5, AF=2 10.77.0.1:2299, 16): Connection refused\n"))

	if reason := watch.failureReason(); reason != FailureTargetUnreachable {
		t.Fatalf("reason = %q", reason)
	}
	select {
	case <-watch.started:
		t.Fatal("繋げなかったのに中継が始まったことになった")
	default:
	}
}

// connect まで届かずに終わったなら、経路そのものが無い。
func TestSilenceMeansTheRouteIsGone(t *testing.T) {
	watch := newConnectWatch()
	_, _ = watch.Write([]byte("Error response from daemon: container is not running\n"))

	if reason := watch.failureReason(); reason != FailureTunnelLost {
		t.Fatalf("reason = %q", reason)
	}
}

// socat が中継を始めたら、繋がったと知らせる。
func TestSocatStartingItsLoopMeansConnected(t *testing.T) {
	watch := newConnectWatch()
	_, _ = watch.Write([]byte("2026/09/24 10:00:00 socat[42] N starting data transfer loop with FDs [0,1] and [5,5]\n"))

	select {
	case <-watch.started:
	default:
		t.Fatal("中継が始まったことを知らせなかった")
	}
	// 二度書かれても閉じ直さない。
	_, _ = watch.Write([]byte("2026/09/24 10:00:00 socat[42] N starting data transfer loop with FDs [0,1] and [5,5]\n"))
}
