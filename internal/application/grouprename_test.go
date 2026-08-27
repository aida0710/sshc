package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/storage"
)

func groupRenameFixture(t *testing.T) (*Service, *storage.Workspace) {
	t.Helper()
	service, workspace := newTestService(t)
	declareGroup(t, service, "work", "work/eu")
	writeGroupFile(t, workspace, "work", "web.conf", "Host web-1\n\tHostName 203.0.113.10\n")
	writeGroupFile(t, workspace, "work/eu", "lon.conf", "Host lon-1\n\tHostName 203.0.113.11\n")
	writeKeyPair(t, workspace, "keys/work/id_work")
	if err := os.WriteFile(filepath.Join(workspace.Root(), "conf.d", "30-keys.conf"),
		[]byte("Host web-1\n\tIdentityFile ~/.ssh/keys/work/id_work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return service, workspace
}

func TestGroupRenameMovesEveryFileAndRewritesEveryLineThatNamedIt(t *testing.T) {
	service, workspace := groupRenameFixture(t)

	result, err := service.RenameGroup(keyInventory(t, workspace), "work", "client-a")
	if err != nil {
		t.Fatalf("RenameGroup error = %v", err)
	}
	wantRelocations := map[string]string{
		"keys/work/id_work":     "keys/client-a/id_work",
		"keys/work/id_work.pub": "keys/client-a/id_work.pub",
	}
	for _, relocation := range result.KeyRelocations {
		if wantRelocations[relocation.From] == relocation.To {
			delete(wantRelocations, relocation.From)
		}
	}
	if len(wantRelocations) != 0 {
		t.Errorf("KeyRelocations omitted %#v: %#v", wantRelocations, result.KeyRelocations)
	}

	for _, name := range []string{
		"connections/client-a/web.conf",
		"connections/client-a/eu/lon.conf",
		"keys/client-a/id_work",
		"keys/client-a/id_work.pub",
	} {
		if _, err := os.Lstat(filepath.Join(workspace.Root(), filepath.FromSlash(name))); err != nil {
			t.Errorf("%s is not there: %v", name, err)
		}
	}
	entry := readFile(t, workspace, "config")
	if !strings.Contains(entry, "Include connections/client-a/eu/*.conf\nInclude connections/client-a/*.conf\n") {
		t.Errorf("entry region = %q", entry)
	}
	if strings.Contains(entry, "connections/work") {
		t.Errorf("the old group is still declared: %q", entry)
	}
	if got := readFile(t, workspace, "conf.d/30-keys.conf"); got != "Host web-1\n\tIdentityFile ~/.ssh/keys/client-a/id_work\n" {
		t.Errorf("key reference = %q", got)
	}
}

func TestGroupRenameCarriesTheMetadataIdentityAndPresentation(t *testing.T) {
	service, workspace := groupRenameFixture(t)
	metadata := NewMetadata()
	metadata.Groups = []GroupMetadata{
		{Name: "work", Colour: "#f97316"},
		{Name: "work/eu"},
	}
	metadata.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "connections/work/web.conf", Alias: "web-1"},
		Colour:   "#22d3ee",
	}}
	if _, err := service.Save(EditRequest{Kind: EditMetadata, Metadata: &metadata}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.RenameGroup(keyInventory(t, workspace), "work", "client-a"); err != nil {
		t.Fatalf("RenameGroup error = %v", err)
	}

	stored, _, err := service.metadata.Load()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, group := range stored.Groups {
		names[group.Name] = group.Colour
	}
	if names["client-a"] != "#f97316" {
		t.Errorf("groups = %#v, want the presentation carried to the new name", stored.Groups)
	}
	if _, stale := names["work"]; stale {
		t.Errorf("the old group name survived: %#v", stored.Groups)
	}
	if len(stored.Hosts) != 1 {
		t.Fatalf("hosts = %#v", stored.Hosts)
	}
	if stored.Hosts[0].Identity.Path != "connections/client-a/web.conf" || stored.Hosts[0].Orphan {
		t.Errorf("identity = %#v", stored.Hosts[0])
	}
	if stored.Hosts[0].Colour != "#22d3ee" {
		t.Errorf("presentation lost on the way: %#v", stored.Hosts[0])
	}
}

func TestGroupRenameTakesTheDirectoriesItEmpties(t *testing.T) {
	service, workspace := groupRenameFixture(t)

	result, err := service.RenameGroup(keyInventory(t, workspace), "work", "client-a")
	if err != nil {
		t.Fatalf("RenameGroup error = %v", err)
	}
	if hasNotice(result.Preview.Notices, NoticeGroupDirectoryLeftover, "work") {
		t.Errorf("it still reports a leftover it now removes: %#v", result.Preview.Notices)
	}
	for _, name := range []string{"connections/work/eu", "connections/work", "keys/work"} {
		if _, err := os.Stat(filepath.Join(workspace.Root(), filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Errorf("%s is still there: %v", name, err)
		}
	}
}

func TestGroupRenameLeavesADirectoryThatStillHoldsSomething(t *testing.T) {
	service, workspace := groupRenameFixture(t)
	stray := filepath.Join(workspace.Root(), "connections", "work", "scratch")
	if err := os.MkdirAll(stray, 0o700); err != nil {
		t.Fatal(err)
	}

	result, err := service.RenameGroup(keyInventory(t, workspace), "work", "client-a")
	if err != nil {
		t.Fatalf("RenameGroup error = %v", err)
	}
	if !hasNotice(result.Preview.Notices, NoticeGroupDirectoryLeftover, "work") {
		t.Errorf("notices = %#v, want group_directory_leftover", result.Preview.Notices)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Errorf("the file the user left there is gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace.Root(), "connections", "work", "eu")); !os.IsNotExist(err) {
		t.Errorf("connections/work/eu is still there: %v", err)
	}
}

func TestGroupRenameOntoAnExistingGroupIsRefused(t *testing.T) {
	service, workspace := groupRenameFixture(t)
	declareGroup(t, service, "work", "work/eu", "client-a")

	_, err := service.RenameGroup(keyInventory(t, workspace), "work", "client-a")
	if !errors.Is(err, ErrGroupExists) {
		t.Fatalf("RenameGroup error = %v, want ErrGroupExists", err)
	}
	if _, statErr := os.Lstat(filepath.Join(workspace.Root(), "connections", "work", "web.conf")); statErr != nil {
		t.Errorf("a refused rename moved a file: %v", statErr)
	}
}

func TestGroupRenameRefusesWhenAKeyReferenceCannotBeRewritten(t *testing.T) {
	service, workspace := groupRenameFixture(t)
	if err := os.WriteFile(filepath.Join(workspace.Root(), "conf.d", "40-relative.conf"),
		[]byte("Host other\n\tIdentityFile id_work\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var blocked *GroupBlockedError
	_, err := service.RenameGroup(keyInventory(t, workspace), "work", "client-a")
	if !errors.As(err, &blocked) {
		t.Fatalf("RenameGroup error = %v, want *GroupBlockedError", err)
	}
	if len(blocked.Blockers) == 0 || !strings.HasPrefix(blocked.Blockers[0], BlockerKeyUnresolved) {
		t.Errorf("blockers = %#v", blocked.Blockers)
	}
	if _, statErr := os.Lstat(filepath.Join(workspace.Root(), "keys", "work", "id_work")); statErr != nil {
		t.Errorf("a blocked rename moved the key: %v", statErr)
	}
}

func TestGroupRenameLeavesEveryByteOutsideTheRegionAlone(t *testing.T) {
	service, workspace := groupRenameFixture(t)
	entry := readFile(t, workspace, "config")
	const external = "\nHost added-by-hand\n\tHostName 192.0.2.99\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(entry+external), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := service.RenameGroup(keyInventory(t, workspace), "work", "client-a"); err != nil {
		t.Fatalf("RenameGroup error = %v", err)
	}

	if !strings.HasSuffix(readFile(t, workspace, "config"), external) {
		t.Errorf("the hand-written block was disturbed:\n%s", readFile(t, workspace, "config"))
	}
}

func TestDeleteGroupRelocatesItsConnectionsAndNeverDeletesAFile(t *testing.T) {
	service, workspace := groupRenameFixture(t)
	declareGroup(t, service, "work", "work/eu", "archive")

	if _, err := service.DeleteGroup(keyInventory(t, workspace), "work", "archive"); err != nil {
		t.Fatalf("DeleteGroup error = %v", err)
	}

	for _, name := range []string{"connections/archive/web.conf", "connections/archive/lon.conf"} {
		if _, err := os.Lstat(filepath.Join(workspace.Root(), filepath.FromSlash(name))); err != nil {
			t.Errorf("%s is not there: %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(workspace.Root(), "keys", "archive", "id_work")); err != nil {
		t.Errorf("the group's key did not follow: %v", err)
	}
	entry := readFile(t, workspace, "config")
	if strings.Contains(entry, "connections/work") {
		t.Errorf("the deleted group is still declared: %q", entry)
	}
	if !strings.Contains(entry, "Include connections/archive/*.conf") {
		t.Errorf("the destination is not declared: %q", entry)
	}
}

func TestDeleteGroupRefusesADestinationThatIsNotDeclared(t *testing.T) {
	service, workspace := groupRenameFixture(t)

	if _, err := service.DeleteGroup(keyInventory(t, workspace), "work", "archive"); !errors.Is(err, ErrGroupNotDeclared) {
		t.Fatalf("DeleteGroup error = %v, want ErrGroupNotDeclared", err)
	}
	if _, err := service.DeleteGroup(keyInventory(t, workspace), "marketing", ""); !errors.Is(err, ErrGroupNotDeclared) {
		t.Fatalf("DeleteGroup of an undeclared group = %v, want ErrGroupNotDeclared", err)
	}
}

func TestGroupOperationsRefuseANameThatIsNotASafeDirectory(t *testing.T) {
	service, workspace := groupRenameFixture(t)
	inventory := keyInventory(t, workspace)

	for _, name := range []string{"../escape", "", "sshc"} {
		if _, err := service.RenameGroup(inventory, "work", name); !errors.Is(err, ErrInvalidGroupName) {
			t.Errorf("RenameGroup to %q = %v, want ErrInvalidGroupName", name, err)
		}
	}
	if _, err := service.RenameGroup(inventory, "work", "work/inside"); !errors.Is(err, ErrGroupSelfNesting) {
		t.Errorf("nesting a group inside itself was allowed")
	}
}

func TestDeleteGroupWarnsThatItsConnectionsWillBeUnreached(t *testing.T) {
	service, workspace := newTestService(t)
	declareGroup(t, service, "work")
	writeGroupFile(t, workspace, "work", "hosts.conf", "Host inwork\n\tUser aida\n")

	result, err := service.DeleteGroup(keyInventory(t, workspace), "work", "")
	if err != nil {
		t.Fatalf("DeleteGroup error = %v", err)
	}
	found := false
	for _, notice := range result.Preview.Notices {
		if notice.Code == NoticeGroupFileUnreached {
			found = true
		}
	}
	if !found {
		t.Errorf("notices = %#v, want group_file_unreached", result.Preview.Notices)
	}
}

func TestDeleteGroupIntoAnotherGroupDoesNotWarn(t *testing.T) {
	service, workspace := newTestService(t)
	declareGroup(t, service, "work", "keep")
	writeGroupFile(t, workspace, "work", "hosts.conf", "Host inwork\n\tUser aida\n")

	result, err := service.DeleteGroup(keyInventory(t, workspace), "work", "keep")
	if err != nil {
		t.Fatalf("DeleteGroup error = %v", err)
	}
	for _, notice := range result.Preview.Notices {
		if notice.Code == NoticeGroupFileUnreached {
			t.Errorf("a connection that stayed inside a group was called unreached: %#v", notice)
		}
	}
}

func TestDeleteGroupHoldingAKeyNeedsNoDestination(t *testing.T) {
	service, workspace := groupRenameFixture(t)

	if _, err := service.DeleteGroup(keyInventory(t, workspace), "work", ""); err != nil {
		t.Fatalf("DeleteGroup error = %v", err)
	}
	for _, name := range []string{"keys/id_work", "keys/id_work.pub", "connections/web.conf"} {
		if _, err := os.Lstat(filepath.Join(workspace.Root(), filepath.FromSlash(name))); err != nil {
			t.Errorf("%s is not there: %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(workspace.Root(), "id_work")); err == nil {
		t.Error("the key was left loose in the workspace root")
	}
}
