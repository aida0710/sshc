package application

import (
	"errors"
	"strings"
	"testing"

	"sshc/internal/secret"
)

func TestAskingOnConnectWritesTheHostBlockAndNothingToTheVault(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	if _, err := harness.service.CreateConnection(harness.inventory, askOnConnectRequest()); err != nil {
		t.Fatal(err)
	}
	entry := readEntry(t, harness)
	if !strings.Contains(entry, "Host lab-node\n\tHostName 203.0.113.20\n\tUser deploy\n\tPort 22\n") {
		t.Fatalf("entry = %q", entry)
	}
	if strings.Contains(entry, "IdentityFile") {
		t.Fatalf("wrote a key into %q", entry)
	}
	binding := connectedTarget(t, harness, "lab-node").AuthenticationBinding()
	if password := harness.secrets.BoundFor(secret.KindPassword, "lab-node", binding); password != "" {
		t.Fatal("stored a password for a connection that asks on connecting")
	}
}

func TestANewConnectionCanNameTheConnectionItGoesThrough(t *testing.T) {
	for _, jump := range []string{"existing", "none"} {
		t.Run(jump, func(t *testing.T) {
			harness := newConnectionCreateHarness(t)
			request := askOnConnectRequest()
			request.ProxyJump = jump
			if _, err := harness.service.CreateConnection(harness.inventory, request); err != nil {
				t.Fatal(err)
			}
			if entry := readEntry(t, harness); !strings.Contains(entry, "\tPort 22\n\tProxyJump "+jump+"\n") {
				t.Fatalf("entry = %q", entry)
			}
		})
	}
}

func TestANewConnectionRefusesAJumpThatIsNotADeclaredConnection(t *testing.T) {
	cases := map[string]string{
		"not declared":    "nobody",
		"itself":          "lab-node",
		"a whole chain":   "existing,other",
		"a destination":   "root@existing",
		"an option":       "-oProxyCommand=x",
		"a line break":    "existing\nProxyCommand evil",
		"none in capital": "NONE",
	}
	for name, jump := range cases {
		t.Run(name, func(t *testing.T) {
			harness := newConnectionCreateHarness(t)
			before := readEntry(t, harness)
			request := askOnConnectRequest()
			request.ProxyJump = jump
			if _, err := harness.service.CreateConnection(harness.inventory, request); !errors.Is(err, ErrInvalidProxyJump) {
				t.Fatalf("err = %v", err)
			}
			if readEntry(t, harness) != before {
				t.Fatal("a refused jump wrote the config")
			}
		})
	}
}

func TestAConnectionInAGroupInheritsTheGroupsJumpUnlessItSaysNone(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	stored, precondition, err := harness.service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	stored.Groups = append(stored.Groups, GroupMetadata{
		Name: "home-lab/others", Settings: []Setting{{Keyword: "ProxyJump", Values: []string{"existing"}}},
	})
	if _, err := harness.service.commitMetadata(stored, precondition, "metadata.test"); err != nil {
		t.Fatal(err)
	}
	inherits := askOnConnectRequest()
	inherits.Group = "home-lab/others"
	if _, err := harness.service.CreateConnection(harness.inventory, inherits); err != nil {
		t.Fatal(err)
	}
	if jump := connectedTarget(t, harness, "lab-node").Jump; len(jump) != 1 || jump[0].Alias != "existing" {
		t.Fatalf("jump = %+v, want the group's", jump)
	}

	direct := askOnConnectRequest()
	direct.Alias, direct.Group, direct.ProxyJump = "lab-direct", "home-lab/others", "none"
	if _, err := harness.service.CreateConnection(harness.inventory, direct); err != nil {
		t.Fatal(err)
	}
	if jump := connectedTarget(t, harness, "lab-direct").Jump; len(jump) != 0 {
		t.Fatalf("jump = %+v, want none", jump)
	}
}
