package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeInfoFixture(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(root, "conf.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sshc"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config"), []byte("Include conf.d/*.conf\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configuration := `Host bastion
  HostName bastion.internal
  User jump-user

Host edge
  HostName edge.internal
  User operator
  IdentityFile ~/.ssh/id_edge
  IdentitiesOnly yes
  ProxyJump bastion
  PreferredAuthentications publickey,password
  RequestTTY force
  StrictHostKeyChecking yes
  ConnectTimeout 9
  ServerAliveInterval 15
  ServerAliveCountMax 4
  ForwardAgent yes

Match host edge user operator
  Port 2200

Host secret-route
  HostName secret.internal
  ProxyCommand sh -c 'PROXY_SECRET_SENTINEL'
  SetEnv API_TOKEN=ENV_SECRET_SENTINEL
`
	if err := os.WriteFile(filepath.Join(root, "conf.d", "targets.conf"), []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := `{
  "schemaVersion": 3,
  "groupsFile": "groups.sshc.conf",
  "hosts": [{
    "identity": {"path": "conf.d/targets.conf", "alias": "edge"},
    "encoding": "shift_jis"
  }]
}
`
	if err := os.WriteFile(filepath.Join(root, "sshc", "metadata.json"), []byte(metadata), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestInfoJSONUsesTheConnectionTargetWithoutAnEngine(t *testing.T) {
	home := writeInfoFixture(t)
	var stdout, stderr strings.Builder
	if code := runInfo("edge", home, true, &stdout, &stderr); code != 0 {
		t.Fatalf("runInfo = %d, stderr = %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	var got infoDocument
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil {
		t.Fatalf("JSON = %v\n%s", err, stdout.String())
	}
	if got.SchemaVersion != 1 || got.Alias != "edge" {
		t.Fatalf("identity = %+v", got)
	}
	if got.Destination != (infoDestination{HostName: "edge.internal", User: "operator", Port: "2200"}) {
		t.Fatalf("destination = %+v", got.Destination)
	}
	wantIdentity := filepath.Join(home, ".ssh", "id_edge")
	if len(got.IdentityFiles) != 1 || got.IdentityFiles[0] != wantIdentity || !got.IdentitiesOnly {
		t.Fatalf("identities = %q, only = %v", got.IdentityFiles, got.IdentitiesOnly)
	}
	if len(got.ProxyJump) != 1 || got.ProxyJump[0].Alias != "bastion" ||
		got.ProxyJump[0].Destination != (infoDestination{HostName: "bastion.internal", User: "jump-user", Port: "22"}) {
		t.Fatalf("proxy jump = %+v", got.ProxyJump)
	}
	if got.Encoding != "shift_jis" || strings.Join(got.AuthenticationMethods, ",") != "publickey,password" {
		t.Fatalf("encoding/auth = %q/%q", got.Encoding, got.AuthenticationMethods)
	}
	if got.RequestTTY != "force" || got.StrictHostKeyChecking != "yes" ||
		got.ConnectTimeoutSeconds != 9 || got.ServerAliveIntervalSeconds != 15 ||
		got.ServerAliveCountMax != 4 || !got.AgentForward {
		t.Fatalf("connection options = %+v", got)
	}
	if got.IdentityFiles == nil || got.ProxyJump == nil || got.AuthenticationMethods == nil || got.Notices == nil {
		t.Fatalf("JSON arrays must not be null: %+v", got)
	}
}

func TestInfoNeverPrintsSetEnvOrProxyCommandContents(t *testing.T) {
	home := writeInfoFixture(t)
	for _, asJSON := range []bool{false, true} {
		var stdout, stderr strings.Builder
		if code := runInfo("secret-route", home, asJSON, &stdout, &stderr); code != 0 {
			t.Fatalf("json=%v: runInfo = %d, stderr = %s", asJSON, code, stderr.String())
		}
		printed := stdout.String() + stderr.String()
		for _, secret := range []string{"PROXY_SECRET_SENTINEL", "ENV_SECRET_SENTINEL", "API_TOKEN"} {
			if strings.Contains(printed, secret) {
				t.Fatalf("json=%v: output leaked %q:\n%s", asJSON, secret, printed)
			}
		}
		if !strings.Contains(printed, "proxy") {
			t.Fatalf("json=%v: configured proxy was omitted entirely:\n%s", asJSON, printed)
		}
	}
}

func TestInfoHumanOutputIncludesResolvedDefaults(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte("Host plain\n  HostName plain.internal\n  User operator\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	if code := runInfo("plain", home, false, &stdout, &stderr); code != 0 {
		t.Fatalf("runInfo = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"alias", "plain", "host", "plain.internal", "user", "operator", "port", "22", "encoding", "utf-8"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("human info lacks %q:\n%s", want, stdout.String())
		}
	}
}

func TestInfoRefusesInvalidAndUnresolvableAliasesWithoutPartialOutput(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	configuration := "Host refused\n  HostName refused.internal\nMatch exec true\n  Port 2200\n"
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		alias string
		code  int
	}{
		{alias: "-option", code: 2},
		{alias: "refused", code: 1},
	} {
		var stdout, stderr strings.Builder
		if code := runInfo(test.alias, home, false, &stdout, &stderr); code != test.code {
			t.Errorf("runInfo(%q) = %d, want %d", test.alias, code, test.code)
		}
		if stdout.Len() != 0 {
			t.Errorf("runInfo(%q) left partial stdout %q", test.alias, stdout.String())
		}
		if stderr.Len() == 0 {
			t.Errorf("runInfo(%q) gave no error", test.alias)
		}
	}
}
