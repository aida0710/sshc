package application

import (
	"testing"

	"sshc/internal/secret"
	"sshc/internal/secret/secrettest"
)

const testAuthenticationBinding = "abababababababababababababababababababababababababababababababab"

func setTestBoundPassword(harness connectionUpdateHarness, alias, password string) error {
	return secrettest.StoreDedicatedPassword(harness.secrets, harness.manager, secrettest.DedicatedPassword{
		Alias: alias, Password: password, Binding: testAuthenticationBinding,
	})
}

func testBoundPasswordFor(service *secret.Service, alias string) string {
	return service.BoundFor(secret.KindPassword, alias, testAuthenticationBinding)
}

func assignTestBoundPassword(service *secret.Service, alias, name string) error {
	return service.AssignBoundCredential(secret.BoundAssignment{Kind: secret.KindPassword, Subject: alias, Name: name, Binding: testAuthenticationBinding})
}

func setPasswordForCurrentTarget(t *testing.T, harness connectionUpdateHarness, alias, password string) {
	t.Helper()
	binding, err := harness.service.PasswordBinding(alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := secrettest.StoreDedicatedPassword(harness.secrets, harness.manager, secrettest.DedicatedPassword{
		Alias: alias, Password: password, Binding: binding,
	}); err != nil {
		t.Fatal(err)
	}
}

func passwordForCurrentTarget(t *testing.T, service *Service, secrets *secret.Service, alias string) string {
	t.Helper()
	binding, err := service.PasswordBinding(alias)
	if err != nil {
		return ""
	}
	return secrets.BoundFor(secret.KindPassword, alias, binding)
}
