package recent_test

import (
	"errors"
	"testing"
	"time"

	"sshc/internal/recent"
)

func TestServiceResolvesCurrentTargetsAndOmitsMissingAliases(t *testing.T) {
	store, _ := newStore(t,
		time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 24, 2, 0, 0, 0, time.UTC),
	)
	if err := store.Record("removed"); err != nil {
		t.Fatal(err)
	}
	if err := store.Record("bastion"); err != nil {
		t.Fatal(err)
	}
	service := recent.NewService(store, func(alias string) (recent.Target, error) {
		if alias == "removed" {
			return recent.Target{}, errors.New("not found")
		}
		return recent.Target{Alias: alias, HostName: "current.example", User: "deploy", Port: "2202"}, nil
	})
	connections, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(connections) != 1 || connections[0].HostName != "current.example" || connections[0].Alias != "bastion" {
		t.Fatalf("connections = %#v", connections)
	}
}
