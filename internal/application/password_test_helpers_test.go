package application

import (
	"testing"

	"sshc/internal/secret"
)

const testAuthenticationBinding = "abababababababababababababababababababababababababababababababab"

func setTestBoundPassword(service *secret.Service, alias, password string) error {
	return service.SetBound(alias, password, testAuthenticationBinding)
}

func testBoundPasswordFor(service *secret.Service, alias string) string {
	return service.BoundPasswordFor(alias, testAuthenticationBinding)
}

func assignTestBoundPassword(service *secret.Service, alias, name string) error {
	return service.AssignPasswordCredential(alias, name, testAuthenticationBinding)
}

func setPasswordForCurrentTarget(t *testing.T, service *Service, secrets *secret.Service, alias, password string) {
	t.Helper()
	binding, err := service.PasswordBinding(alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := secrets.SetBound(alias, password, binding); err != nil {
		t.Fatal(err)
	}
}

func passwordForCurrentTarget(t *testing.T, service *Service, secrets *secret.Service, alias string) string {
	t.Helper()
	binding, err := service.PasswordBinding(alias)
	if err != nil {
		return ""
	}
	return secrets.BoundPasswordFor(alias, binding)
}
