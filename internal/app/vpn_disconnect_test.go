package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"sshc/internal/sshclient"
	"sshc/internal/terminal"
	"sshc/internal/vpn"
)

// 利用者が切断した経路は、その切断で切れた接続の自動再接続では起動し直さない。
func TestAnAutomaticReconnectDoesNotRestartADisconnectedRoute(t *testing.T) {
	err := refuseRestartAfterDisconnect(terminal.WithAutomaticReconnect(context.Background()), "lab", true)

	var explained *sshclient.ExplainedError
	if !errors.As(err, &explained) || !errors.Is(err, vpn.ErrRouteDisconnected) {
		t.Fatalf("err = %v, want an explained ErrRouteDisconnected", err)
	}
	if !strings.Contains(explained.Sentence, "VPN経路「lab」は切断されています。") {
		t.Fatalf("sentence = %q", explained.Sentence)
	}
}

// ［再接続］の操作と新しく開いた接続は、切断した経路を起動し直す。利用者がいま
// 接続を求めている。
func TestAReconnectAskedForByTheUserRestartsADisconnectedRoute(t *testing.T) {
	if err := refuseRestartAfterDisconnect(context.Background(), "lab", true); err != nil {
		t.Fatalf("err = %v", err)
	}
}

// 切断していない経路は、自動再接続でも起動する（VPNが切れた、などで止まった経路）。
func TestAnAutomaticReconnectStartsARouteThatWasNotDisconnected(t *testing.T) {
	if err := refuseRestartAfterDisconnect(terminal.WithAutomaticReconnect(context.Background()), "lab", false); err != nil {
		t.Fatalf("err = %v", err)
	}
}
