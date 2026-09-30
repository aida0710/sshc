package application

import (
	"errors"
	"testing"

	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/storage"
)

// seedMetadata は metadata.json を直接書く。画面の保存（kind "metadata" と "groups"）は
// 1 つの部分しか変えないので、複数の部分を前提にする試験や、ほかの書き手（CLI、
// 同期、別のタブ）の変更はこれで用意する。
func seedMetadata(t *testing.T, service *Service, metadata Metadata) {
	t.Helper()
	if err := service.metadata.EnsureDirectory(); err != nil {
		t.Fatal(err)
	}
	_, precondition, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	change, err := service.metadata.Change(metadata, precondition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.manager.Commit(storage.Request{Operation: "test.seed", Changes: []storage.Change{change}}); err != nil {
		t.Fatal(err)
	}
}

func loadMetadata(t *testing.T, service *Service) Metadata {
	t.Helper()
	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

// changeEnginePortElsewhere は、画面が読み込んだあとにほかの書き手が別の節を変えた状態を作る。
func changeEnginePortElsewhere(t *testing.T, service *Service) {
	t.Helper()
	stored := loadMetadata(t, service)
	stored.Engine = &EngineSettings{Port: 18422}
	seedMetadata(t, service, stored)
}

var bastion = HostIdentity{Path: "config", Alias: "bastion"}

func TestSavingOneHostsMetadataKeepsSectionsChangedElsewhere(t *testing.T) {
	service, _ := newTestService(t)
	changeEnginePortElsewhere(t, service)

	if _, err := service.Save(EditRequest{
		Kind: EditMetadata, Path: bastion.Path, Alias: bastion.Alias,
		HostMetadata: &HostMetadata{Identity: bastion, Tags: []string{"prod"}},
	}); err != nil {
		t.Fatal(err)
	}

	stored := loadMetadata(t, service)
	if stored.Engine == nil || stored.Engine.Port != 18422 {
		t.Fatalf("engine settings = %#v, want the port written elsewhere", stored.Engine)
	}
	if len(stored.Hosts) != 1 || stored.Hosts[0].Tags[0] != "prod" {
		t.Fatalf("hosts = %#v", stored.Hosts)
	}
}

func TestSavingOneHostsMetadataFromAStaleCopyIsRefused(t *testing.T) {
	service, _ := newTestService(t)
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{Identity: bastion, Note: "written elsewhere"}}
	seedMetadata(t, service, metadata)

	_, err := service.Save(EditRequest{
		Kind: EditMetadata, Path: bastion.Path, Alias: bastion.Alias,
		HostMetadataBase: &HostMetadata{Identity: bastion},
		HostMetadata:     &HostMetadata{Identity: bastion, Tags: []string{"prod"}},
	})
	if !errors.Is(err, ErrMetadataChanged) {
		t.Fatalf("Save error = %v, want ErrMetadataChanged", err)
	}
	if stored := loadMetadata(t, service); stored.Hosts[0].Note != "written elsewhere" {
		t.Fatalf("hosts = %#v", stored.Hosts)
	}
}

func TestSavingOneHostsMetadataKeepsTheOperatingSystemTheEngineDetected(t *testing.T) {
	service, _ := newTestService(t)
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{Identity: bastion, DetectedOS: "ubuntu", DetectedOSBinding: "binding"}}
	seedMetadata(t, service, metadata)

	// 画面は判定の前に読み込んだので、base と新しい entry には判定結果が無い。
	if _, err := service.Save(EditRequest{
		Kind: EditMetadata, Path: bastion.Path, Alias: bastion.Alias,
		HostMetadataBase: &HostMetadata{Identity: bastion},
		HostMetadata:     &HostMetadata{Identity: bastion, Colour: "#22d3ee"},
	}); err != nil {
		t.Fatal(err)
	}

	host := loadMetadata(t, service).Hosts[0]
	if host.DetectedOS != "ubuntu" || host.DetectedOSBinding != "binding" || host.Colour != "#22d3ee" {
		t.Fatalf("host = %#v", host)
	}
}

func TestReassociatingAnOrphanMovesItsMetadataToTheChosenHost(t *testing.T) {
	service, _ := newTestService(t)
	gone := HostIdentity{Path: "config", Alias: "retired"}
	nas := HostIdentity{Path: "conf.d/10-home.conf", Alias: "nas"}
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{Identity: gone, Note: "rack 3", Orphan: true}}
	seedMetadata(t, service, metadata)

	if _, err := service.Save(EditRequest{
		Kind: EditMetadata, Path: gone.Path, Alias: gone.Alias,
		HostMetadataBase: &HostMetadata{Identity: gone, Note: "rack 3", Orphan: true},
		HostMetadata:     &HostMetadata{Identity: nas, Note: "rack 3"},
	}); err != nil {
		t.Fatal(err)
	}

	hosts := loadMetadata(t, service).Hosts
	if len(hosts) != 1 || hosts[0].Identity != nas || hosts[0].Note != "rack 3" || hosts[0].Orphan {
		t.Fatalf("hosts = %#v", hosts)
	}
}

func TestDiscardingAnOrphanRemovesOnlyThatEntry(t *testing.T) {
	service, _ := newTestService(t)
	gone := HostIdentity{Path: "config", Alias: "retired"}
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{Identity: gone, Note: "old", Orphan: true}, {Identity: bastion, Note: "keep"}}
	seedMetadata(t, service, metadata)

	if _, err := service.Save(EditRequest{
		Kind: EditMetadata, Path: gone.Path, Alias: gone.Alias,
		HostMetadataBase: &HostMetadata{Identity: gone, Note: "old", Orphan: true},
	}); err != nil {
		t.Fatal(err)
	}

	hosts := loadMetadata(t, service).Hosts
	if len(hosts) != 1 || hosts[0].Identity != bastion || hosts[0].Note != "keep" {
		t.Fatalf("hosts = %#v", hosts)
	}
}

func TestSavingGroupsKeepsHostsChangedElsewhere(t *testing.T) {
	service, _ := newTestService(t)
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{Identity: bastion, Note: "written elsewhere"}}
	seedMetadata(t, service, metadata)

	if _, err := service.Save(EditRequest{Kind: EditGroups, Groups: []GroupMetadata{{Name: "work"}}}); err != nil {
		t.Fatal(err)
	}

	stored := loadMetadata(t, service)
	if len(stored.Groups) != 1 || len(stored.Hosts) != 1 || stored.Hosts[0].Note != "written elsewhere" {
		t.Fatalf("stored = %#v", stored)
	}
}

func TestSavingGroupsFromAStaleCopyIsRefused(t *testing.T) {
	service, _ := newTestService(t)
	metadata := NewMetadata()
	metadata.Groups = []GroupMetadata{{Name: "work", Colour: "#f97316"}}
	seedMetadata(t, service, metadata)

	_, err := service.Save(EditRequest{
		Kind:       EditGroups,
		GroupsBase: []GroupMetadata{{Name: "work"}},
		Groups:     []GroupMetadata{{Name: "work"}, {Name: "home"}},
	})
	if !errors.Is(err, ErrMetadataChanged) {
		t.Fatalf("Save error = %v, want ErrMetadataChanged", err)
	}
}

func TestSavingOneHostsMetadataFromAnOlderFileWithTwoEntriesForItKeepsOne(t *testing.T) {
	service, _ := newTestService(t)
	// schema 9 より前の sshc は、同じ接続の entry を 2 つ残すことがあった。
	acltest.WritePrivateFile(t, service.metadata.Path(), []byte(`{"schemaVersion":8,"hosts":[`+
		`{"identity":{"path":"config","alias":"bastion"},"note":"first"},`+
		`{"identity":{"path":"config","alias":"bastion"},"note":"second"}]}`))

	detail, err := service.HostDetail(bastion.Path, bastion.Alias)
	if err != nil {
		t.Fatal(err)
	}
	next := detail.Metadata
	next.Tags = []string{"prod"}
	if _, err := service.Save(EditRequest{
		Kind: EditMetadata, Path: bastion.Path, Alias: bastion.Alias,
		HostMetadataBase: &detail.Metadata,
		HostMetadata:     &next,
	}); err != nil {
		t.Fatalf("Save error = %v, want the copy HostDetail returned to be current", err)
	}

	hosts := loadMetadata(t, service).Hosts
	if len(hosts) != 1 || hosts[0].Note != "first" || len(hosts[0].Tags) != 1 {
		t.Fatalf("hosts = %#v, want one entry built from the copy the screen read", hosts)
	}
}

func TestRenamingOntoALeftoverOrphanKeepsOnlyTheRenamedHostsMetadata(t *testing.T) {
	service, _ := newTestService(t)
	jump := HostIdentity{Path: "config", Alias: "jump"}
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{Identity: bastion, Note: "bastion"}, {Identity: jump, Note: "retired jump", Orphan: true}}
	seedMetadata(t, service, metadata)

	if _, err := service.Save(EditRequest{
		Kind: EditRename, Path: "config", Base: serviceMainConfig, Alias: "bastion", NewAlias: "jump",
	}); err != nil {
		t.Fatal(err)
	}

	hosts := loadMetadata(t, service).Hosts
	if len(hosts) != 1 || hosts[0].Identity != jump || hosts[0].Note != "bastion" {
		t.Fatalf("hosts = %#v, want only the renamed host's entry", hosts)
	}
}
