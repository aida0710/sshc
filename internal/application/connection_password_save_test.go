package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/secret"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// askOnConnectRequest は、認証の情報を保存しない新しい接続である。
func askOnConnectRequest() CreateConnectionRequest {
	return CreateConnectionRequest{
		Alias: "lab-node", HostName: "203.0.113.20", User: "deploy",
		Authentication: CreateAuthentication{Kind: CreateAuthenticationAskOnConnect},
	}
}

// connectedTarget は、接続が使うのと同じ Target である。
func connectedTarget(t *testing.T, harness connectionCreateHarness, alias string) sshclient.Target {
	t.Helper()
	graph, err := harness.service.resolve()
	if err != nil {
		t.Fatal(err)
	}
	target, err := harness.service.targetForGraph(graph, alias)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func typedFor(t *testing.T, harness connectionCreateHarness, alias, password string) terminal.TypedPassword {
	t.Helper()
	return terminal.TypedPassword{Alias: alias, Binding: connectedTarget(t, harness, alias).AuthenticationBinding(), Password: password}
}

func readEntry(t *testing.T, harness connectionCreateHarness) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(harness.workspace.Root(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func TestSavingATypedPasswordLetsTheNextConnectionUseIt(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	if _, err := harness.service.CreateConnection(harness.inventory, askOnConnectRequest()); err != nil {
		t.Fatal(err)
	}
	typed := typedFor(t, harness, "lab-node", "typed-at-the-prompt")
	if err := harness.service.SaveTypedPassword(typed); err != nil {
		t.Fatal(err)
	}
	if password := harness.secrets.BoundFor(secret.KindPassword, "lab-node", typed.Binding); password != "typed-at-the-prompt" {
		t.Fatalf("the vault gives %q", password)
	}
	// 同じ答えが2回届いても（ボタンを2度押した）、失敗にしない。
	if err := harness.service.SaveTypedPassword(typed); err != nil {
		t.Fatalf("saving the same password again = %v", err)
	}
}

func TestATypedPasswordIsNotSavedForADestinationThatChangedSinceItWasTyped(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	if _, err := harness.service.CreateConnection(harness.inventory, askOnConnectRequest()); err != nil {
		t.Fatal(err)
	}
	typed := typedFor(t, harness, "lab-node", "typed-at-the-prompt")
	entry := filepath.Join(harness.workspace.Root(), "config")
	changed := strings.ReplaceAll(readEntry(t, harness), "203.0.113.20", "203.0.113.99")
	if err := os.WriteFile(entry, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := harness.service.SaveTypedPassword(typed); !errors.Is(err, ErrConnectionChanged) {
		t.Fatalf("err = %v", err)
	}
	for _, binding := range []string{typed.Binding, connectedTarget(t, harness, "lab-node").AuthenticationBinding()} {
		if harness.secrets.BoundFor(secret.KindPassword, "lab-node", binding) != "" {
			t.Fatal("stored the password anyway")
		}
	}
}

func TestDecliningIsAskedOnceForTheSameDestination(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	if _, err := harness.service.CreateConnection(harness.inventory, askOnConnectRequest()); err != nil {
		t.Fatal(err)
	}
	target := connectedTarget(t, harness, "lab-node")
	if !harness.service.OffersPasswordSave(target) {
		t.Fatal("did not ask the first time")
	}
	if err := harness.service.DeclineTypedPassword(typedFor(t, harness, "lab-node", "typed")); err != nil {
		t.Fatal(err)
	}
	if harness.service.OffersPasswordSave(target) {
		t.Fatal("asked again after the answer was no")
	}
	if harness.secrets.BoundFor(secret.KindPassword, "lab-node", target.AuthenticationBinding()) != "" {
		t.Fatal("declining stored the password")
	}

	entry := filepath.Join(harness.workspace.Root(), "config")
	changed := strings.ReplaceAll(readEntry(t, harness), "203.0.113.20", "203.0.113.99")
	if err := os.WriteFile(entry, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	if !harness.service.OffersPasswordSave(connectedTarget(t, harness, "lab-node")) {
		t.Fatal("did not ask about a new destination")
	}
}

func TestAnEditOnTheConnectionsScreenKeepsTheAnswerNo(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	if _, err := harness.service.CreateConnection(harness.inventory, askOnConnectRequest()); err != nil {
		t.Fatal(err)
	}
	if err := harness.service.DeclineTypedPassword(typedFor(t, harness, "lab-node", "typed")); err != nil {
		t.Fatal(err)
	}
	identity := HostIdentity{Path: entryFileName, Alias: "lab-node"}
	stored, _, err := harness.service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	index := hostMetadataIndex(stored.Hosts, identity)
	if index < 0 {
		t.Fatal("no entry for the connection")
	}
	base := stored.Hosts[index]
	edited, err := applyHostMetadataEdit(stored, EditRequest{
		Path: identity.Path, Alias: identity.Alias, HostMetadataBase: &base,
		HostMetadata: &HostMetadata{Identity: identity, Colour: "#112233"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := edited.Hosts[hostMetadataIndex(edited.Hosts, identity)]; got.PasswordSaveDeclinedBinding == "" || got.Colour != "#112233" {
		t.Fatalf("host = %+v", got)
	}
}

func TestOnlyAPasswordThatWouldBeUsedIsAskedAbout(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	if _, err := harness.service.CreateConnection(harness.inventory, askOnConnectRequest()); err != nil {
		t.Fatal(err)
	}
	if harness.service.OffersPasswordSave(sshclient.Target{Alias: "nobody", HostName: "203.0.113.5"}) {
		t.Fatal("asked about a host that is not saved")
	}
	direct := connectedTarget(t, harness, "lab-node")
	direct.Direct = true
	if harness.service.OffersPasswordSave(direct) {
		t.Fatal("asked about a destination that was not saved")
	}
	harness.secrets.Lock()
	if harness.service.OffersPasswordSave(connectedTarget(t, harness, "lab-node")) {
		t.Fatal("asked while the vault is locked")
	}
}

func TestAConnectionThatNamesAKeyIsNotAskedAboutAPassword(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	request := keyCreateRequest(t, harness)
	request.Group = ""
	if _, err := harness.service.CreateConnection(harness.inventory, request); err != nil {
		t.Fatal(err)
	}
	if harness.service.OffersPasswordSave(connectedTarget(t, harness, "lab-node")) {
		t.Fatal("asked about a password that would never be used")
	}
}
