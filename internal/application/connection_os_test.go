package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/sshclient"
)

func TestConnectionOSPersistsAndDoesNotOverwriteAnOverride(t *testing.T) {
	service, workspace := newTestService(t)
	target, err := sshclient.NewTarget("bastion", service.ResolveConnection, workspace.Home())
	if err != nil {
		t.Fatal(err)
	}
	observed := service.ObserveConnectionOS(target)
	if observed == nil {
		t.Fatal("missing observer")
	}
	observed("amazonlinux")
	overview, err := service.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Metadata.Hosts) != 1 || overview.Metadata.Hosts[0].DetectedOS != "amazonlinux" {
		t.Fatalf("metadata: %+v", overview.Metadata.Hosts)
	}
	metadata := overview.Metadata
	metadata.Hosts[0].OS = "debian"
	metadata.Hosts[0].Note = "keep this note"
	writeServiceMetadata(t, service, workspace, metadata)
	observed("redhat")
	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Hosts[0].OS != "debian" || stored.Hosts[0].Note != "keep this note" || stored.Hosts[0].DetectedOS != "amazonlinux" {
		t.Fatalf("override lost: %+v", stored.Hosts[0])
	}
	if service.ObserveConnectionOS(target) != nil {
		t.Fatal("manual override should skip detection")
	}
}

func TestConnectionOSRejectsLateResultsAfterRetargetingAndHidesOldIcon(t *testing.T) {
	service, workspace := newTestService(t)
	target, err := sshclient.NewTarget("bastion", service.ResolveConnection, workspace.Home())
	if err != nil {
		t.Fatal(err)
	}
	observed := service.ObserveConnectionOS(target)
	observed("ubuntu")
	path := filepath.Join(workspace.Root(), "config")
	if err := os.WriteFile(path, []byte(strings.Replace(serviceMainConfig, "203.0.113.10", "203.0.113.20", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	observed("redhat")
	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Hosts[0].DetectedOS != "ubuntu" {
		t.Fatal("late result was applied to a different machine")
	}
	overview, err := service.Overview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Metadata.Hosts[0].DetectedOS != "" {
		t.Fatal("Home shows stale icon")
	}
}
