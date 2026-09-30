package application

import (
	"testing"

	"sshc/internal/secret"
	"sshc/internal/sshclient"
)

// officeProfile は、labProfile と同じ設定で名前だけが違うプロファイルである。
func officeProfile() VPNProfile {
	office := labProfile()
	office.Name = "office"
	return office
}

func passwordBindingOf(t *testing.T, service *Service, alias string) string {
	t.Helper()
	binding, err := service.PasswordBinding(alias)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func setConnectionVPN(t *testing.T, service *Service, alias, profile string) {
	t.Helper()
	if _, err := service.SetConnectionVPN(alias, profile); err != nil {
		t.Fatal(err)
	}
}

// 保存済みのパスワードと TOTP の割り当てが記録する値は、接続に付けた VPN プロファイルを
// 付けたとき、付け替えたとき、外したときに変わる。外せば、付ける前の値に戻る。
func TestThePasswordBindingChangesWithTheVPNProfileOfTheConnection(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{labProfile(), officeProfile()}
	service := serviceWithVPNMetadata(t, metadata)

	direct := passwordBindingOf(t, service, "lab")
	setConnectionVPN(t, service, "lab", "lab")
	throughLab := passwordBindingOf(t, service, "lab")
	setConnectionVPN(t, service, "lab", "office")
	throughOffice := passwordBindingOf(t, service, "lab")
	setConnectionVPN(t, service, "lab", "")
	detached := passwordBindingOf(t, service, "lab")

	if throughLab == direct || throughOffice == direct || throughOffice == throughLab {
		t.Fatalf("bindings do not tell the routes apart: direct %s, lab %s, office %s", direct, throughLab, throughOffice)
	}
	if detached != direct {
		t.Fatalf("binding after removing the VPN = %s, want the direct binding %s", detached, direct)
	}
}

// VPN を付けた接続で検出した OS は、その接続の一覧に残る。検出を記録するときと一覧に
// 出すときの両方で、接続（sshclient.Target）と同じく VPN を含めて照合する。
func TestTheDetectedOSOfAConnectionThroughAVPNIsKept(t *testing.T) {
	metadata := NewMetadata()
	metadata.VPNProfiles = []VPNProfile{labProfile()}
	metadata.Hosts = []HostMetadata{{Identity: HostIdentity{Path: "config", Alias: "lab"}, VPN: "lab"}}
	service := serviceWithVPNMetadata(t, metadata)
	target, err := sshclient.NewTarget("lab", service.ResolveConnection, LocalFactsFor(service.workspace.Home()))
	if err != nil {
		t.Fatal(err)
	}
	// 接続は、alias のブロックに付けた VPN を Target に入れて開く（internal/app の sshParts.target）。
	target.VPN = "lab"

	observed := service.ObserveConnectionOS(target)
	if observed == nil {
		t.Fatal("missing observer")
	}
	observed("ubuntu")

	overview, err := service.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Metadata.Hosts) != 1 || overview.Metadata.Hosts[0].DetectedOS != "ubuntu" {
		t.Fatalf("hosts = %+v, want ubuntu detected on lab", overview.Metadata.Hosts)
	}
}

// VPN プロファイルを付け替えて停止中になったパスワードと TOTP は、基本設定の保存で
// 経路を確認し直すと、付け替えた先のプロファイルを通る接続で使える。
func TestConfirmingTheRouteRebindsSavedCredentialsToTheNewVPNProfile(t *testing.T) {
	const config = "Host edge\n\tHostName 10.9.9.1\n\tUser deploy\n"
	harness := newConnectionUpdateHarness(t, config)
	for _, profile := range []VPNProfile{labProfile(), officeProfile()} {
		change, err := harness.service.PlanVPNProfileCreate(profile)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := harness.service.CommitVPNProfileChange(change, nil); err != nil {
			t.Fatal(err)
		}
	}
	setConnectionVPN(t, harness.service, "edge", "lab")
	setPasswordForCurrentTarget(t, harness, "edge", "edge-password")
	if err := harness.secrets.SetCredential(secret.KindTOTP, "edge-code", "JBSWY3DPEHPK3PXP"); err != nil {
		t.Fatal(err)
	}
	if err := harness.secrets.AssignBoundCredential(secret.BoundAssignment{
		Kind: secret.KindTOTP, Subject: "edge", Name: "edge-code", Binding: passwordBindingOf(t, harness.service, "edge"),
	}); err != nil {
		t.Fatal(err)
	}
	setConnectionVPN(t, harness.service, "edge", "office")
	if got := passwordForCurrentTarget(t, harness.service, harness.secrets, "edge"); got != "" {
		t.Fatalf("through office before confirming, the password saved for lab was released: %q", got)
	}

	if _, err := harness.service.UpdateConnection(harness.inventory, UpdateConnectionRequest{
		Identity: HostIdentity{Path: "config", Alias: "edge"}, Base: config,
		Password: UpdateConnectionPassword{Kind: UpdatePasswordRebind},
		TOTP:     UpdateConnectionTOTP{Kind: UpdateTOTPRebind},
	}); err != nil {
		t.Fatal(err)
	}

	throughOffice := passwordBindingOf(t, harness.service, "edge")
	if got := harness.secrets.BoundFor(secret.KindPassword, "edge", throughOffice); got != "edge-password" {
		t.Fatalf("through office after confirming, the password = %q", got)
	}
	if got := harness.secrets.BoundFor(secret.KindTOTP, "edge", throughOffice); got == "" {
		t.Fatal("through office after confirming, the TOTP was not released")
	}
}
