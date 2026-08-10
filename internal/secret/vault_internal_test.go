package secret

import "testing"

func TestDedicatedKeyPassphraseCloneIsIsolated(t *testing.T) {
	vault, err := Create("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.SetDedicatedKeyPassphrase("keys/id_a", "original"); err != nil {
		t.Fatal(err)
	}

	cloned := vault.clone()
	if err := cloned.SetDedicatedKeyPassphrase("keys/id_a", "changed"); err != nil {
		t.Fatal(err)
	}
	if err := cloned.SetDedicatedKeyPassphrase("keys/id_b", "new"); err != nil {
		t.Fatal(err)
	}

	if got, _ := vault.SecretFor(KindKeyPassphrase, "keys/id_a"); got != "original" {
		t.Fatalf("live value changed through clone: %q", got)
	}
	if _, ok := vault.SecretFor(KindKeyPassphrase, "keys/id_b"); ok {
		t.Fatal("live vault gained a clone-only subject")
	}
}
