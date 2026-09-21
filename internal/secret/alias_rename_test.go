package secret_test

import (
	"strings"
	"testing"

	"sshc/internal/secret"
)

func TestAliasRenameDoesNotReuseAnUnrelatedDedicatedPassword(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetBound("edge", "old-destination-password", strings.Repeat("cd", 32)); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "shared", "source-password"); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignBoundCredential(secret.BoundAssignment{Kind: secret.KindPassword, Subject: "bastion", Name: "shared", Binding: testAuthenticationBinding}); err != nil {
		t.Fatal(err)
	}
	if err := service.Rename("bastion", "edge"); err != nil {
		t.Fatal(err)
	}
	if got := service.BoundFor(secret.KindPassword, "edge", testAuthenticationBinding); got != "source-password" {
		t.Error("old destination password is now released for the source host's authentication binding")
	}
	service.Lock()
	if err := service.Unlock(passphrase); err != nil {
		t.Errorf("renamed vault cannot reopen: %v", err)
	}
	if got := service.BoundFor(secret.KindPassword, "edge", testAuthenticationBinding); got == "old-destination-password" {
		t.Error("the unrelated destination password remains active after locking and reopening the vault")
	}
}
