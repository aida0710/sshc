package remotesync_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/objectstore"
	"sshc/internal/remotesync"
	"sshc/internal/storage"
)

// 二台のマシンと、本物のバケット。
//
// 密閉されたスイートは、仕様どおりに振る舞う偽物に対してこれを示すが、それは
// その偽物が仕様から書かれたことを示すにすぎない。こちらは、このリポジトリを
// 読んでいないサーバーに対して compare-and-swap が成り立つことを
// 示す。
//
// SSHC_TEST_S3_ENDPOINT がサーバーを指していなければスキップする。`make integration`
// はコンテナで SeaweedFS を起動し、それを設定する。
func integrationBucket(t *testing.T) (objectstore.Client, string) {
	t.Helper()
	endpoint := os.Getenv("SSHC_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("SSHC_TEST_S3_ENDPOINT is not set; run `make integration`")
	}
	bucket := os.Getenv("SSHC_TEST_S3_BUCKET")
	if bucket == "" {
		bucket = "sshc-test"
	}
	region := os.Getenv("SSHC_TEST_S3_REGION")
	if region == "" {
		region = "us-east-1"
	}
	return objectstore.Client{
		HTTP:     &http.Client{Timeout: 30 * time.Second},
		Endpoint: endpoint,
		Bucket:   bucket,
		Region:   region,
		Creds: objectstore.Credentials{
			AccessKeyID:     os.Getenv("SSHC_TEST_S3_KEY"),
			SecretAccessKey: os.Getenv("SSHC_TEST_S3_SECRET"),
		},
	}, endpoint
}

// realInstallation は、プロセス内の偽物ではなく本物のサーバーに向けた
// newInstallation。実行どうしが衝突しないよう、各テストは自前のオブジェクトキーを持つ。
func realInstallation(t *testing.T, files map[string]string) installation {
	return realInstallationAt(t, "", files)
}

// realInstallationAt は、同じバケット内の独立したprefixへ設置を向ける。
// 複数のfresh workspaceを一つのシナリオで動かすテストは、同じpathを渡す。
func realInstallationAt(t *testing.T, objectPath string, files map[string]string) installation {
	t.Helper()
	client, endpoint := integrationBucket(t)

	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range files {
		absolute := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	source := func() ([]string, error) {
		var paths []string
		err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, name)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			// Collect自身が鍵と背景を歩く。それ以外は、本番のInclude graphと
			// 同じく、その時点でworkspaceに存在するファイルを返す。
			if strings.HasPrefix(relative, "keys/") || strings.HasPrefix(relative, "sshc/backgrounds/") {
				return nil
			}
			paths = append(paths, relative)
			return nil
		})
		return paths, err
	}
	counter := 0
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := remotesync.NewService(workspace,
		manager, source,
		func() string { return time.Now().UTC().Format(time.RFC3339) },
		func() (string, error) { counter++; return "origin-integration", nil })
	service.OpenVault = func() ([]byte, error) { return nil, nil }
	service.SealVault = func(document []byte) ([]byte, error) { return document, nil }
	config := remotesync.Config{
		Endpoint: endpoint, Bucket: client.Bucket, Path: objectPath, Region: client.Region,
		Direction: remotesync.DirectionBoth,
	}
	service.Configure(config, client.Creds, &client)
	return installation{
		service: service, workspace: workspace, manager: manager, home: home,
		config: config, creds: client.Creds, client: &client,
	}
}

// connectionGate は、実際のS3 clientのtransportだけを切断する。復帰時は同じclient、
// 同じ資格情報、同じSeaweedFS endpointを通るので、再設定で状態を作り直してはいない。
type connectionGate struct {
	offline atomic.Bool
	base    http.RoundTripper
}

func (g *connectionGate) RoundTrip(request *http.Request) (*http.Response, error) {
	if g.offline.Load() {
		return nil, fmt.Errorf("integration network is disconnected")
	}
	return g.base.RoundTrip(request)
}

func writeRealWorkspace(t *testing.T, machine installation, name, contents string) {
	t.Helper()
	absolute := filepath.Join(machine.workspace.Root(), filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func uniqueRealPath(t *testing.T) string {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	return fmt.Sprintf("integration/%s/%d-%d", name, os.Getpid(), time.Now().UnixNano())
}

func TestAgainstARealBucketASnapshotTravelsBetweenTwoMachines(t *testing.T) {
	first := realInstallation(t, map[string]string{
		"config":               "Host bastion\r\n\tPort 2222   \n",
		"keys/work/id_ed25519": "-----BEGIN OPENSSH PRIVATE KEY-----\nnot really\n",
		"sshc/metadata.json":   `{"schemaVersion":2}`,
	})
	// オブジェクトキーは固定なので、以前に使われたことのあるバケットにはすでに
	// スナップショットがあり、条件付き書き込みは拒否される。それはクライアントが
	// 正しく振る舞っているということであり、以前はそのせいでこのテストがコンテナ
	// あたりちょうど一度しか通らなかった。二度目の実行は、一度目が残したオブジェクト
	// の上で失敗した。それでは誰も頻繁には走らせられない。
	//
	// 実際のマシンは、埋まっているバケットに出会えばまず pull する。これも同じことを
	// する。pull すれば ETag を知るので、push は本来あるべき If-Match になる。
	// ワークスペースがすでに一致している場合、Pull は完全な結果とともに
	// ErrNothingToApply を返す、それは API の形であって失敗ではなく、その結果を
	// apply することが ETag を記録する。
	result, err := first.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	switch {
	case err == nil, errors.Is(err, remotesync.ErrNothingToApply):
		if err := first.service.Apply(result); err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
			t.Fatalf("Apply of the snapshot already in the bucket = %v", err)
		}
	case errors.Is(err, remotesync.ErrNoSnapshot), errors.Is(err, objectstore.ErrNotFound):
		// 空のバケット、新しいコンテナが始まる状態である。そこから学ぶものは何もなく、
		// push はオブジェクトを作る書き込みになる。サービスが返すのは ErrNoSnapshot で、
		// その下のクライアントが返すのは ErrNotFound。どちらもここへ届き
		// うる。
	default:
		t.Fatalf("Pull before push = %v", err)
	}
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("Push = %v", err)
	}

	second := realInstallation(t, map[string]string{})
	result, err = second.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("Pull = %v", err)
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("conflicts = %#v", result.Conflicts)
	}
	if err := second.service.Apply(result); err != nil {
		t.Fatalf("Apply = %v", err)
	}

	// 本物のネットワーク往復を通しても、CRLF と末尾の空白を含めて 1 バイト
	// 違わない。
	if got := second.read(t, "config"); got != "Host bastion\r\n\tPort 2222   \n" {
		t.Errorf("config = %q", got)
	}
	if got := second.read(t, "keys/work/id_ed25519"); !strings.HasPrefix(got, "-----BEGIN") {
		t.Errorf("the private key did not arrive: %q", got)
	}
}

// 一つの実運用シナリオで、初回同期から競合解決、鍵交換、回線復帰までを通す。
// 個々の性質には密閉された単体テストもあるが、ここではすべてのread/writeが
// aws-sdk-go-v2の署名を経て実際のSeaweedFSへ届くことを検証する。
func TestAgainstARealBucketTwoFreshWorkspacesSurviveAFullSyncLifecycle(t *testing.T) {
	ctx := context.Background()
	remotePath := uniqueRealPath(t)
	first := realInstallationAt(t, remotePath, map[string]string{
		"config": "Host shared\n  HostName first.example\n",
	})
	second := realInstallationAt(t, remotePath, map[string]string{})

	if _, err := first.service.Push(ctx, syncPassphrase, ""); err != nil {
		t.Fatalf("first workspace initial Push = %v", err)
	}
	bucketView, err := first.service.BucketStatus(ctx)
	if err != nil {
		t.Fatalf("BucketStatus after initial Push = %v", err)
	}
	if bucketView.Live == nil || len(bucketView.History) != 1 || !bucketView.LocalIsLive {
		t.Fatalf("BucketStatus after initial Push = %#v", bucketView)
	}
	if _, err := second.service.Pull(ctx, "this is not the shared sync key", remotesync.ResolveNone); !errors.Is(err, remotesync.ErrWrongPassphrase) {
		t.Fatalf("second workspace Pull with a different key = %v, want ErrWrongPassphrase", err)
	}

	initial, err := second.service.Pull(ctx, syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("second workspace initial Pull = %v", err)
	}
	if err := second.service.Apply(initial); err != nil {
		t.Fatalf("second workspace initial Apply = %v", err)
	}
	if got := second.read(t, "config"); got != "Host shared\n  HostName first.example\n" {
		t.Fatalf("second workspace config after initial Pull = %q", got)
	}

	// 両方が同じbaseを知ったあと、同じファイルを別々に編集する。
	writeRealWorkspace(t, first, "config", "Host shared\n  HostName remote-change.example\n")
	if _, err := first.service.Push(ctx, syncPassphrase, ""); err != nil {
		t.Fatalf("first workspace Push after editing = %v", err)
	}
	history, err := first.service.History(ctx, syncPassphrase)
	if err != nil {
		t.Fatalf("History against the real bucket = %v", err)
	}
	var ancestorKey string
	for _, revision := range history.Revisions {
		if revision.Relation == remotesync.HistoryAncestor {
			ancestorKey = revision.Key
		}
	}
	if ancestorKey == "" {
		t.Fatalf("real bucket history has no ancestor: %#v", history.Revisions)
	}
	diff, err := first.service.DiffHistory(ctx, syncPassphrase, ancestorKey)
	if err != nil {
		t.Fatalf("DiffHistory against the real bucket = %v", err)
	}
	if len(diff.Modified) != 1 || diff.Modified[0] != "config" {
		t.Fatalf("real bucket history diff = %#v", diff)
	}
	writeRealWorkspace(t, second, "config", "Host shared\n  HostName local-choice.example\n")

	conflicted, err := second.service.Pull(ctx, syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("second workspace conflict preview = %v", err)
	}
	if len(conflicted.Conflicts) == 0 {
		t.Fatal("two divergent real workspaces produced no conflict")
	}
	if err := second.service.Apply(conflicted); !errors.Is(err, remotesync.ErrConflicts) {
		t.Fatalf("Apply with unresolved real conflict = %v, want ErrConflicts", err)
	}
	if got := second.read(t, "config"); got != "Host shared\n  HostName local-choice.example\n" {
		t.Fatalf("refused conflict Apply changed the local file: %q", got)
	}

	// 利用者がlocalを選んだ状態をbaseとして記録し、その選択を逆方向へpushする。
	localChoice, err := second.service.Pull(ctx, syncPassphrase, remotesync.ResolveLocal)
	if err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		t.Fatalf("resolve conflict in favour of local = %v", err)
	}
	if err := second.service.Apply(localChoice); err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		t.Fatalf("Apply local conflict resolution = %v", err)
	}
	if _, err := second.service.Push(ctx, syncPassphrase, ""); err != nil {
		t.Fatalf("second workspace Push of local conflict choice = %v", err)
	}

	chosen, err := first.service.Pull(ctx, syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("first workspace Pull of the chosen resolution = %v", err)
	}
	if err := first.service.Apply(chosen); err != nil {
		t.Fatalf("first workspace Apply of the chosen resolution = %v", err)
	}
	if got := first.read(t, "config"); got != "Host shared\n  HostName local-choice.example\n" {
		t.Fatalf("the conflict choice did not travel back: %q", got)
	}

	writeRealWorkspace(t, first, "config", "Host shared\n  HostName after-recovery.example\n")
	if _, err := first.service.Push(ctx, syncPassphrase, ""); err != nil {
		t.Fatalf("first workspace Push after conflict resolution = %v", err)
	}

	gate := &connectionGate{base: http.DefaultTransport}
	second.client.HTTP = &http.Client{Transport: gate, Timeout: 5 * time.Second}
	gate.offline.Store(true)
	if _, err := second.service.Pull(ctx, syncPassphrase, remotesync.ResolveNone); err == nil {
		t.Fatal("Pull succeeded while the integration transport was disconnected")
	}
	if got := second.read(t, "config"); got != "Host shared\n  HostName local-choice.example\n" {
		t.Fatalf("a disconnected Pull changed the workspace: %q", got)
	}

	gate.offline.Store(false)
	recovered, err := second.service.Pull(ctx, syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("Pull after network recovery = %v", err)
	}
	if err := second.service.Apply(recovered); err != nil {
		t.Fatalf("Apply after network recovery = %v", err)
	}
	if got := second.read(t, "config"); got != "Host shared\n  HostName after-recovery.example\n" {
		t.Fatalf("second workspace after recovery = %q", got)
	}
}

func TestAgainstARealBucketForcePushUsesTheConfirmedGeneration(t *testing.T) {
	ctx := context.Background()
	remotePath := uniqueRealPath(t)
	first := realInstallationAt(t, remotePath, map[string]string{"config": "Host first\n"})
	replacement := realInstallationAt(t, remotePath, map[string]string{"config": "Host replacement\n"})

	if _, err := first.service.Push(ctx, syncPassphrase, ""); err != nil {
		t.Fatalf("initial Push = %v", err)
	}
	stale, err := replacement.service.ForcePushConfirmation(ctx, remotesync.ForcePushTarget)
	if err != nil {
		t.Fatalf("first ForcePushConfirmation = %v", err)
	}
	writeRealWorkspace(t, first, "config", "Host newer-generation\n")
	if _, err := first.service.Push(ctx, syncPassphrase, ""); err != nil {
		t.Fatalf("competing Push = %v", err)
	}
	if _, err := replacement.service.ForcePush(ctx, syncPassphrase, stale.ETag, ""); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("ForcePush with stale confirmed generation = %v, want ErrRemoteMoved", err)
	}

	current, err := replacement.service.ForcePushConfirmation(ctx, remotesync.ForcePushTarget)
	if err != nil {
		t.Fatalf("second ForcePushConfirmation = %v", err)
	}
	if _, err := replacement.service.ForcePush(ctx, syncPassphrase, current.ETag, ""); err != nil {
		t.Fatalf("ForcePush with current confirmed generation = %v", err)
	}
	view, err := replacement.service.BucketStatus(ctx)
	if err != nil {
		t.Fatalf("BucketStatus after ForcePush = %v", err)
	}
	if view.Live == nil || !view.LocalIsLive || len(view.History) < 3 {
		t.Fatalf("BucketStatus after ForcePush = %#v", view)
	}
}

func TestAgainstARealBucketAStalePushIsRefused(t *testing.T) {
	// 「自動」という語が乗っている性質を、このリポジトリを読んでいないサーバーに
	// 対して検査する。
	first := realInstallation(t, map[string]string{"config": "first\n"})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil &&
		!errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("Push = %v", err)
	}

	behind := realInstallation(t, map[string]string{"config": "second\n"})
	if _, err := behind.service.Push(context.Background(), syncPassphrase, ""); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("a machine that has never synced pushed anyway: %v", err)
	}
}

func TestAgainstARealBucketTheObjectIsCiphertext(t *testing.T) {
	machine := realInstallation(t, map[string]string{
		"config":               "Host bastion\n\tHostName 203.0.113.10\n",
		"keys/work/id_ed25519": "PRIVATE KEY MATERIAL",
	})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil &&
		!errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("Push = %v", err)
	}

	client, _ := integrationBucket(t)
	object, err := client.Get(context.Background(), remotesync.ObjectName)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	for _, plaintext := range []string{"PRIVATE KEY MATERIAL", "bastion", "203.0.113.10", "manifest", "id_ed25519"} {
		if strings.Contains(string(object.Body), plaintext) {
			t.Errorf("the object in the bucket contains %q in clear", plaintext)
		}
	}
}

func TestAgainstARealBucketTheWrongPassphraseCannotRead(t *testing.T) {
	machine := realInstallation(t, map[string]string{"config": "Host bastion\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil &&
		!errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("Push = %v", err)
	}

	other := realInstallation(t, map[string]string{})
	if _, err := other.service.Pull(context.Background(), "a completely different passphrase", remotesync.ResolveNone); err == nil {
		t.Fatal("the snapshot opened with the wrong passphrase")
	}
}
