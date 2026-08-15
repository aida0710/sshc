package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestDecodeMetadataAcceptsAnAbsentFileAndRejectsAFutureSchema(t *testing.T) {
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
	// バージョン 2 の文書は端末の選択を文字列として持っている。**同じキーを
	// オブジェクトへ変えると json.Unmarshal は文書全体で失敗する**ので、埋め込み
	// ターミナルの設定は別のキーに置いてある。この検査がその判断を留めている。
	previous, err := DecodeMetadata([]byte(`{"schemaVersion":2,"terminal":"iterm2",` +
		`"customTerminal":{"application":"/Applications/Term.app","arguments":["-e"]},` +
		`"hosts":[{"identity":{"path":"config","alias":"bastion"},"favourite":true}]}`))
	if err != nil {
		t.Fatalf("a version 2 document = %v, want it to survive", err)
	}
	if len(previous.Hosts) != 1 || !previous.Hosts[0].Favourite {
		t.Fatalf("a version 2 document lost its hosts: %#v", previous)
	}
	if previous.EmbeddedTerminal != nil {
		t.Fatalf("the old terminal choice became embedded settings: %#v", previous.EmbeddedTerminal)
	}
}

// 範囲の外の上限は既定へ戻る。ここは読み取りであり、数字ひとつが色もタグも
// お気に入りも道連れに読めなくしてよい理由はない。
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

	// 範囲の中の値はそのまま通る。
	kept, err := DecodeMetadata([]byte(`{"schemaVersion":3,"embeddedTerminal":{"maxSessions":8,"scrollbackBytes":32768}}`))
	if err != nil {
		t.Fatal(err)
	}
	if limits := kept.TerminalLimits(); limits.MaxSessions != 8 || limits.Scrollback != 32768 {
		t.Fatalf("limits = %#v", limits)
	}
}

// 書き込み側は範囲の外を拒否する。読み取りが既定へ戻すのとは対称ではない
// ——書き込みはこのアプリケーション自身の操作だからである。
func TestEncodeMetadataRefusesLimitsOutsideTheirRange(t *testing.T) {
	// 「少なすぎるセッション数」は無い。下限は 1 で、その下は 0 ——0 は
	// 「書かれていない」であって範囲の外ではない。
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

	// **0 は「書かれていない」であり、範囲の外ではない。** この節には上限以外の
	// ものも入るので、上限に触れずに開始位置だけを書く文書が成立する。
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

	// グループ名はディレクトリパスなので、このアプリケーションが作成を
	// 拒否するような名前を挙げる文書もまた、信用してはならない。
	for _, name := range []string{"../escape", "", "sshc", "work/"} {
		withBadGroup := NewMetadata()
		withBadGroup.Groups = []GroupMetadata{{Name: name}}
		if err := ValidateMetadata(withBadGroup); !errors.Is(err, ErrMetadataGroup) {
			t.Errorf("group %q error = %v, want ErrMetadataGroup", name, err)
		}
	}

	// 大文字小文字だけが異なる 2 つのグループは、デフォルトの macOS ボリューム
	// では 1 個のディレクトリになるので、両方を宣言する文書は拒否される。
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
	// membership はディレクトリであり、note はコメントなので、ここにはどちら
	// のキーも無い。端末の選択もこのアプリケーションが持たなくなったので無い。
	// バイト列そのものを assert することが、こっそり戻るのを防ぐ。
	for _, absent := range []string{`"group"`, `"parent"`, `"terminal"`, `"customTerminal"`} {
		if strings.Contains(string(encoded), absent) {
			t.Errorf("encoded metadata still carries %s:\n%s", absent, encoded)
		}
	}
	if !strings.Contains(string(encoded), `"schemaVersion": 3`) {
		t.Errorf("encoded metadata is not version 3:\n%s", encoded)
	}
}

func TestDecodeMetadataDropsGroupMembershipFromAnOlderDocument(t *testing.T) {
	// バージョン 1 の文書はデコードされ、もはや意味を持たない 2 個のフィールドを
	// 単に失う。今やディレクトリが正であり、v1 文書のグループが名指す
	// レイアウトはディスク上にはまだ存在しない。
	const document = `{"schemaVersion":1,"groups":[{"name":"work","parent":"company"}],` +
		`"hosts":[{"identity":{"path":"config","alias":"bastion"},"group":"work","colour":"#f97316"}]}`

	metadata, err := DecodeMetadata([]byte(document))
	if err != nil {
		t.Fatalf("DecodeMetadata error = %v", err)
	}
	if len(metadata.Hosts) != 1 || metadata.Hosts[0].Colour != "#f97316" {
		t.Fatalf("hosts = %#v, want the presentation kept", metadata.Hosts)
	}
	encoded, err := EncodeMetadata(metadata)
	if err != nil {
		t.Fatalf("EncodeMetadata error = %v", err)
	}
	if strings.Contains(string(encoded), "company") {
		t.Errorf("the parent survived re-encoding:\n%s", encoded)
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
		Identity:  HostIdentity{Path: "config", Alias: "bastion"},
		Tags:      []string{"personal"},
		Colour:    "#22d3ee",
		Note:      "office jump host",
		Favourite: true,
		Order:     1,
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
	if err := os.WriteFile(change.Path, change.Contents, 0o600); err != nil {
		t.Fatal(err)
	}

	reloaded, reloadedPrecondition, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reloadedPrecondition.Exists || reloadedPrecondition.Digest != storage.Digest(change.Contents) {
		t.Fatalf("reloaded precondition = %#v", reloadedPrecondition)
	}
	if len(reloaded.Hosts) != 1 || reloaded.Hosts[0].Alias() != "bastion" || !reloaded.Hosts[0].Favourite {
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

// Hidden は Colour や Order と同様に見た目であり、このエンジンはそれを運ぶ
// だけで決して作用しない。他のグループを保持することが目的のグループは、
// connections ツリーにそれ自体として示すものが何もなく、これはその見出し
// をそこから取り除く——一方で Include 行、ディレクトリ、ssh が返す
// あらゆる答えはそのままにする。
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

// hidden でないグループはキーを一切書き込まないので、これが出荷された瞬間に、
// 手つかずのワークスペースの metadata がすべてのグループ分のフィールドを増やすことはない。
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
