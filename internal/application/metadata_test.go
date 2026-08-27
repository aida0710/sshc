package application

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/storage"
	"sshc/internal/terminal"
)

func newTestWorkspace(t *testing.T) *storage.Workspace {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.EnsureDirectory(workspace.Root()); err != nil {
		t.Fatal(err)
	}
	return workspace
}

func TestDecodeMetadataAcceptsAnAbsentFileAndRejectsUnsupportedSchemas(t *testing.T) {
	empty, err := DecodeMetadata(nil)
	if err != nil {
		t.Fatal(err)
	}
	if empty.SchemaVersion != MetadataSchemaVersion || len(empty.Hosts) != 0 || len(empty.Groups) != 0 {
		t.Fatalf("empty metadata = %#v", empty)
	}
	if empty.EmbeddedTerminal != nil {
		t.Errorf("a new document already carries terminal settings = %#v", empty.EmbeddedTerminal)
	}
	if limits := empty.TerminalLimits(); limits != terminal.DefaultLimits() {
		t.Errorf("default limits = %#v, want %#v", limits, terminal.DefaultLimits())
	}
	if _, err := DecodeMetadata([]byte(`{"schemaVersion":99}`)); !errors.Is(err, ErrMetadataVersion) {
		t.Fatalf("future schema error = %v, want ErrMetadataVersion", err)
	}
	if _, err := DecodeMetadata([]byte(`{"schemaVersion":1,`)); err == nil {
		t.Fatal("truncated metadata was accepted")
	}
	if _, err := DecodeMetadata([]byte(`{"schemaVersion":2,"hosts":[]}`)); !errors.Is(err, ErrMetadataVersion) {
		t.Fatalf("old schema error = %v, want ErrMetadataVersion", err)
	}
}

func TestDecodeMetadataFallsBackToTheDefaultLimits(t *testing.T) {
	for name, document := range map[string]string{
		"zero":        `{"schemaVersion":3,"embeddedTerminal":{"maxSessions":0,"scrollbackBytes":0}}`,
		"below range": `{"schemaVersion":3,"embeddedTerminal":{"maxSessions":0,"scrollbackBytes":1}}`,
		"above range": `{"schemaVersion":3,"embeddedTerminal":{"maxSessions":9999,"scrollbackBytes":99999999}}`,
		"negative":    `{"schemaVersion":3,"embeddedTerminal":{"maxSessions":-1,"scrollbackBytes":-1}}`,
	} {
		decoded, err := DecodeMetadata([]byte(document))
		if err != nil {
			t.Fatalf("%s = %v, want the document to survive", name, err)
		}
		if limits := decoded.TerminalLimits(); limits != terminal.DefaultLimits() {
			t.Errorf("%s limits = %#v, want the defaults", name, limits)
		}
	}

	kept, err := DecodeMetadata([]byte(`{"schemaVersion":3,"embeddedTerminal":{"maxSessions":8,"scrollbackBytes":32768}}`))
	if err != nil {
		t.Fatal(err)
	}
	if limits := kept.TerminalLimits(); limits.MaxSessions != 8 || limits.Scrollback != 32768 {
		t.Fatalf("limits = %#v", limits)
	}
}

func TestEncodeMetadataRefusesLimitsOutsideTheirRange(t *testing.T) {
	for name, settings := range map[string]EmbeddedTerminal{
		"too many sessions":    {MaxSessions: terminal.MaxMaxSessions + 1, ScrollbackBytes: terminal.DefaultScrollback},
		"scrollback too small": {MaxSessions: 1, ScrollbackBytes: terminal.MinScrollback - 1},
		"scrollback too large": {MaxSessions: 1, ScrollbackBytes: terminal.MaxScrollback + 1},
	} {
		broken := NewMetadata()
		broken.EmbeddedTerminal = &settings
		if _, err := EncodeMetadata(broken); !errors.Is(err, ErrMetadataTerminal) {
			t.Errorf("%s = %v, want ErrMetadataTerminal", name, err)
		}
	}

	onlyTheDirectory := NewMetadata()
	onlyTheDirectory.EmbeddedTerminal = &EmbeddedTerminal{StartDirectory: "~/work"}
	written, err := EncodeMetadata(onlyTheDirectory)
	if err != nil {
		t.Fatalf("a document that carries only the start directory = %v", err)
	}
	restored, err := DecodeMetadata(written)
	if err != nil {
		t.Fatal(err)
	}
	if restored.TerminalStartDirectory() != "~/work" {
		t.Fatalf("the start directory did not survive the round trip: %q", restored.TerminalStartDirectory())
	}

	kept := NewMetadata()
	kept.EmbeddedTerminal = &EmbeddedTerminal{MaxSessions: 8, ScrollbackBytes: 32768}
	encoded, err := EncodeMetadata(kept)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMetadata(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if limits := decoded.TerminalLimits(); limits.MaxSessions != 8 || limits.Scrollback != 32768 {
		t.Fatalf("round trip = %#v", limits)
	}
}

func TestValidateMetadataRefusesKeyMaterialAndUnknownPaths(t *testing.T) {
	withNote := NewMetadata()
	withNote.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "config", Alias: "bastion"},
		Note:     "-----BEGIN OPENSSH PRIVATE KEY-----",
	}}
	if err := ValidateMetadata(withNote); !errors.Is(err, ErrMetadataSecret) {
		t.Fatalf("note error = %v, want ErrMetadataSecret", err)
	}

	withTag := NewMetadata()
	withTag.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "config", Alias: "bastion"},
		Tags:     []string{"ssh-rsa AAAAB3NzaC1yc2EAAAA"},
	}}
	if err := ValidateMetadata(withTag); !errors.Is(err, ErrMetadataSecret) {
		t.Fatalf("tag error = %v, want ErrMetadataSecret", err)
	}

	withAbsolutePath := NewMetadata()
	withAbsolutePath.Hosts = []HostMetadata{{Identity: HostIdentity{Path: "/etc/ssh/ssh_config", Alias: "x"}}}
	if err := ValidateMetadata(withAbsolutePath); !errors.Is(err, ErrMetadataPath) {
		t.Fatalf("path error = %v, want ErrMetadataPath", err)
	}

	for _, name := range []string{"../escape", "", "sshc", "work/"} {
		withBadGroup := NewMetadata()
		withBadGroup.Groups = []GroupMetadata{{Name: name}}
		if err := ValidateMetadata(withBadGroup); !errors.Is(err, ErrMetadataGroup) {
			t.Errorf("group %q error = %v, want ErrMetadataGroup", name, err)
		}
	}

	withCaseClash := NewMetadata()
	withCaseClash.Groups = []GroupMetadata{{Name: "work"}, {Name: "Work"}}
	if err := ValidateMetadata(withCaseClash); !errors.Is(err, ErrMetadataGroup) {
		t.Errorf("case clash error = %v, want ErrMetadataGroup", err)
	}
}

func TestMetadataCarriesOnlyPresentation(t *testing.T) {
	metadata := NewMetadata()
	metadata.Groups = []GroupMetadata{{Name: "work", Colour: "#f97316", Note: "the office", Order: 2}}
	metadata.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "connections/work/web.conf", Alias: "web-1"},
		Tags:     []string{"prod"},
		Colour:   "#22d3ee",
	}}

	encoded, err := EncodeMetadata(metadata)
	if err != nil {
		t.Fatalf("EncodeMetadata error = %v", err)
	}
	for _, absent := range []string{`"group"`, `"parent"`, `"terminal"`, `"customTerminal"`} {
		if strings.Contains(string(encoded), absent) {
			t.Errorf("encoded metadata still carries %s:\n%s", absent, encoded)
		}
	}
	if !strings.Contains(string(encoded), `"schemaVersion": 3`) {
		t.Errorf("encoded metadata is not version 3:\n%s", encoded)
	}
}

func TestMetadataStoreRoundTripsThroughOneTransaction(t *testing.T) {
	workspace := newTestWorkspace(t)
	store := NewMetadataStore(workspace)

	loaded, precondition, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if precondition.Exists {
		t.Fatalf("precondition for an absent file = %#v", precondition)
	}
	loaded.Groups = []GroupMetadata{{Name: "home", Settings: []Setting{{Keyword: "User", Values: []string{"aida"}}}}}
	loaded.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "config", Alias: "bastion"},
		Tags:     []string{"personal"},
		Colour:   "#22d3ee",
		Note:     "office jump host",
		Order:    1,
	}}

	change, err := store.Change(loaded, precondition)
	if err != nil {
		t.Fatal(err)
	}
	if change.Path != store.Path() || change.Precondition.Exists {
		t.Fatalf("change = %#v", change)
	}
	if err := store.EnsureDirectory(); err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, change.Path, change.Contents)

	reloaded, reloadedPrecondition, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reloadedPrecondition.Exists || reloadedPrecondition.Digest != storage.Digest(change.Contents) {
		t.Fatalf("reloaded precondition = %#v", reloadedPrecondition)
	}
	if len(reloaded.Hosts) != 1 || reloaded.Hosts[0].Alias() != "bastion" || reloaded.Hosts[0].Colour != "#22d3ee" {
		t.Fatalf("reloaded hosts = %#v", reloaded.Hosts)
	}
	if got := string(change.Contents); !strings.HasSuffix(got, "\n") {
		t.Fatal("encoded metadata must end with a newline")
	}
	if store.Path() != filepath.Join(workspace.StateDir(), MetadataFileName) {
		t.Fatalf("store path = %q", store.Path())
	}
}

func TestMetadataStoreDropsTheRetiredFavouriteFieldOnTheNextWrite(t *testing.T) {
	workspace := newTestWorkspace(t)
	store := NewMetadataStore(workspace)
	if err := store.EnsureDirectory(); err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, store.Path(), []byte(
		`{"schemaVersion":3,"hosts":[{"identity":{"path":"config","alias":"bastion"},"favourite":true,"tags":["prod"]}]}`,
	))

	loaded, precondition, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Hosts) != 1 || len(loaded.Hosts[0].Tags) != 1 {
		t.Fatalf("loaded hosts = %#v", loaded.Hosts)
	}
	change, err := store.Change(loaded, precondition)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(change.Contents), "favourite") {
		t.Fatalf("retired field survived: %s", change.Contents)
	}
}

func TestReconcileMetadataMarksVanishedTargetsAsOrphansWithoutGuessing(t *testing.T) {
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{
		{Identity: HostIdentity{Path: "config", Alias: "bastion"}, Note: "kept"},
		{Identity: HostIdentity{Path: "conf.d/10-home.conf", Alias: "nas"}, Note: "vanished"},
	}
	present := []HostIdentity{
		{Path: "config", Alias: "bastion"},
		{Path: "conf.d/10-home.conf", Alias: "nas-new"},
	}

	reconciled, notices := ReconcileMetadata(metadata, present)
	if reconciled.Hosts[0].Orphan {
		t.Fatalf("present host became an orphan: %#v", reconciled.Hosts[0])
	}
	if !reconciled.Hosts[1].Orphan || reconciled.Hosts[1].Note != "vanished" {
		t.Fatalf("orphan entry = %#v", reconciled.Hosts[1])
	}
	if reconciled.Hosts[1].Identity.Alias != "nas" {
		t.Fatal("an orphan must keep its original identity instead of being re-pointed")
	}
	if len(notices) != 1 || notices[0].Code != NoticeOrphanMetadata || notices[0].Path != "conf.d/10-home.conf" {
		t.Fatalf("notices = %#v", notices)
	}
}

func TestRenameHostIdentityMovesExactlyOneEntry(t *testing.T) {
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{
		{Identity: HostIdentity{Path: "config", Alias: "bastion"}, Note: "renamed"},
		{Identity: HostIdentity{Path: "config", Alias: "nas"}, Note: "untouched"},
	}
	renamed := RenameHostIdentity(metadata,
		HostIdentity{Path: "config", Alias: "bastion"},
		HostIdentity{Path: "config", Alias: "jump"},
	)
	if renamed.Hosts[0].Identity.Alias != "jump" || renamed.Hosts[0].Note != "renamed" || renamed.Hosts[0].Orphan {
		t.Fatalf("renamed entry = %#v", renamed.Hosts[0])
	}
	if renamed.Hosts[1].Identity.Alias != "nas" {
		t.Fatalf("second entry = %#v", renamed.Hosts[1])
	}
	if metadata.Hosts[0].Identity.Alias != "bastion" {
		t.Fatal("RenameHostIdentity must not mutate its input")
	}
}

func TestGroupMetadataCarriesTheHiddenFlagThroughARoundTrip(t *testing.T) {
	metadata := NewMetadata()
	metadata.Groups = []GroupMetadata{{Name: "dubguild", Hidden: true}, {Name: "dubguild/mdx"}}

	encoded, err := EncodeMetadata(metadata)
	if err != nil {
		t.Fatalf("EncodeMetadata error = %v", err)
	}
	if !strings.Contains(string(encoded), `"hidden": true`) {
		t.Errorf("encoded metadata lost the hidden flag:\n%s", encoded)
	}

	decoded, err := DecodeMetadata(encoded)
	if err != nil {
		t.Fatalf("DecodeMetadata error = %v", err)
	}
	if len(decoded.Groups) != 2 || !decoded.Groups[0].Hidden || decoded.Groups[1].Hidden {
		t.Errorf("groups = %#v, want only dubguild hidden", decoded.Groups)
	}
}

func TestAGroupThatIsNotHiddenWritesNoHiddenKey(t *testing.T) {
	metadata := NewMetadata()
	metadata.Groups = []GroupMetadata{{Name: "work"}}

	encoded, err := EncodeMetadata(metadata)
	if err != nil {
		t.Fatalf("EncodeMetadata error = %v", err)
	}
	if strings.Contains(string(encoded), "hidden") {
		t.Errorf("encoded metadata carries a hidden key it did not need:\n%s", encoded)
	}
}
