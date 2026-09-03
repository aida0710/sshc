package app

import (
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/textencoding"
)

// Resolve が別の設定解決器を持つと、表示した接続先と実際の接続先が分岐する。
// Include、Match、ProxyJump、IdentityFile を一緒に通し、接続用 target の形で確かめる。
func TestCLIConnectionResolveUsesTheConnectionTarget(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(filepath.Join(root, "conf.d"), 0o700); err != nil {
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
  ProxyJump bastion

Match host edge user operator
  Port 2200
`
	if err := os.WriteFile(filepath.Join(root, "conf.d", "targets.conf"), []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}

	connection, err := NewCLIConnection(home, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := connection.Resolve("edge")
	if err != nil {
		t.Fatal(err)
	}
	if target.Alias != "edge" || target.HostName != "edge.internal" || target.User != "operator" || target.Port != "2200" {
		t.Fatalf("target = %+v", target)
	}
	wantIdentity := filepath.Join(root, "id_edge")
	if len(target.Identities) != 1 || target.Identities[0] != wantIdentity {
		t.Fatalf("identities = %q, want %q", target.Identities, wantIdentity)
	}
	if len(target.Jump) != 1 || target.Jump[0].Alias != "bastion" ||
		target.Jump[0].HostName != "bastion.internal" || target.Jump[0].User != "jump-user" ||
		target.Jump[0].Port != "22" {
		t.Fatalf("jump = %+v", target.Jump)
	}
	if target.Encoding != textencoding.UTF8 {
		t.Fatalf("encoding = %q, want UTF-8", target.Encoding)
	}
}
