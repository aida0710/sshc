package workspace_test

import (
	"errors"
	"testing"
	"time"

	"sshc/internal/storage"
	"sshc/internal/workspace"
)

func TestStoreRejectsMalformedPaneTrees(t *testing.T) {
	store := newStore(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	valid := workspace.Workspace{
		ID: "workspace-1", Name: "Production", Layout: pane("pane-1", "bastion"),
		FocusedPaneID: "pane-1", CreatedAt: now, UpdatedAt: now,
	}
	cases := map[string]func(*workspace.Workspace){
		"missing root": func(candidate *workspace.Workspace) { candidate.Layout = workspace.Node{} },
		"pane and split": func(candidate *workspace.Workspace) {
			candidate.Layout.Split = &workspace.Split{
				Direction: workspace.Horizontal, Ratio: 50,
				First: pane("pane-2", "web"), Second: pane("pane-3", "db"),
			}
		},
		"duplicate pane": func(candidate *workspace.Workspace) {
			candidate.Layout = split(workspace.Horizontal, pane("same", "web"), pane("same", "db"))
			candidate.FocusedPaneID = "same"
		},
		"unknown focus": func(candidate *workspace.Workspace) { candidate.FocusedPaneID = "missing" },
		"bad direction": func(candidate *workspace.Workspace) {
			candidate.Layout = split("diagonal", pane("pane-1", "web"), pane("pane-2", "db"))
		},
		"bad ratio": func(candidate *workspace.Workspace) {
			candidate.Layout = split(workspace.Vertical, pane("pane-1", "web"), pane("pane-2", "db"))
			candidate.Layout.Split.Ratio = 9
		},
		"more than four panes": func(candidate *workspace.Workspace) {
			candidate.Layout = split(workspace.Horizontal,
				split(workspace.Vertical, pane("pane-1", "web"), pane("pane-2", "db")),
				split(workspace.Vertical, pane("pane-3", "logs"), split(workspace.Horizontal,
					pane("pane-4", "metrics"), pane("pane-5", "worker"))),
			)
		},
		"control in alias": func(candidate *workspace.Workspace) { candidate.Layout.Pane.Alias = "db\nserver" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Layout = cloneLayout(valid.Layout)
			change(&candidate)
			if err := store.Save(candidate); !errors.Is(err, workspace.ErrInvalidWorkspace) {
				t.Fatalf("Save = %v, want ErrInvalidWorkspace", err)
			}
		})
	}
}

func TestAliasesFollowTheLayoutAndRemoveDuplicates(t *testing.T) {
	stored := workspace.Workspace{Layout: split(workspace.Horizontal,
		pane("one", "bastion"),
		split(workspace.Vertical, pane("two", "database"), workspace.Node{Pane: &workspace.Pane{ID: "three", Alias: "localhost", Kind: workspace.PaneShell}}),
	)}
	aliases := stored.Aliases()
	if len(aliases) != 2 || aliases[0] != "bastion" || aliases[1] != "database" {
		t.Fatalf("Aliases = %#v", aliases)
	}
}

func TestStoreAcceptsLocalShellPaneAndRejectsItsInvalidForms(t *testing.T) {
	store := newStore(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	valid := workspace.Workspace{
		ID: "workspace-local", Name: "Local", Layout: workspace.Node{Pane: &workspace.Pane{
			ID: "local-pane", Alias: "localhost", Kind: workspace.PaneShell,
		}}, FocusedPaneID: "local-pane", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Save(valid); err != nil {
		t.Fatalf("Save local pane = %v", err)
	}

	for name, test := range map[string]struct {
		kind  workspace.PaneKind
		alias string
	}{
		"unknown kind":           {kind: "serial", alias: "localhost"},
		"remote alias for shell": {kind: workspace.PaneShell, alias: "bastion"},
	} {
		candidate := valid
		candidate.ID = name
		candidate.Layout = workspace.Node{Pane: &workspace.Pane{ID: "local-pane", Alias: test.alias, Kind: test.kind}}
		if err := store.Save(candidate); !errors.Is(err, workspace.ErrInvalidWorkspace) {
			t.Fatalf("Save(%s) = %v, want ErrInvalidWorkspace", name, err)
		}
	}
}

func newStore(t *testing.T) *workspace.Store {
	t.Helper()
	home := t.TempDir()
	files, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	return workspace.NewStore(files)
}

func pane(id, alias string) workspace.Node {
	return workspace.Node{Pane: &workspace.Pane{ID: id, Alias: alias}}
}

func split(direction workspace.Direction, first, second workspace.Node) workspace.Node {
	return workspace.Node{Split: &workspace.Split{
		Direction: direction, Ratio: 50, First: first, Second: second,
	}}
}

func cloneLayout(source workspace.Node) workspace.Node {
	if source.Pane != nil {
		return pane(source.Pane.ID, source.Pane.Alias)
	}
	return workspace.Node{Split: &workspace.Split{
		Direction: source.Split.Direction,
		Ratio:     source.Split.Ratio,
		First:     cloneLayout(source.Split.First),
		Second:    cloneLayout(source.Split.Second),
	}}
}
