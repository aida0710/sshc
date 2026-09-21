package application

import (
	"strings"
	"testing"

	"sshc/internal/storage"
)

func TestMovingAHostAppliesItsNewGroupsSettings(t *testing.T) {
	service, workspace := newTestService(t)
	relative := writeGroupFile(t, workspace, "old", "audit.conf", "Host audit\n\tHostName server.example\n")
	metadata := NewMetadata()
	metadata.Groups = []GroupMetadata{
		{Name: "old", Settings: []Setting{{Keyword: "Port", Values: []string{"2222"}}}},
		{Name: "new", Settings: []Setting{{Keyword: "Port", Values: []string{"3333"}}}},
	}
	if _, err := service.Save(EditRequest{Kind: EditGroups, Metadata: &metadata}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(EditRequest{Kind: EditMove, Path: relative, Base: readFile(t, workspace, relative), Alias: "audit", DestinationGroup: "new"}); err != nil {
		t.Fatal(err)
	}
	detail, err := service.HostDetail("connections/new/audit.conf", "audit")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range detail.Effective.Entries {
		if entry.Keyword != "Port" {
			continue
		}
		if strings.Join(entry.Values, " ") != "3333" {
			t.Fatalf("new group still uses old group's Port: %v", entry.Values)
		}
		return
	}
	t.Fatal("moved host has no inherited Port")
}

func TestHostRenameRebuildsInheritedGroupSettings(t *testing.T) {
	service, workspace := newTestService(t)
	writeGroupFile(t, workspace, "work", "host.conf", "Host audit-old\n\tHostName server.example\n")
	metadata := NewMetadata()
	metadata.Groups = []GroupMetadata{{Name: "work", Settings: []Setting{{Keyword: "Port", Values: []string{"2222"}}}}}
	if _, err := service.Save(EditRequest{Kind: EditGroups, Metadata: &metadata}); err != nil {
		t.Fatal(err)
	}
	relative := GroupDirectory("work") + "/host.conf"
	if _, err := service.Save(EditRequest{Kind: EditRename, Path: relative, Base: readFile(t, workspace, relative), Alias: "audit-old", NewAlias: "audit-new"}); err != nil {
		t.Fatal(err)
	}
	compiled := readFile(t, workspace, DefaultGroupsFile)
	if !strings.Contains(compiled, "Host audit-new\n") || strings.Contains(compiled, "Host audit-old\n") {
		t.Fatalf("group settings retain old alias after rename:\n%s", compiled)
	}
}

func TestFileRenamePreservesHostMetadata(t *testing.T) {
	service, root := newFileOpsService(t)
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{Identity: HostIdentity{Path: "work/lon.conf", Alias: "lon"}, Tags: []string{"production"}}}
	change, err := service.metadata.Change(metadata, storage.Precondition{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.manager.Commit(storage.Request{Operation: "audit.fixture", Changes: []storage.Change{change}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(EditRequest{Kind: EditFileRename, Path: "work/lon.conf", Base: readWorkspace(t, root, "work/lon.conf"), DestinationPath: "work/london.conf"}); err != nil {
		t.Fatal(err)
	}
	detail, err := service.HostDetail("work/london.conf", "lon")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Metadata.Tags) != 1 {
		t.Fatal("file rename loses host metadata; original record stays under the old path")
	}
}

func TestCreatingAndDeletingAHostUpdatesItsGroupsHostList(t *testing.T) {
	harness := newConnectionCreateHarness(t)
	metadata := NewMetadata()
	metadata.Groups = []GroupMetadata{{Name: "home-lab/others", Settings: []Setting{{Keyword: "ServerAliveInterval", Values: []string{"45"}}}}}
	if _, err := harness.service.Save(EditRequest{Kind: EditGroups, Metadata: &metadata}); err != nil {
		t.Fatal(err)
	}
	created, err := harness.service.CreateConnection(harness.secrets, harness.inventory, keyCreateRequest(t, harness))
	if err != nil {
		t.Fatal(err)
	}
	compiled := readFile(t, harness.workspace, DefaultGroupsFile)
	if !strings.Contains(compiled, "Host lab-node\n") {
		t.Fatalf("new host is missing from its group: %s", compiled)
	}
	if _, err := harness.service.Save(EditRequest{
		Kind: EditFileDelete, Path: created.Identity.Path,
		Base: readFile(t, harness.workspace, created.Identity.Path),
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, harness.workspace, DefaultGroupsFile), "lab-node") {
		t.Fatal("deleted host remains in the generated group settings")
	}
}
