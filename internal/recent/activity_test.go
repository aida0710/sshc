package recent_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/recent"
	"sshc/internal/storage"
)

// newActivityStore は、now が呼ばれるたびに clock の時刻を返す記録を作る。
func newActivityStore(t *testing.T, clock *time.Time) *recent.ActivityStore {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return recent.NewActivityStore(workspace, func() time.Time { return *clock })
}

func record(t *testing.T, store *recent.ActivityStore, aliases ...string) {
	t.Helper()
	for _, alias := range aliases {
		if err := store.Record(alias); err != nil {
			t.Fatal(err)
		}
	}
}

func listActivity(t *testing.T, store *recent.ActivityStore) []recent.ActivityHour {
	t.Helper()
	hours, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	return hours
}

func TestConnectionsInTheSameHourAreCountedTogetherWithTheirHostsOnce(t *testing.T) {
	clock := time.Date(2026, 11, 8, 7, 5, 0, 0, time.UTC)
	store := newActivityStore(t, &clock)
	record(t, store, "web-01", "db", "web-01")
	clock = clock.Add(70 * time.Minute)
	record(t, store, "web-01")

	hours := listActivity(t, store)
	if len(hours) != 2 {
		t.Fatalf("hours = %#v, want two hours", hours)
	}
	first := hours[0]
	if !first.Hour.Equal(time.Date(2026, 11, 8, 7, 0, 0, 0, time.UTC)) || first.Connections != 3 || fmt.Sprint(first.Aliases) != "[web-01 db]" {
		t.Fatalf("first hour = %#v, want three connections to web-01 and db from 07:00", first)
	}
	if !hours[1].Hour.Equal(time.Date(2026, 11, 8, 8, 0, 0, 0, time.UTC)) || hours[1].Connections != 1 {
		t.Fatalf("second hour = %#v, want one connection from 08:00", hours[1])
	}
}

func TestAnHourInAnotherTimeZoneIsKeptInUTC(t *testing.T) {
	tokyo := time.FixedZone("JST", 9*60*60)
	clock := time.Date(2026, 11, 8, 1, 30, 0, 0, tokyo)
	store := newActivityStore(t, &clock)
	record(t, store, "web-01")

	if hour := listActivity(t, store)[0].Hour; !hour.Equal(time.Date(2026, 11, 7, 16, 0, 0, 0, time.UTC)) || hour.Location() != time.UTC {
		t.Fatalf("hour = %v, want 2026-11-07 16:00 UTC", hour)
	}
}

func TestHoursOlderThanTheRetentionAreDropped(t *testing.T) {
	clock := time.Date(2025, 11, 1, 12, 0, 0, 0, time.UTC)
	store := newActivityStore(t, &clock)
	record(t, store, "old-host")
	clock = clock.Add(recent.ActivityRetention + time.Hour)
	record(t, store, "web-01")

	hours := listActivity(t, store)
	if len(hours) != 1 || hours[0].Aliases[0] != "web-01" {
		t.Fatalf("hours = %#v, want only the hour within the retention", hours)
	}
}

func TestOnlyTheFirstHostsOfABusyHourAreKeptButEveryConnectionIsCounted(t *testing.T) {
	clock := time.Date(2026, 11, 8, 7, 0, 0, 0, time.UTC)
	store := newActivityStore(t, &clock)
	for index := range recent.MaxActivityAliasesPerHour + 3 {
		record(t, store, fmt.Sprintf("host-%02d", index))
	}

	hour := listActivity(t, store)[0]
	if hour.Connections != recent.MaxActivityAliasesPerHour+3 || len(hour.Aliases) != recent.MaxActivityAliasesPerHour {
		t.Fatalf("hour = %d connections, %d hosts", hour.Connections, len(hour.Aliases))
	}
}

func TestABrokenActivityFileIsRefusedRatherThanOverwritten(t *testing.T) {
	clock := time.Date(2026, 11, 8, 7, 0, 0, 0, time.UTC)
	store := newActivityStore(t, &clock)
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, contents := range []string{
		`{"schemaVersion":1,"hours":[{"hour":"2026-11-08T07:30:00Z","connections":1,"aliases":["web"]}]}`,
		`{"schemaVersion":1,"hours":[{"hour":"2026-11-08T08:00:00Z","connections":1,"aliases":["web"]},{"hour":"2026-11-08T07:00:00Z","connections":1,"aliases":["db"]}]}`,
		`{"schemaVersion":1,"hours":[{"hour":"2026-11-08T07:00:00Z","connections":0,"aliases":[]}]}`,
		`{"schemaVersion":1,"hours":[{"hour":"2026-11-08T07:00:00Z","connections":2,"aliases":["web","web"]}]}`,
	} {
		if err := os.WriteFile(store.Path(), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := store.Record("web"); !errors.Is(err, recent.ErrInvalidActivity) {
			t.Fatalf("Record over %s = %v, want ErrInvalidActivity", contents, err)
		}
		if written, _ := os.ReadFile(store.Path()); string(written) != contents {
			t.Fatalf("the broken file was overwritten with %s", written)
		}
	}
}

func TestAnActivityFileFromANewerSSHCIsNotRead(t *testing.T) {
	clock := time.Date(2026, 11, 8, 7, 0, 0, 0, time.UTC)
	store := newActivityStore(t, &clock)
	record(t, store, "web")
	if err := os.WriteFile(store.Path(), []byte(`{"schemaVersion":2,"hours":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := store.List(); !errors.Is(err, recent.ErrUnsupportedVersion) {
		t.Fatalf("List = %v, want ErrUnsupportedVersion", err)
	}
}
