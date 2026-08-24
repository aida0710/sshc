package workspace_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"sshc/internal/workspace"
)

func TestServiceCreatesUpdatesRestoresAndDeletesAWorkspace(t *testing.T) {
	store := newStore(t)
	createdAt := time.Date(2026, 8, 24, 7, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	moments := []time.Time{createdAt, updatedAt}
	clock := func() time.Time {
		moment := moments[0]
		moments = moments[1:]
		return moment
	}
	service := workspace.NewService(store, clock, bytes.NewReader(bytes.Repeat([]byte{0x2a}, 16)))

	created, err := service.Create(workspace.Definition{
		Name: "Production", Layout: split(workspace.Horizontal,
			pane("web-pane", "web"), pane("db-pane", "database")),
		FocusedPaneID: "web-pane",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a" || created.CreatedAt != "2026-08-24T07:00:00Z" {
		t.Fatalf("created = %#v", created)
	}

	plan, err := service.Restore(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Panes) != 2 || plan.Panes[0].Alias != "web" || plan.Panes[1].Alias != "database" {
		t.Fatalf("restore panes = %#v", plan.Panes)
	}
	for _, reconnect := range plan.Panes {
		if reconnect.State != workspace.ReconnectRequired {
			t.Fatalf("restore state = %q", reconnect.State)
		}
	}

	updated, err := service.Update(created.ID, workspace.Definition{
		Name: "Production incident", Layout: pane("db-pane", "database"), FocusedPaneID: "db-pane",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CreatedAt != created.CreatedAt || updated.UpdatedAt != "2026-08-24T08:00:00Z" {
		t.Fatalf("updated = %#v", updated)
	}
	if err := service.Delete(created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(created.ID); !errors.Is(err, workspace.ErrNotFound) {
		t.Fatalf("Get after Delete = %v", err)
	}
}

func TestReturnedLayoutsDoNotMutateStoredData(t *testing.T) {
	store := newStore(t)
	service := workspace.NewService(
		store,
		func() time.Time { return time.Date(2026, 8, 24, 7, 0, 0, 0, time.UTC) },
		bytes.NewReader(bytes.Repeat([]byte{0x1b}, 16)),
	)
	created, err := service.Create(workspace.Definition{
		Name: "Production", Layout: pane("web-pane", "web"), FocusedPaneID: "web-pane",
	})
	if err != nil {
		t.Fatal(err)
	}
	created.Layout.Pane.Alias = "mutated"
	loaded, err := service.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Layout.Pane.Alias != "web" {
		t.Fatalf("stored alias = %q", loaded.Layout.Pane.Alias)
	}
}
