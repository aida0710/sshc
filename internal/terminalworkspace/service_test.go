package terminalworkspace

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/storage"
)

func newService(t *testing.T) (*Service, string) {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(workspace, nil), filepath.Join(workspace.Root(), filepath.FromSlash(PathRelative))
}

func ids(t *testing.T, service *Service) []string {
	t.Helper()
	workspaces, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, workspace := range workspaces {
		listed = append(listed, workspace.ID)
	}
	return listed
}

func TestWithoutAFileOnlyTheBuiltInWorkspacesExistAndAgentsIsHidden(t *testing.T) {
	service, _ := newService(t)
	workspaces, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 2 || workspaces[0].ID != DefaultID || workspaces[1].ID != AgentsID {
		t.Fatalf("workspaces = %+v", workspaces)
	}
	if workspaces[0].Hidden || !workspaces[1].Hidden {
		t.Fatalf("hidden flags = %+v", workspaces)
	}
}

func TestASplitLayoutFileFromBeforeIsTreatedAsAbsent(t *testing.T) {
	service, path := newService(t)
	old := `{"schemaVersion":2,"workspaces":[{"id":"w1","name":"Ops","layout":{"pane":{"id":"p","alias":"edge","kind":"ssh"}}}]}`
	acltest.WritePrivateFile(t, path, []byte(old))
	if got := ids(t, service); len(got) != 2 {
		t.Fatalf("ids = %v, want only the built-in workspaces", got)
	}
}

func TestANewerFileIsNotOverwritten(t *testing.T) {
	service, path := newService(t)
	acltest.WritePrivateFile(t, path, []byte(`{"schemaVersion":4,"workspaces":[]}`))
	if _, err := service.Create("Ops", false); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("Create = %v, want ErrUnsupportedVersion", err)
	}
}

func TestAWorkspaceIsCreatedRenamedHiddenAndKeptAcrossReads(t *testing.T) {
	service, _ := newService(t)
	created, err := service.Create("  sshc開発  ", false)
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "sshc開発" || created.BuiltIn() {
		t.Fatalf("created = %+v", created)
	}
	name, hidden := "学習", true
	if _, err := service.Update(created.ID, Change{Name: &name, Hidden: &hidden}); err != nil {
		t.Fatal(err)
	}
	workspaces, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 3 || workspaces[0].ID != DefaultID || workspaces[2].ID != AgentsID {
		t.Fatalf("order = %+v, want default first and agents last", workspaces)
	}
	if renamed := workspaces[1]; renamed.ID != created.ID || renamed.Name != "学習" || !renamed.Hidden {
		t.Fatalf("listed = %+v", workspaces)
	}
}

func TestOnlyABuiltInWorkspaceGoesBackToItsDefaultNameWithAnEmptyName(t *testing.T) {
	service, _ := newService(t)
	empty := ""
	if _, err := service.Update(DefaultID, Change{Name: &empty}); err != nil {
		t.Fatalf("built-in rename to empty = %v", err)
	}
	created, err := service.Create("Ops", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(created.ID, Change{Name: &empty}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("user workspace rename to empty = %v, want ErrInvalidName", err)
	}
	if _, err := service.Create("bad\nname", false); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("control character = %v, want ErrInvalidName", err)
	}
}

func TestDeletingAWorkspaceMovesItsSessionsToTheDefaultOne(t *testing.T) {
	service, _ := newService(t)
	created, err := service.Create("Ops", false)
	if err != nil {
		t.Fatal(err)
	}
	var moved [2]string
	if err := service.Delete(created.ID, func(from, to string) { moved = [2]string{from, to} }); err != nil {
		t.Fatal(err)
	}
	if moved != [2]string{created.ID, DefaultID} {
		t.Fatalf("moved = %v", moved)
	}
	if got := ids(t, service); len(got) != 2 {
		t.Fatalf("ids after delete = %v", got)
	}
	for _, builtIn := range []string{DefaultID, AgentsID} {
		if err := service.Delete(builtIn, func(string, string) {}); !errors.Is(err, ErrBuiltIn) {
			t.Fatalf("Delete(%s) = %v, want ErrBuiltIn", builtIn, err)
		}
	}
}

func TestNoMoreThanTheLimitCanBeCreated(t *testing.T) {
	service, _ := newService(t)
	for range MaxWorkspaces - 2 {
		if _, err := service.Create("w", false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.Create("one too many", false); !errors.Is(err, ErrLimit) {
		t.Fatalf("Create past the limit = %v, want ErrLimit", err)
	}
}

func TestAMovedWorkspaceKeepsItsPlaceAcrossReads(t *testing.T) {
	service, _ := newService(t)
	first, err := service.Create("A", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create("B", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(t, service); !slices.Equal(got, []string{DefaultID, first.ID, second.ID, AgentsID}) {
		t.Fatalf("created = %v, want each new one before agents", got)
	}
	if err := service.Move(second.ID, DefaultID); err != nil {
		t.Fatal(err)
	}
	if got := ids(t, service); !slices.Equal(got, []string{second.ID, DefaultID, first.ID, AgentsID}) {
		t.Fatalf("moved before default = %v", got)
	}
	if err := service.Move(DefaultID, ""); err != nil {
		t.Fatal(err)
	}
	if got := ids(t, service); !slices.Equal(got, []string{second.ID, first.ID, AgentsID, DefaultID}) {
		t.Fatalf("moved to the end = %v", got)
	}
	if err := service.Move(first.ID, first.ID); err != nil {
		t.Fatalf("moved before itself = %v", err)
	}
	if got := ids(t, service); !slices.Equal(got, []string{second.ID, first.ID, AgentsID, DefaultID}) {
		t.Fatalf("moved before itself = %v, want no change", got)
	}
}

func TestAWorkspaceCreatedAfterAgentsWasMovedUpGoesToTheEnd(t *testing.T) {
	service, _ := newService(t)
	if err := service.Move(AgentsID, DefaultID); err != nil {
		t.Fatal(err)
	}
	created, err := service.Create("Ops", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(t, service); !slices.Equal(got, []string{AgentsID, DefaultID, created.ID}) {
		t.Fatalf("ids = %v", got)
	}
}

func TestMovingAnUnknownWorkspaceOrBeforeOneIsRefused(t *testing.T) {
	service, _ := newService(t)
	if err := service.Move("missing", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Move(missing) = %v, want ErrNotFound", err)
	}
	if err := service.Move(DefaultID, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Move(before missing) = %v, want ErrNotFound", err)
	}
	if got := ids(t, service); !slices.Equal(got, []string{DefaultID, AgentsID}) {
		t.Fatalf("ids after the refusals = %v", got)
	}
}
