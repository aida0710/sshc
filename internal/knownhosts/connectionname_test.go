package knownhosts_test

import (
	"reflect"
	"testing"

	"sshc/internal/knownhosts"
)

func TestAliasesAmongNamesTheConnectionsOfAHashedEntry(t *testing.T) {
	file := knownhosts.ParseFile([]byte(fixtureHashedHost + " " + fixtureKeyType + " " + fixtureKey + "\n" +
		"[db.example.com]:2222 " + fixtureKeyType + " " + fixtureKey + "\n"))
	names := []knownhosts.ConnectionName{
		{Alias: "bastion", Field: "bastion.example.com"},
		{Alias: "db", Field: "[db.example.com]:2222"},
		{Alias: "db-default-port", Field: "db.example.com"},
		{Alias: "jump", Field: "bastion.example.com"},
	}

	if got := file.Lines[0].Entry.AliasesAmong(names); !reflect.DeepEqual(got, []string{"bastion", "jump"}) {
		t.Errorf("hashed entry = %q, want bastion and jump", got)
	}
	if got := file.Lines[1].Entry.AliasesAmong(names); !reflect.DeepEqual(got, []string{"db"}) {
		t.Errorf("entry with a port = %q, want only the connection on that port", got)
	}
	if got := file.Lines[1].Entry.AliasesAmong(nil); got != nil {
		t.Errorf("no connections = %q", got)
	}
}
