package remotesync_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/remotesync"
	"sshc/internal/storage"
)

const root = "/Users/tester/.ssh"

func digestOf(contents string) string { return remotesync.Digest([]byte(contents)) }

func manifestOf(files ...remotesync.Entry) remotesync.Manifest {
	return remotesync.Manifest{SchemaVersion: remotesync.SchemaVersion, Files: files}
}

func file(path, contents string, secret bool) remotesync.Entry {
	return remotesync.Entry{Path: path, SHA256: digestOf(contents), Mode: "0600", Secret: secret}
}

func TestPlanProducesOneTransaction(t *testing.T) {
	remote := manifestOf(
		file("config", "new config", false),
		file("connections/work/lon.conf", "new host", false),
	)
	contents := map[string][]byte{
		"config":                    []byte("new config"),
		"connections/work/lon.conf": []byte("new host"),
	}
	base := manifestOf(file("config", "old config", false))
	local := map[string]string{"config": digestOf("old config")}

	request, conflicts, err := remotesync.Plan(root, &base, local, remote, contents, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("Plan = %v", err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %#v", conflicts)
	}
	if request.Operation != "sync.pull" {
		t.Errorf("operation = %q", request.Operation)
	}
	if len(request.Changes) != 2 || len(request.Removals) != 0 {
		t.Fatalf("changes = %d, removals = %d", len(request.Changes), len(request.Removals))
	}
	if request.Changes[0].Path != filepath.Join(root, "config") {
		t.Errorf("path = %q", request.Changes[0].Path)
	}
}

func TestEveryChangeCarriesAPrecondition(t *testing.T) {
	// 事前条件がゼロの変更は、見ずに上書きすることになる。それは、このアプリケーション
	// が他のどこでもしていない唯一のことである。
	remote := manifestOf(file("config", "new", false))
	base := manifestOf(file("config", "old", false))
	local := map[string]string{"config": digestOf("old")}

	request, _, err := remotesync.Plan(root, &base, local, remote, map[string][]byte{"config": []byte("new")}, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	change := request.Changes[0]
	if !change.Precondition.Exists || change.Precondition.Digest != digestOf("old") {
		t.Errorf("precondition = %#v, want the digest currently on disk", change.Precondition)
	}
}

func TestANewFileGetsAPreconditionThatItDoesNotExist(t *testing.T) {
	remote := manifestOf(file("connections/new.conf", "x", false))
	base := manifestOf()

	request, _, err := remotesync.Plan(root, &base, map[string]string{}, remote,
		map[string][]byte{"connections/new.conf": []byte("x")}, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if request.Changes[0].Precondition.Exists {
		t.Errorf("precondition = %#v, want the zero value for a file that is not there", request.Changes[0].Precondition)
	}
}

// ローカルの秘密鍵を上書きする pull は、以前のものを残す。
//
// 以前はバックアップを求めなかった。そのコピーが平文の鍵になってしまうからだ。いま
// バックアップはマスターパスワードで暗号化されており、そしてここは、置き換えられた鍵
// こそが取り戻したいものになるまさにその場合である。別のマシンからのスナップショット
// が、ローカルの鍵の上に着地するのだから。
func TestPlanKeepsTheKeyAPullOverwrites(t *testing.T) {
	remote := manifestOf(
		file("config", "c", false),
		file("keys/work/id_ed25519", "private", true),
	)
	base := manifestOf()

	request, _, err := remotesync.Plan(root, &base, map[string]string{}, remote, map[string][]byte{
		"config":               []byte("c"),
		"keys/work/id_ed25519": []byte("private"),
	}, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range request.Changes {
		if change.SkipBackup {
			t.Errorf("%s asked for no backup, so the pull cannot be undone", change.Path)
		}
	}
}

func TestPlanDistinguishesDeletedThereFromCreatedHere(t *testing.T) {
	// 両者を区別できるのは、最後に同期したマニフェストだけである。ここを間違えると、
	// ユーザーがいま作ったばかりのファイルが削除される。
	base := manifestOf(file("connections/gone.conf", "old", false))
	local := map[string]string{
		"connections/gone.conf": digestOf("old"),  // base にあり remote にない → あちらで削除された
		"connections/new.conf":  digestOf("mine"), // どちらにもない → ここで作られた
	}
	remote := manifestOf(file("config", "c", false))

	request, conflicts, err := remotesync.Plan(root, &base, local, remote, map[string][]byte{"config": []byte("c")}, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %#v", conflicts)
	}
	if len(request.Removals) != 1 || request.Removals[0].Path != filepath.Join(root, "connections/gone.conf") {
		t.Fatalf("removals = %#v", request.Removals)
	}
	for _, removal := range request.Removals {
		if strings.Contains(removal.Path, "new.conf") {
			t.Error("a file created on this machine was scheduled for deletion")
		}
	}
}

func TestPlanReportsAConflictInsteadOfChoosing(t *testing.T) {
	// 両側で変わった。同じ Host ブロックを双方が変えた二つの ssh_config のマージに
	// 正解はなく、推測すれば、パーサが守るために存在するバイト保存の約束に反する
	// ことになる。
	base := manifestOf(file("config", "common ancestor", false))
	local := map[string]string{"config": digestOf("mine")}
	remote := manifestOf(file("config", "theirs", false))

	request, conflicts, err := remotesync.Plan(root, &base, local, remote, map[string][]byte{"config": []byte("theirs")}, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Changes) != 0 || len(request.Removals) != 0 {
		t.Fatalf("a conflicted file was written anyway: %#v", request)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %#v", conflicts)
	}
	got := conflicts[0]
	if got.BaseDigest != digestOf("common ancestor") ||
		got.LocalDigest != digestOf("mine") ||
		got.RemoteDigest != digestOf("theirs") {
		t.Errorf("conflict = %#v", got)
	}
}

func TestAConflictCarriesNoContents(t *testing.T) {
	// 秘密鍵のバイト列を運ぶ衝突レコードは、レスポンス本文の中にあるその鍵のコピー
	// になってしまう。
	base := manifestOf(file("keys/id_ed25519", "ancestor", true))
	local := map[string]string{"keys/id_ed25519": digestOf("local key material")}
	remote := manifestOf(file("keys/id_ed25519", "remote key material", true))

	_, conflicts, err := remotesync.Plan(root, &base, local, remote,
		map[string][]byte{"keys/id_ed25519": []byte("remote key material")}, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %#v", conflicts)
	}
	for _, field := range []string{conflicts[0].BaseDigest, conflicts[0].LocalDigest, conflicts[0].RemoteDigest} {
		if strings.Contains(field, "key material") {
			t.Error("a conflict carries file contents")
		}
	}
}

func TestDeletedThereButEditedHereIsAConflict(t *testing.T) {
	base := manifestOf(file("config", "ancestor", false))
	local := map[string]string{"config": digestOf("edited here")}
	remote := manifestOf()

	request, conflicts, err := remotesync.Plan(root, &base, local, remote, map[string][]byte{}, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Removals) != 0 {
		t.Fatal("a file edited on this machine was deleted because the remote dropped it")
	}
	if len(conflicts) != 1 || conflicts[0].RemoteDigest != "" {
		t.Fatalf("conflicts = %#v", conflicts)
	}
}

func TestAFirstSyncDeletesNothing(t *testing.T) {
	// base のマニフェストがなければ何も削除とは呼べないので、一度も同期していない
	// マシンが pull によってファイルを失うことはない。
	local := map[string]string{"connections/local-only.conf": digestOf("mine")}
	remote := manifestOf(file("config", "c", false))

	request, conflicts, err := remotesync.Plan(root, nil, local, remote, map[string][]byte{"config": []byte("c")}, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Removals) != 0 {
		t.Errorf("removals = %#v", request.Removals)
	}
	if len(conflicts) != 0 {
		t.Errorf("conflicts = %#v", conflicts)
	}
}

func TestAnIdenticalSnapshotIsNothingToApply(t *testing.T) {
	base := manifestOf(file("config", "same", false))
	local := map[string]string{"config": digestOf("same")}
	remote := manifestOf(file("config", "same", false))

	_, _, err := remotesync.Plan(root, &base, local, remote, map[string][]byte{"config": []byte("same")}, remotesync.ResolveNone)
	if !errors.Is(err, remotesync.ErrNothingToApply) {
		t.Fatalf("Plan = %v, want ErrNothingToApply", err)
	}
}

func TestPlanNeedsNothingStorageDoesNotAlreadyHave(t *testing.T) {
	// 現在の Change、Removal、Request で pull を表現できないなら、設計の方が誤って
	// いるのであって、ストレージ層を膨らませるのではなく計画へ戻るべきである。これは、
	// 生成される形がまさにその用語だけでできていることを
	// 表明する。
	base := manifestOf(file("config", "old", false), file("gone.conf", "g", false))
	local := map[string]string{"config": digestOf("old"), "gone.conf": digestOf("g")}
	remote := manifestOf(file("config", "new", false))

	request, _, err := remotesync.Plan(root, &base, local, remote, map[string][]byte{"config": []byte("new")}, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	var _ storage.Request = request
	if len(request.Moves) != 0 {
		t.Errorf("a pull produced moves, which it has no way to justify: %#v", request.Moves)
	}
}

// 別のマシンで消えたという理由で消えるファイルは、取り戻せなければならない。
// 押したユーザーは、その中身を見たことすら無いかもしれない。
func TestARemovalCarriedByAPullKeepsACopy(t *testing.T) {
	base := remotesync.Manifest{Files: []remotesync.Entry{
		{Path: "config", SHA256: "aaa"},
		{Path: "connections/old.conf", SHA256: "bbb"},
	}}
	remote := remotesync.Manifest{Files: []remotesync.Entry{{Path: "config", SHA256: "aaa"}}}
	local := map[string]string{"config": "aaa", "connections/old.conf": "bbb"}

	request, conflicts, err := remotesync.Plan("/root", &base, local, remote, map[string][]byte{}, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 || len(request.Removals) != 1 {
		t.Fatalf("plan = %+v, conflicts %+v", request, conflicts)
	}
	if !request.Removals[0].Backup {
		t.Fatal("the removal keeps no copy; History would have nothing to restore")
	}
}

// 寄せ先を選んだなら、その通りにする。2 台目が自分の設定を持ったまま
// 最初の pull をすると、いまは必ず衝突する。選ぶ道が無ければ、その 2 台目は
// 一度も同期を終えられない。
func TestChoosingTheRemoteSideWritesTheContestedFile(t *testing.T) {
	base := manifestOf(file("config", "base", false))
	remote := manifestOf(file("config", "theirs", false))
	local := map[string]string{"config": digestOf("mine")}

	request, conflicts, err := remotesync.Plan(root, &base, local, remote,
		map[string][]byte{"config": []byte("theirs")}, remotesync.ResolveRemote)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %+v, want none once a side was chosen", conflicts)
	}
	if len(request.Changes) != 1 || string(request.Changes[0].Contents) != "theirs" {
		t.Fatalf("changes = %+v", request.Changes)
	}
}

// こちらを残すなら、書かないだけである。次の push が、こちらを向こうへ運ぶ。
func TestChoosingThisMachineWritesNothingForTheContestedFile(t *testing.T) {
	base := manifestOf(file("config", "base", false))
	remote := manifestOf(file("config", "theirs", false))
	local := map[string]string{"config": digestOf("mine")}

	request, conflicts, err := remotesync.Plan(root, &base, local, remote,
		map[string][]byte{"config": []byte("theirs")}, remotesync.ResolveLocal)
	if err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		t.Fatal(err)
	}
	if len(conflicts) != 0 || len(request.Changes) != 0 {
		t.Fatalf("changes = %+v, conflicts = %+v", request.Changes, conflicts)
	}
}

// あちらで消え、こちらで編集された。こちらを残すなら消さない。
func TestChoosingThisMachineKeepsAFileTheOtherSideRemoved(t *testing.T) {
	base := manifestOf(file("config", "base", false), file("connections/x.conf", "base", false))
	remote := manifestOf(file("config", "base", false))
	local := map[string]string{"config": digestOf("base"), "connections/x.conf": digestOf("mine")}

	request, conflicts, err := remotesync.Plan(root, &base, local, remote,
		map[string][]byte{}, remotesync.ResolveLocal)
	if err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		t.Fatal(err)
	}
	if len(conflicts) != 0 || len(request.Removals) != 0 {
		t.Fatalf("removals = %+v, conflicts = %+v", request.Removals, conflicts)
	}
}
