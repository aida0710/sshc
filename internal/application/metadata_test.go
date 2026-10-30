package application

import (
	"errors"
	"fmt"
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
	if len(empty.Presets) != 1 || !sameSettings(empty.Presets[0], SettingsPreset{ID: DefaultPresetID}) || empty.Settings != nil {
		t.Errorf("a new document already carries settings = %#v, %#v", empty.Presets, empty.Settings)
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

// 同期で届いた標準のプリセットの上限が範囲の外なら、metadata は読め、標準のプリセットが既定に戻る。
func TestAStandardPresetWithLimitsOutOfRangeIsResetWhenRead(t *testing.T) {
	for name, document := range map[string]string{
		"below range": `{"schemaVersion":10,"presets":[{"id":"default","terminal":{"scrollbackBytes":1}}]}`,
		"above range": `{"schemaVersion":10,"presets":[{"id":"default","terminal":{"maxSessions":9999,"scrollbackBytes":99999999}}]}`,
		"negative":    `{"schemaVersion":10,"presets":[{"id":"default","terminal":{"maxSessions":-1,"scrollbackBytes":-1}}]}`,
	} {
		decoded, err := DecodeMetadata([]byte(document))
		if err != nil {
			t.Fatalf("%s = %v, want the document to survive", name, err)
		}
		if decoded.Presets[0].Terminal != nil {
			t.Errorf("%s: standard preset terminal = %#v, want it reset", name, decoded.Presets[0].Terminal)
		}
	}

	kept, err := DecodeMetadata([]byte(`{"schemaVersion":10,"presets":[{"id":"default","terminal":{"maxSessions":8,"scrollbackBytes":32768}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if terminal := kept.Presets[0].Terminal; terminal == nil || terminal.MaxSessions != 8 || terminal.ScrollbackBytes != 32768 {
		t.Fatalf("terminal = %#v", terminal)
	}
}

func TestEncodeMetadataRefusesLimitsOutsideTheirRange(t *testing.T) {
	for name, settings := range map[string]EmbeddedTerminal{
		"too many sessions":    {MaxSessions: terminal.MaxMaxSessions + 1, ScrollbackBytes: terminal.DefaultScrollback},
		"scrollback too small": {MaxSessions: 1, ScrollbackBytes: terminal.MinScrollback - 1},
		"scrollback too large": {MaxSessions: 1, ScrollbackBytes: terminal.MaxScrollback + 1},
	} {
		broken := NewMetadata()
		broken.Presets[0].Terminal = &settings
		if _, err := EncodeMetadata(broken); !errors.Is(err, ErrMetadataTerminal) {
			t.Errorf("%s = %v, want ErrMetadataTerminal", name, err)
		}
	}

	onlyTheDirectory := NewMetadata()
	onlyTheDirectory.Presets[0].Terminal = &EmbeddedTerminal{StartDirectory: "~/work"}
	written, err := EncodeMetadata(onlyTheDirectory)
	if err != nil {
		t.Fatalf("a document that carries only the start directory = %v", err)
	}
	restored, err := DecodeMetadata(written)
	if err != nil {
		t.Fatal(err)
	}
	if terminal := restored.Presets[0].Terminal; terminal == nil || terminal.StartDirectory != "~/work" {
		t.Fatalf("the start directory did not survive the round trip: %#v", terminal)
	}

	kept := NewMetadata()
	kept.Presets[0].Terminal = &EmbeddedTerminal{MaxSessions: 8, ScrollbackBytes: 32768}
	encoded, err := EncodeMetadata(kept)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMetadata(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if terminal := decoded.Presets[0].Terminal; terminal == nil || terminal.MaxSessions != 8 || terminal.ScrollbackBytes != 32768 {
		t.Fatalf("round trip = %#v", terminal)
	}
}

func TestMetadataDropsThePerConnectionOSC52PolicyEarlierVersionsWrote(t *testing.T) {
	stored := []byte(`{"schemaVersion": 10, "hosts": [{"identity": {"path": "config", "alias": "host"}, "tags": ["prod"], "osc52": "allow"}]}`)
	decoded, err := DecodeMetadata(stored)
	if err != nil {
		t.Fatalf("decode = %v", err)
	}
	encoded, err := EncodeMetadata(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "osc52") || !strings.Contains(string(encoded), `"prod"`) {
		t.Fatalf("encoded = %s", encoded)
	}
}

func TestValidateMetadataRefusesKeyMaterialAndUnknownPaths(t *testing.T) {
	withNote := NewMetadata()
	withNote.Hosts = []HostMetadata{{
		Identity: HostIdentity{Path: "config", Alias: "bastion"},
		Tags:     []string{"-----BEGIN OPENSSH PRIVATE KEY-----"},
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

// ProxyCommand などは行の残りを 1 つの値で持つ。複数の値に分けた形は、同じ値から違う行が
// できないよう、受け付けない。
func TestValidateMetadataRefusesSeveralValuesForAKeywordThatTakesTheRestOfTheLine(t *testing.T) {
	split := NewMetadata()
	split.Groups = []GroupMetadata{{Name: "company", Settings: []Setting{
		{Keyword: "proxycommand", Values: []string{"sh", "-c", "exec nc %h %p"}},
	}}}
	if err := ValidateMetadata(split); !errors.Is(err, ErrMetadataGroup) {
		t.Errorf("split ProxyCommand error = %v, want ErrMetadataGroup", err)
	}

	whole := NewMetadata()
	whole.Groups = []GroupMetadata{{Name: "company", Settings: []Setting{
		{Keyword: "ProxyCommand", Values: []string{"sh -c 'exec nc %h %p'"}},
		{Keyword: "SendEnv", Values: []string{"LANG", "LC_*"}},
	}}}
	if err := ValidateMetadata(whole); err != nil {
		t.Errorf("one ProxyCommand value and several SendEnv values: %v", err)
	}
}

func TestMetadataAcceptsOnlyCanonicalTerminalEncodings(t *testing.T) {
	for _, encoding := range []string{"", "utf-8", "shift_jis", "euc-jp", "iso-2022-jp"} {
		metadata := NewMetadata()
		metadata.Hosts = []HostMetadata{{
			Identity: HostIdentity{Path: "config", Alias: "legacy"}, Encoding: encoding,
		}}
		if _, err := EncodeMetadata(metadata); err != nil {
			t.Errorf("encoding %q = %v", encoding, err)
		}
	}
	for _, encoding := range []string{"sjis", "UTF-8", "guess"} {
		metadata := NewMetadata()
		metadata.Hosts = []HostMetadata{{
			Identity: HostIdentity{Path: "config", Alias: "legacy"}, Encoding: encoding,
		}}
		if _, err := EncodeMetadata(metadata); !errors.Is(err, ErrMetadataEncoding) {
			t.Errorf("encoding %q = %v, want ErrMetadataEncoding", encoding, err)
		}
	}
}

func TestMetadataCarriesOnlyPresentation(t *testing.T) {
	metadata := NewMetadata()
	metadata.Groups = []GroupMetadata{{Name: "work", Colour: "#f97316", Order: 2}}
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
	if !strings.Contains(string(encoded), `"schemaVersion": 10`) {
		t.Errorf("encoded metadata is not the current version:\n%s", encoded)
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

func TestReconcileMetadataMarksVanishedTargetsAsOrphansWithoutGuessing(t *testing.T) {
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{
		{Identity: HostIdentity{Path: "config", Alias: "bastion"}, Tags: []string{"kept"}},
		{Identity: HostIdentity{Path: "conf.d/10-home.conf", Alias: "nas"}, Tags: []string{"vanished"}},
	}
	present := []HostIdentity{
		{Path: "config", Alias: "bastion"},
		{Path: "conf.d/10-home.conf", Alias: "nas-new"},
	}

	reconciled, notices := ReconcileMetadata(metadata, present)
	if reconciled.Hosts[0].Orphan {
		t.Fatalf("present host became an orphan: %#v", reconciled.Hosts[0])
	}
	if !reconciled.Hosts[1].Orphan || strings.Join(reconciled.Hosts[1].Tags, ",") != "vanished" {
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
		{Identity: HostIdentity{Path: "config", Alias: "bastion"}, Tags: []string{"renamed"}},
		{Identity: HostIdentity{Path: "config", Alias: "nas"}, Tags: []string{"untouched"}},
	}
	renamed := RenameHostIdentity(metadata,
		HostIdentity{Path: "config", Alias: "bastion"},
		HostIdentity{Path: "config", Alias: "jump"},
	)
	if renamed.Hosts[0].Identity.Alias != "jump" || strings.Join(renamed.Hosts[0].Tags, ",") != "renamed" || renamed.Hosts[0].Orphan {
		t.Fatalf("renamed entry = %#v", renamed.Hosts[0])
	}
	if renamed.Hosts[1].Identity.Alias != "nas" {
		t.Fatalf("second entry = %#v", renamed.Hosts[1])
	}
	if metadata.Hosts[0].Identity.Alias != "bastion" {
		t.Fatal("RenameHostIdentity must not mutate its input")
	}
}

func TestRenameHostIdentityReplacesAnEntryLeftAtTheDestination(t *testing.T) {
	bastion := HostIdentity{Path: "config", Alias: "bastion"}
	jump := HostIdentity{Path: "config", Alias: "jump"}
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{Identity: jump, Tags: []string{"retired"}, Orphan: true}, {Identity: bastion, Tags: []string{"renamed"}}}

	renamed := RenameHostIdentity(metadata, bastion, jump)

	if len(renamed.Hosts) != 1 || renamed.Hosts[0].Identity != jump || strings.Join(renamed.Hosts[0].Tags, ",") != "renamed" {
		t.Fatalf("hosts = %#v, want only the renamed entry", renamed.Hosts)
	}
}

func TestRenameHostIdentityKeepsTheDestinationEntryWhenNothingMoves(t *testing.T) {
	jump := HostIdentity{Path: "config", Alias: "jump"}
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{{Identity: jump, Tags: []string{"retired"}, Orphan: true}}

	renamed := RenameHostIdentity(metadata, HostIdentity{Path: "config", Alias: "bastion"}, jump)

	if len(renamed.Hosts) != 1 || strings.Join(renamed.Hosts[0].Tags, ",") != "retired" {
		t.Fatalf("hosts = %#v, want the destination entry kept", renamed.Hosts)
	}
}

func TestRelocateHostIdentitiesReplacesEntriesLeftAtTheDestinationWithTheSameAlias(t *testing.T) {
	metadata := NewMetadata()
	metadata.Hosts = []HostMetadata{
		{Identity: HostIdentity{Path: "conf.d/new.conf", Alias: "nas"}, Tags: []string{"retired"}, Orphan: true},
		{Identity: HostIdentity{Path: "conf.d/new.conf", Alias: "printer"}, Tags: []string{"other host"}},
		{Identity: HostIdentity{Path: "conf.d/old.conf", Alias: "nas"}, Tags: []string{"moved"}},
	}

	relocated := RelocateHostIdentities(metadata, "conf.d/old.conf", "conf.d/new.conf")

	if len(relocated.Hosts) != 2 {
		t.Fatalf("hosts = %#v, want the leftover nas entry dropped", relocated.Hosts)
	}
	for _, host := range relocated.Hosts {
		if host.Identity.Path != "conf.d/new.conf" {
			t.Fatalf("host = %#v, want every entry under the destination", host)
		}
		if host.Identity.Alias == "nas" && strings.Join(host.Tags, ",") != "moved" {
			t.Fatalf("nas = %#v, want the moved entry", host)
		}
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

// これより新しいバージョンのファイルは読まない。知らない項目を読み飛ばして書き直すと消える。
func TestMetadataFromANewerSshcIsRefused(t *testing.T) {
	newer := fmt.Sprintf(`{"schemaVersion":%d}`, MetadataSchemaVersion+1)
	if _, err := DecodeMetadata([]byte(newer)); !errors.Is(err, ErrMetadataVersion) {
		t.Fatalf("DecodeMetadata = %v", err)
	}
}

// 前の schema の metadata.json は読み替えずに断る。
func TestMetadataFromAnOlderSchemaIsRefused(t *testing.T) {
	for _, version := range []int{3, 8} {
		document := fmt.Sprintf(`{"schemaVersion":%d,"hosts":[]}`, version)
		if _, err := DecodeMetadata([]byte(document)); !errors.Is(err, ErrMetadataVersion) {
			t.Errorf("schema %d = %v, want ErrMetadataVersion", version, err)
		}
	}
}
