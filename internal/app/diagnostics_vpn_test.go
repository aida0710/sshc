package app

import (
	"context"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/application"
	"sshc/internal/diagnostics"
	"sshc/internal/vpn"
)

// recordingDialer は、ダイヤルしようとした宛先を控え、どこへも繋がない。
type recordingDialer struct{ dialled []string }

func (d *recordingDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.dialled = append(d.dialled, address)
	return nil, net.UnknownNetworkError("not dialled in test")
}

// engine の Diagnostics は、接続に付けた VPN プロファイルを sshc の設定から読み、
// VPN を付けた接続の到達確認でこのマシンからダイヤルしない。
func TestTheEngineReachabilityCheckDoesNotDialAConnectionWithAVPN(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config"), []byte("Host lab\n  HostName 10.9.9.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	services, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	// engine は metadata の巻き戻し用の控えを Vault の鍵で封じるので、紐付けを書くには
	// Vault が要る。
	if err := services.vault.Initialise("reachability test passphrase"); err != nil {
		t.Fatal(err)
	}
	change, err := services.config.PlanVPNProfileCreate(application.VPNProfile{
		Name: "office", Backend: vpn.WireGuard,
		WireGuard: &application.WireGuardProfile{Servers: []string{"vpn.example.jp"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := services.config.CommitVPNProfileChange(change, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := services.config.SetConnectionVPN("lab", "office"); err != nil {
		t.Fatal(err)
	}
	dialer := &recordingDialer{}
	services.diagnostics.Reachability = diagnostics.Reachability{Dialer: dialer}

	result, err := services.diagnostics.Reach(context.Background(), "lab")
	if err != nil {
		t.Fatalf("Reach = %v", err)
	}
	if result.Outcome != diagnostics.ReachabilityNotChecked || len(dialer.dialled) != 0 {
		t.Fatalf("outcome = %q, dialled %v, want not_checked without a dial", result.Outcome, dialer.dialled)
	}
}
