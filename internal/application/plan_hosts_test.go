package application

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"sshc/internal/config"
	"sshc/internal/storage"
)

func hostEdits(t *testing.T, workspace *storage.Workspace, identities ...HostIdentity) []HostEdit {
	t.Helper()
	hosts := make([]HostEdit, 0, len(identities))
	for _, identity := range identities {
		hosts = append(hosts, HostEdit{Path: identity.Path, Alias: identity.Alias, Base: readFile(t, workspace, identity.Path)})
	}
	return hosts
}

func assertPreviewPaths(t *testing.T, preview SavePreview, paths ...string) {
	t.Helper()
	var actual []string
	for _, diff := range preview.Diffs {
		actual = append(actual, diff.Path)
	}
	slices.Sort(actual)
	slices.Sort(paths)
	if !slices.Equal(actual, paths) {
		t.Fatalf("preview paths = %v, want %v", actual, paths)
	}
}

func TestMoveHosts同じファイルの2件を移すと両方とMetadataが移る(t *testing.T) {
	service, workspace := newTestService(t)
	declareGroup(t, service, "work")
	base := readFile(t, workspace, "config") + "\n# second host\nHost second\n\tHostName second.example\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := loadMetadata(t, service)
	metadata.Hosts = []HostMetadata{
		{Identity: HostIdentity{Path: "config", Alias: "bastion"}, Note: "first"},
		{Identity: HostIdentity{Path: "config", Alias: "second"}, Tags: []string{"second"}},
	}
	seedMetadata(t, service, metadata)
	request := EditRequest{Kind: EditMoveHosts, DestinationGroup: "work", Hosts: hostEdits(t, workspace,
		HostIdentity{Path: "config", Alias: "bastion"}, HostIdentity{Path: "config", Alias: "second"})}
	preview, err := service.Preview(request)
	if err != nil {
		t.Fatal(err)
	}
	assertPreviewPaths(t, preview, "config", "connections/work/config.conf", service.displayPath(service.metadata.Path()))
	if readFile(t, workspace, "config") != base {
		t.Fatal("preview wrote the source")
	}
	saved, err := service.Save(request)
	if err != nil {
		t.Fatal(err)
	}
	if saved.TransactionID == "" || saved.Preview.Operation != "config.move_hosts" || !reflect.DeepEqual(saved.Preview, preview) {
		t.Fatalf("save = %#v", saved)
	}
	for _, alias := range []string{"bastion", "second"} {
		if strings.Contains(readFile(t, workspace, "config"), "Host "+alias) ||
			!strings.Contains(readFile(t, workspace, "connections/work/config.conf"), "Host "+alias) {
			t.Fatalf("%s was not moved", alias)
		}
	}
	stored := loadMetadata(t, service)
	for index, host := range stored.Hosts {
		want := metadata.Hosts[index]
		want.Identity.Path = "connections/work/config.conf"
		if !reflect.DeepEqual(host, want) {
			t.Fatalf("metadata = %#v, want %#v", host, want)
		}
	}
}

func TestHostEdits2件目のBaseが古いと全ファイルが変わらない(t *testing.T) {
	for _, kind := range []EditKind{EditMoveHosts, EditDeleteHosts} {
		t.Run(string(kind), func(t *testing.T) {
			service, workspace := newTestService(t)
			declareGroup(t, service, "work")
			hosts := hostEdits(t, workspace, HostIdentity{Path: "config", Alias: "bastion"},
				HostIdentity{Path: "conf.d/10-home.conf", Alias: "nas"})
			hosts[1].Base += "# stale\n"
			paths := []string{"config", "conf.d/10-home.conf", DefaultGroupsFile, service.displayPath(service.metadata.Path())}
			before := map[string]string{}
			for _, path := range paths {
				before[path] = readFile(t, workspace, path)
			}
			_, err := service.Save(EditRequest{Kind: kind, Hosts: hosts, DestinationGroup: "work"})
			var conflict *ConflictError
			if !errors.As(err, &conflict) || conflict.Report.Path != hosts[1].Path {
				t.Fatalf("save = %v, want conflict on second file", err)
			}
			for path, contents := range before {
				if readFile(t, workspace, path) != contents {
					t.Fatalf("rejected operation changed %s", path)
				}
			}
			if _, err := os.Stat(filepath.Join(workspace.Root(), "connections/work/config.conf")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("rejected operation created destination: %v", err)
			}
		})
	}
}

func TestMoveHosts別グループの接続先をまとめると未分類へも移せる(t *testing.T) {
	service, workspace := newTestService(t)
	declareGroup(t, service, "one", "two", "target")
	first := writeGroupFile(t, workspace, "one", "hosts.conf", "Host first\n\tHostName first.example\n")
	second := writeGroupFile(t, workspace, "two", "hosts.conf", "Host second\n\tHostName second.example\n")
	third := writeGroupFile(t, workspace, "two", "other.conf", "Host third\n\tHostName third.example\n")
	hosts := hostEdits(t, workspace, HostIdentity{Path: first, Alias: "first"},
		HostIdentity{Path: second, Alias: "second"}, HostIdentity{Path: third, Alias: "third"})
	if _, err := service.Save(EditRequest{Kind: EditMoveHosts, Hosts: hosts, DestinationGroup: "target"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{first, second, third} {
		if readFile(t, workspace, path) != "" {
			t.Fatalf("source %s is not empty", path)
		}
	}
	hosts = hostEdits(t, workspace, HostIdentity{Path: "connections/target/hosts.conf", Alias: "first"},
		HostIdentity{Path: "connections/target/hosts.conf", Alias: "second"},
		HostIdentity{Path: "connections/target/other.conf", Alias: "third"})
	if _, err := service.Save(EditRequest{Kind: EditMoveHosts, Hosts: hosts,
		DestinationPath: "config", DestinationBase: readFile(t, workspace, "config")}); err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		if !strings.Contains(readFile(t, workspace, "config"), "Host "+host.Alias) || readFile(t, workspace, host.Path) != "" {
			t.Fatalf("%s was not moved to the entry file", host.Alias)
		}
	}
}

func TestMoveHosts移動先の接続先を含めると飛ばされ全件なら変更なしになる(t *testing.T) {
	service, workspace := newTestService(t)
	declareGroup(t, service, "work")
	path := writeGroupFile(t, workspace, "work", "config.conf", "Host resident\n\tHostName resident.example\n")
	for _, residentFirst := range []bool{true, false} {
		hosts := hostEdits(t, workspace, HostIdentity{Path: path, Alias: "resident"}, HostIdentity{Path: "config", Alias: "bastion"})
		if !residentFirst {
			slices.Reverse(hosts)
		}
		if _, err := service.Preview(EditRequest{Kind: EditMoveHosts, Hosts: hosts, DestinationGroup: "work"}); err != nil {
			t.Fatal(err)
		}
	}
	hosts := hostEdits(t, workspace, HostIdentity{Path: path, Alias: "resident"}, HostIdentity{Path: "config", Alias: "bastion"})
	if _, err := service.Save(EditRequest{Kind: EditMoveHosts, Hosts: hosts, DestinationGroup: "work"}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(readFile(t, workspace, path), "Host resident") != 1 {
		t.Fatal("resident was duplicated")
	}
	hosts = hostEdits(t, workspace, HostIdentity{Path: path, Alias: "resident"}, HostIdentity{Path: path, Alias: "bastion"})
	request := EditRequest{Kind: EditMoveHosts, Hosts: hosts, DestinationGroup: "work"}
	preview, err := service.Preview(request)
	if err != nil || len(preview.Diffs) != 0 {
		t.Fatalf("no-op preview = %#v, %v", preview, err)
	}
	if _, err := service.Save(request); !errors.Is(err, storage.ErrNoChanges) {
		t.Fatalf("no-op save = %v", err)
	}
}

func TestDeleteHosts同一ファイル2件と別ファイル1件を消すと単体削除と同じコメントとMetadataになる(t *testing.T) {
	service, workspace := newTestService(t)
	const base = "# file banner\r\n\r\n# first comment\r\nHost first\r\n  HostName first.example\r\n\r\n# second comment\r\nHost second\r\n  HostName second.example\r\n# retained comment\r\nHost retained\r\n  HostName retained.example"
	path := writeGroupFile(t, workspace, "work", "hosts.conf", base)
	declareGroup(t, service, "work")
	metadata := loadMetadata(t, service)
	metadata.Hosts = []HostMetadata{{Identity: HostIdentity{Path: path, Alias: "first"}, Note: "keep orphan"}}
	seedMetadata(t, service, metadata)
	metadataBefore := readFile(t, workspace, service.displayPath(service.metadata.Path()))
	// HostForm が画面へ返す line/raw/commentLines を使い、removeHostBlock と
	// 同じ文字列の切り出しで期待値を作る。
	want := base
	for _, alias := range []string{"second", "first"} {
		detail, err := service.HostDetail(path, alias)
		if err != nil {
			t.Fatal(err)
		}
		form := detail.Form
		lines := strings.SplitAfter(want, "\n")
		offset := len(strings.Join(lines[:form.Entry.Line-1], ""))
		start := len(strings.Join(lines[:form.Entry.Line-1-form.CommentLines], ""))
		want = want[:start] + want[offset+len(form.Raw):]
	}
	request := EditRequest{Kind: EditDeleteHosts, Hosts: hostEdits(t, workspace,
		HostIdentity{Path: path, Alias: "first"}, HostIdentity{Path: path, Alias: "second"},
		HostIdentity{Path: "conf.d/10-home.conf", Alias: "nas"})}
	preview, err := service.Preview(request)
	if err != nil {
		t.Fatal(err)
	}
	assertPreviewPaths(t, preview, path, "conf.d/10-home.conf")
	if _, err := service.Save(request); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, workspace, path); got != want {
		t.Fatalf("deleted contents = %q, want %q", got, want)
	}
	if readFile(t, workspace, "conf.d/10-home.conf") != "" ||
		readFile(t, workspace, service.displayPath(service.metadata.Path())) != metadataBefore {
		t.Fatal("deletion retained a block or rewrote metadata")
	}
}

func TestHostEdits移動と削除をするとグループの共通設定のメンバーが変わる(t *testing.T) {
	service, workspace := newTestService(t)
	path := writeGroupFile(t, workspace, "old", "hosts.conf", "Host first\n  HostName first.example\nHost second\n  HostName second.example\n")
	groups := []GroupMetadata{
		{Name: "old", Settings: []Setting{{Keyword: "Port", Values: []string{"2222"}}}},
		{Name: "new", Settings: []Setting{{Keyword: "Port", Values: []string{"3333"}}}},
	}
	if _, err := service.Save(EditRequest{Kind: EditGroups, Groups: groups}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []EditKind{EditMoveHosts, EditDeleteHosts} {
		request := EditRequest{Kind: kind, DestinationGroup: "new", Hosts: hostEdits(t, workspace,
			HostIdentity{Path: path, Alias: "first"}, HostIdentity{Path: path, Alias: "second"})}
		preview, err := service.Preview(request)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(preview.Diffs, func(diff FileDiff) bool { return diff.Path == DefaultGroupsFile }) {
			t.Fatal("preview omitted group settings")
		}
		if _, err := service.Save(request); err != nil {
			t.Fatal(err)
		}
		compiled := readFile(t, workspace, DefaultGroupsFile)
		if kind == EditDeleteHosts {
			if strings.Contains(compiled, "Host first") || strings.Contains(compiled, "second") {
				t.Fatalf("deleted hosts remain in groups: %s", compiled)
			}
			continue
		}
		if !strings.Contains(compiled, "Host first second\n") || strings.Contains(compiled, "2222") || !strings.Contains(compiled, "3333") {
			t.Fatalf("moved hosts have wrong group settings: %s", compiled)
		}
		path = "connections/new/hosts.conf"
	}
}

func TestHostEdits同じファイルのBaseが違うか選択が不正だと拒否される(t *testing.T) {
	service, workspace := newTestService(t)
	host := hostEdits(t, workspace, HostIdentity{Path: "config", Alias: "bastion"})[0]
	for _, hosts := range [][]HostEdit{
		nil,
		make([]HostEdit, MaxHostEdits+1),
		{host, host},
		{host, {Path: "config", Alias: "other", Base: "different"}},
	} {
		if _, err := service.Save(EditRequest{Kind: EditDeleteHosts, Hosts: hosts}); !errors.Is(err, ErrInvalidHostSelection) {
			t.Fatalf("invalid selection returned %v", err)
		}
	}
}

func TestMoveHosts保存直前にファイルが変わると全体がConflictになる(t *testing.T) {
	service, workspace := newTestService(t)
	declareGroup(t, service, "work")
	request := EditRequest{Kind: EditMoveHosts, DestinationGroup: "work", Hosts: hostEdits(t, workspace,
		HostIdentity{Path: "config", Alias: "bastion"}, HostIdentity{Path: "conf.d/10-home.conf", Alias: "nas"})}
	prepared, err := service.plan(request)
	if err != nil {
		t.Fatal(err)
	}
	changed := request.Hosts[1].Base + "# external edit\n"
	if err := os.WriteFile(filepath.Join(workspace.Root(), request.Hosts[1].Path), []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = service.commitAtomicPlannedRequest(prepared, service.requestFor(prepared))
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.Report.Path != request.Hosts[1].Path {
		t.Fatalf("commit = %v", err)
	}
	if readFile(t, workspace, "config") != request.Hosts[0].Base || readFile(t, workspace, request.Hosts[1].Path) != changed {
		t.Fatal("conflict changed a source")
	}
}

func TestHostEdits書き込み途中に失敗すると先に書いたファイルも元に戻る(t *testing.T) {
	for _, kind := range []EditKind{EditMoveHosts, EditDeleteHosts} {
		t.Run(string(kind), func(t *testing.T) {
			fileSystem := &failRenameOnceFileSystem{FileSystem: storage.OSFileSystem{}}
			workspace, err := storage.NewWorkspace(fileSystem, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := workspace.EnsureDirectory(workspace.Root()); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte("Host first\n  HostName first.example\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			manager := storage.NewManager(workspace, time.Now, bytes.NewReader(bytes.Repeat([]byte{0x5a}, 4096)))
			service := NewService(workspace, manager)
			declareGroup(t, service, "old", "new")
			path := writeGroupFile(t, workspace, "old", "hosts.conf", "Host second\n  HostName second.example\n")
			hosts := hostEdits(t, workspace, HostIdentity{Path: "config", Alias: "first"}, HostIdentity{Path: path, Alias: "second"})
			fileSystem.path = filepath.Join(workspace.Root(), path)
			fileSystem.err = errors.New("injected second source write failure")
			if _, err := service.Save(EditRequest{Kind: kind, Hosts: hosts, DestinationGroup: "new"}); err == nil {
				t.Fatal("save unexpectedly succeeded")
			}
			if fileSystem.err != nil {
				t.Fatal("save failed before reaching the injected write failure")
			}
			for _, host := range hosts {
				if readFile(t, workspace, host.Path) != host.Base {
					t.Fatalf("rollback did not restore %s", host.Path)
				}
			}
		})
	}
}

func TestHostEdits上限の256件を選ぶと1回で処理できる(t *testing.T) {
	for _, kind := range []EditKind{EditMoveHosts, EditDeleteHosts} {
		t.Run(string(kind), func(t *testing.T) {
			service, workspace := newTestService(t)
			declareGroup(t, service, "work")
			var contents strings.Builder
			contents.WriteString(readFile(t, workspace, "config"))
			for index := range MaxHostEdits {
				fmt.Fprintf(&contents, "\nHost batch-%d\n  HostName node-%d.example\n", index, index)
			}
			base := contents.String()
			if err := os.WriteFile(filepath.Join(workspace.Root(), "config"), []byte(base), 0o600); err != nil {
				t.Fatal(err)
			}
			hosts := make([]HostEdit, MaxHostEdits)
			for index := range hosts {
				hosts[index] = HostEdit{Path: "config", Alias: fmt.Sprintf("batch-%d", index), Base: base}
			}
			if _, err := service.Save(EditRequest{Kind: kind, Hosts: hosts, DestinationGroup: "work"}); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(readFile(t, workspace, "config"), "Host batch-") {
				t.Fatal("some selected hosts remain")
			}
			if kind == EditMoveHosts && strings.Count(readFile(t, workspace, "connections/work/config.conf"), "Host batch-") != MaxHostEdits {
				t.Fatal("some selected hosts did not arrive")
			}
		})
	}
}

func TestExtractHostBlockを使う削除で生成領域の境界が残る(t *testing.T) {
	contents := "Host first\n  HostName first.example\n" + RegionStartMarker + "\nInclude groups.sshc.conf\n" + RegionEndMarker + "\nHost second\n  HostName second.example\n"
	for _, alias := range []string{"first", "second"} {
		file := config.Parse([]byte(contents))
		if _, err := ExtractHostBlock(file, alias); err != nil {
			t.Fatal(err)
		}
		if _, _, found, err := FindRegion(file); err != nil || !found {
			t.Fatalf("deletion damaged generated region: %s", file.Render())
		}
	}
}
