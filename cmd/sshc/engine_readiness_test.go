package main

import (
	"strings"
	"testing"

	"sshc/internal/app"
)

func TestEngineReadinessAsksForUnlockOnlyWhileTheVaultIsLocked(t *testing.T) {
	cases := []struct {
		name      string
		readiness app.Readiness
		want      string
	}{
		{name: "no vault", readiness: app.Readiness{}, want: "sshc: create the password vault with `sshc vault create`"},
		{name: "locked vault", readiness: app.Readiness{VaultExists: true}, want: "sshc: unlock the password vault with `sshc vault unlock`"},
		{name: "passwordless vault", readiness: app.Readiness{VaultExists: true, VaultUnlocked: true}, want: "sshc: engine ready; vault is unlocked"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var stdout strings.Builder
			if err := announceReadiness(&stdout)(testCase.readiness); err != nil {
				t.Fatal(err)
			}
			if got := stdout.String(); got != testCase.want+"\n" {
				t.Fatalf("announcement = %q, want %q", got, testCase.want)
			}
		})
	}
}
