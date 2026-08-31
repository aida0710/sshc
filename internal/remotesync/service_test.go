package remotesync_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/objectstore"
	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/remotesync"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

const syncPassphrase = "correct horse battery staple"

// fakeBucket は、ETag を持つオブジェクトの集合と、この設計全体が乗っている条件付き
// 書き込みのルールである。スタブのクライアントではなく意図的に本物の HTTP サーバー
// にしてあるので、条件は、それが実際に通る場所で表明される。
//
// これが集合になったのは、push のたびにライブのオブジェクトの隣へ日付付きの
// コピーが残るようになってからだ。オブジェクトひとつの偽物では、二つの書き込みが
// ひとつに見えてしまい、どちらについても何も示せなかったはずである。
type fakeBucket struct {
	mu         sync.Mutex
	objects    map[string]storedObject
	generation int
	gets       int
	// refuseConditional は、すべての条件付き PUT を失敗させる。R2 がそれらに対応して
	// いないと判明した場合に、計画にあるフォールバックがどう働くかを試すための
	// ものである。
	refuseConditional        bool
	refuseLiveConditional    bool
	refuseNextConditional    bool
	failAfterConditional     bool
	failConditionalResponses int
	contentAddressedETag     bool
	listETagOverride         string
	historyUnconditional     int
	refuseGets               int
	listStarted              chan struct{}
	releaseList              chan struct{}
}

type storedObject struct {
	body []byte
	etag string
}

// key はバケット名を取り除く。これにより、偽物が保存するものは、このアプリケーション
// がオブジェクトと呼ぶものと一致する。
func (b *fakeBucket) key(path string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, "/"), "sshc/")
}

func (b *fakeBucket) keys() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	names := make([]string, 0, len(b.objects))
	for name := range b.objects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// replace は、別のマシンがそのオブジェクトを書いた状況の代わりを務める。
func (b *fakeBucket) replace(key, etag string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	stored := b.objects[key]
	stored.etag = etag
	b.objects[key] = stored
}

func (b *fakeBucket) removeObject(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.objects, key)
}

func (b *fakeBucket) putObject(key string, body []byte, etag string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.objects == nil {
		b.objects = map[string]storedObject{}
	}
	b.objects[key] = storedObject{body: body, etag: etag}
}

func (b *fakeBucket) refuseNextConditionalPut() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refuseNextConditional = true
}

// failAfterNextConditionalPut simulates a lost or 5xx response after the
// object store has durably applied the conditional write.
func (b *fakeBucket) failAfterNextConditionalPut() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failAfterConditional = true
}

func (b *fakeBucket) restoreConditionalResponses() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failConditionalResponses = 0
}

func (b *fakeBucket) refuseObjectGets(count int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refuseGets = count
}

func (b *fakeBucket) object(key string) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.objects[key].body
}

func (b *fakeBucket) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" {
			b.mu.Lock()
			started, release := b.listStarted, b.releaseList
			b.mu.Unlock()
			if started != nil {
				select {
				case started <- struct{}{}:
				default:
				}
			}
			if release != nil {
				<-release
			}
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.objects == nil {
			b.objects = map[string]storedObject{}
		}
		if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" {
			type content struct {
				Key          string `xml:"Key"`
				LastModified string `xml:"LastModified"`
				ETag         string `xml:"ETag"`
				Size         int    `xml:"Size"`
				StorageClass string `xml:"StorageClass"`
			}
			type result struct {
				XMLName     xml.Name  `xml:"ListBucketResult"`
				Name        string    `xml:"Name"`
				Prefix      string    `xml:"Prefix"`
				KeyCount    int       `xml:"KeyCount"`
				MaxKeys     int       `xml:"MaxKeys"`
				IsTruncated bool      `xml:"IsTruncated"`
				Contents    []content `xml:"Contents"`
			}
			prefix := r.URL.Query().Get("prefix")
			listed := result{Name: "sshc", Prefix: prefix, MaxKeys: 1000}
			for key, stored := range b.objects {
				if strings.HasPrefix(key, prefix) {
					etag := stored.etag
					if b.listETagOverride != "" {
						etag = b.listETagOverride
					}
					listed.Contents = append(listed.Contents, content{
						Key: key, LastModified: "2026-08-25T01:00:00Z", ETag: etag,
						Size: len(stored.body), StorageClass: "STANDARD",
					})
				}
			}
			sort.Slice(listed.Contents, func(i, j int) bool { return listed.Contents[i].Key < listed.Contents[j].Key })
			listed.KeyCount = len(listed.Contents)
			w.Header().Set("Content-Type", "application/xml")
			_ = xml.NewEncoder(w).Encode(listed)
			return
		}
		key := b.key(r.URL.Path)
		stored, present := b.objects[key]
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			if !present {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if r.Method == http.MethodGet && b.refuseGets > 0 {
				b.refuseGets--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("ETag", stored.etag)
			w.Header().Set("Content-Length", strconv.Itoa(len(stored.body)))
			w.Header().Set("Last-Modified", "Tue, 25 Aug 2026 01:00:00 GMT")
			if r.Method == http.MethodGet {
				b.gets++
				_, _ = w.Write(stored.body)
			}
		case http.MethodPut:
			ifMatch, ifNone := r.Header.Get("If-Match"), r.Header.Get("If-None-Match")
			if b.failConditionalResponses > 0 && (ifMatch != "" || ifNone != "") {
				b.failConditionalResponses--
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if b.refuseNextConditional && (ifMatch != "" || ifNone != "") {
				b.refuseNextConditional = false
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			if strings.Contains(key, remotesync.SnapshotPrefix) && ifNone != "*" {
				b.historyUnconditional++
			}
			if b.refuseLiveConditional && strings.HasSuffix(key, remotesync.ObjectName) && (ifMatch != "" || ifNone != "") {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			if b.refuseConditional && (ifMatch != "" || ifNone != "") {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			if ifNone == "*" && present {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			if ifMatch != "" && ifMatch != stored.etag {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			body := make([]byte, 0)
			buffer := make([]byte, 4096)
			for {
				n, err := r.Body.Read(buffer)
				body = append(body, buffer[:n]...)
				if err != nil {
					break
				}
			}
			b.generation++
			etag := `"` + string(rune('a'+b.generation)) + `"`
			if b.contentAddressedETag {
				etag = `"` + remotesync.Digest(body) + `"`
			}
			b.objects[key] = storedObject{body: body, etag: etag}
			if b.failAfterConditional && (ifMatch != "" || ifNone != "") {
				b.failAfterConditional = false
				// Keep every automatic SDK retry ambiguous as well. The test
				// explicitly restores the network before starting recovery.
				b.failConditionalResponses = 16
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("ETag", etag)
		case http.MethodDelete:
			delete(b.objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (b *fakeBucket) downloads() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.gets
}

func (b *fakeBucket) unconditionalHistoryPuts() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.historyUnconditional
}

type installation struct {
	service   *remotesync.Service
	workspace *storage.Workspace
	manager   *storage.Manager
	home      string

	// Configure が何で呼ばれたか。テストがフィクスチャを組み立て直さずに、
	// そのフィールドをひとつだけ変えられるようにするためである。
	config remotesync.Config
	creds  objectstore.Credentials
	client *objectstore.Client
}

func defaultIntegrationHooks() remotesync.IntegrationHooks {
	return remotesync.IntegrationHooks{
		OpenVault:          func() ([]byte, error) { return nil, nil },
		SealVault:          func(document []byte) ([]byte, error) { return document, nil },
		EmptyVaultDocument: func() ([]byte, error) { return []byte("empty-vault"), nil },
		VaultAdopted:       func() error { return nil },
		OpenSnippets:       func() ([]byte, error) { return nil, nil },
		SealSnippets:       func(document []byte) ([]byte, error) { return document, nil },
		SecretMutation:     func(run func() error) error { return run() },
		StableSnapshot:     func(run func() error) error { return run() },
	}
}

func (i *installation) replaceIntegrations(t *testing.T, configure func(*remotesync.IntegrationHooks)) {
	t.Helper()
	hooks := defaultIntegrationHooks()
	configure(&hooks)
	counter := 0
	service, err := remotesync.NewIntegratedService(i.workspace, i.manager,
		func() string { return "2026-08-05T00:00:00Z" },
		func() (string, error) { counter++; return "origin-" + string(rune('A'+counter)), nil },
		hooks)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Configure(i.config, i.creds, i.client); err != nil {
		t.Fatal(err)
	}
	i.service = service
}

// direct は、設定フォームと同じやり方で、このインストールを同じバケットの別の
// direction へ向け直す。
func (i installation) direct(direction remotesync.Direction) {
	config := i.config
	config.Direction = direction
	if err := i.service.Configure(config, i.creds, i.client); err != nil {
		panic(err)
	}
}

func newInstallation(t *testing.T, bucket *fakeBucket, files map[string]string) installation {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range files {
		absolute := filepath.Join(root, filepath.FromSlash(name))
		// sshc/ の下は private state であり、読み口が所有者と権限を先に確かめる。
		// 素の書き込みで置いたものは、中身を見られる前に断られる。
		if strings.HasPrefix(name, "sshc/") {
			acltest.WritePrivateFile(t, absolute, []byte(contents))
			continue
		}
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
	manager := storage.NewManager(workspace, time.Now, rand.Reader)

	counter := 0
	service, err := remotesync.NewIntegratedService(workspace, manager,
		func() string { return "2026-08-05T00:00:00Z" },
		func() (string, error) { counter++; return "origin-" + string(rune('A'+counter)), nil },
		defaultIntegrationHooks())
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewTLSServer(bucket.handler())
	t.Cleanup(server.Close)
	config := remotesync.Config{Endpoint: server.URL, Bucket: "sshc", Region: "auto", Direction: remotesync.DirectionBoth}
	credentials := objectstore.Credentials{AccessKeyID: "AKID", SecretAccessKey: "secret"}
	client := &objectstore.Client{
		HTTP: server.Client(), Endpoint: server.URL, Bucket: "sshc", Region: "auto",
		Creds: credentials,
	}
	if err := service.Configure(config, credentials, client); err != nil {
		t.Fatal(err)
	}
	return installation{
		service: service, workspace: workspace, manager: manager, home: home,
		config: config, creds: credentials, client: client,
	}
}

// uploads は、これまでに置かれたオブジェクトの数を数える。
func (b *fakeBucket) uploads() (int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	total := 0
	for _, stored := range b.objects {
		total += len(stored.body)
	}
	return len(b.objects), total
}

// remove は、このマシンでファイルを 1 つ消す。
func (i installation) remove(t *testing.T, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(i.home, ".ssh", filepath.FromSlash(name))); err != nil {
		t.Fatalf("remove %s: %v", name, err)
	}
}

func (i installation) read(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(i.home, ".ssh", filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}

func (i installation) write(t *testing.T, name, contents string) {
	t.Helper()
	absolute := filepath.Join(i.workspace.Root(), filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteSetupVerifiesRemoteBeforePersisting(t *testing.T) {
	bucket := &fakeBucket{}
	writer := newInstallation(t, bucket, map[string]string{"config": "Host writer\n"})
	empty, err := remotesync.InspectSetupTarget(context.Background(), writer.client, writer.config)
	if err != nil {
		t.Fatal(err)
	}
	if empty.State != remotesync.SetupTargetEmpty || empty.HistoryPresent {
		t.Fatalf("empty inspection = %+v", empty)
	}
	if _, err := writer.service.Push(context.Background(), syncPassphrase, "Initial snapshot"); err != nil {
		t.Fatal(err)
	}

	reader := newInstallation(t, bucket, map[string]string{"config": "Host reader\n"})
	existing, err := remotesync.InspectSetupTarget(context.Background(), reader.client, reader.config)
	if err != nil {
		t.Fatal(err)
	}
	if existing.State != remotesync.SetupTargetExisting || existing.ETag == "" || !existing.HistoryPresent {
		t.Fatalf("existing inspection = %+v", existing)
	}
	persisted := 0
	persist := func() error { persisted++; return nil }
	if err := reader.service.CompleteSetup(context.Background(), reader.config, reader.creds, reader.client,
		existing, "a wrong but sufficiently long synchronization key", persist); !errors.Is(err, remotesync.ErrWrongPassphrase) {
		t.Fatalf("CompleteSetup with wrong key = %v, want ErrWrongPassphrase", err)
	}
	if persisted != 0 {
		t.Fatalf("wrong key persisted settings %d times", persisted)
	}
	if err := reader.service.CompleteSetup(context.Background(), reader.config, reader.creds, reader.client,
		existing, syncPassphrase, persist); err != nil {
		t.Fatal(err)
	}
	if persisted != 1 {
		t.Fatalf("verified setup persisted settings %d times, want 1", persisted)
	}
}

func TestCompleteSetupRefusesOrphanedHistory(t *testing.T) {
	bucket := &fakeBucket{}
	writer := newInstallation(t, bucket, map[string]string{"config": "Host writer\n"})
	if _, err := writer.service.Push(context.Background(), syncPassphrase, "Initial snapshot"); err != nil {
		t.Fatal(err)
	}
	bucket.removeObject(remotesync.ObjectName)
	inspection, err := remotesync.InspectSetupTarget(context.Background(), writer.client, writer.config)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != remotesync.SetupTargetIncomplete {
		t.Fatalf("inspection state = %q, want incomplete", inspection.State)
	}
	persisted := false
	err = writer.service.CompleteSetup(context.Background(), writer.config, writer.creds, writer.client,
		inspection, syncPassphrase, func() error { persisted = true; return nil })
	if !errors.Is(err, remotesync.ErrSetupTargetIncomplete) {
		t.Fatalf("CompleteSetup = %v, want ErrSetupTargetIncomplete", err)
	}
	if persisted {
		t.Fatal("incomplete target persisted settings")
	}
}

func TestPersistedRestoreCannotOverwriteAnExplicitBinding(t *testing.T) {
	installation := newInstallation(t, &fakeBucket{}, map[string]string{"config": "Host current\n"})
	restoredConfig := installation.config
	restoredConfig.Bucket = "stale-restored-bucket"
	restoredCredentials := objectstore.Credentials{AccessKeyID: "STALE", SecretAccessKey: "stale"}
	restoredClient := *installation.client
	restoredClient.Bucket = restoredConfig.Bucket
	restoredClient.Creds = restoredCredentials

	applied, err := installation.service.ConfigureIfUnconfigured(restoredConfig, restoredCredentials, &restoredClient)
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("persisted restore overwrote an already explicit binding")
	}
	_, bucket, _, _ := installation.service.Target()
	if bucket != installation.config.Bucket {
		t.Fatalf("bucket = %q, want explicit %q", bucket, installation.config.Bucket)
	}
}

func TestCollectRunsInsideStableSnapshotHook(t *testing.T) {
	installation := newInstallation(t, &fakeBucket{}, map[string]string{"config": "Host current\n"})
	called := false
	installation.replaceIntegrations(t, func(hooks *remotesync.IntegrationHooks) {
		hooks.StableSnapshot = func(snapshot func() error) error {
			called = true
			return snapshot()
		}
	})
	if _, _, err := installation.service.Collect(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("Collect bypassed StableSnapshot")
	}
}

func TestASnapshotTravelsBetweenTwoMachines(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{
		"config":                  "Host bastion\r\n\tIdentityFile ~/.ssh/id_ed25519_server\n\tPort 2222   \n",
		"id_ed25519_server":       "-----BEGIN OPENSSH PRIVATE KEY-----\nroot key\n",
		"id_ed25519_server.pub":   "ssh-ed25519 public-key server\n",
		"keys/work/id_ed25519":    "-----BEGIN OPENSSH PRIVATE KEY-----\nnested key\n",
		"custom/nested/arbitrary": "not referenced by the Include graph\n",
		"sshc/metadata.json":      `{"schemaVersion":3}`,
	})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("Push = %v", err)
	}

	second := newInstallation(t, bucket, map[string]string{})
	result, err := second.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("Pull = %v", err)
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("conflicts = %#v", result.Conflicts)
	}
	if err := second.service.Apply(result); err != nil {
		t.Fatalf("Apply = %v", err)
	}

	// CRLF と末尾の空白も含め、1 バイト違わない。
	if got := second.read(t, "config"); got != "Host bastion\r\n\tIdentityFile ~/.ssh/id_ed25519_server\n\tPort 2222   \n" {
		t.Errorf("config = %q", got)
	}
	for path, want := range map[string]string{
		"id_ed25519_server":       "-----BEGIN OPENSSH PRIVATE KEY-----\nroot key\n",
		"id_ed25519_server.pub":   "ssh-ed25519 public-key server\n",
		"keys/work/id_ed25519":    "-----BEGIN OPENSSH PRIVATE KEY-----\nnested key\n",
		"custom/nested/arbitrary": "not referenced by the Include graph\n",
	} {
		if got := second.read(t, path); got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

func TestSnippetDocumentIsResealedForTheReceivingMachine(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{
		remotesync.SnippetsPath: "ciphertext-from-machine-a",
	})
	logical := []byte(`{"schemaVersion":1,"snippets":[{"command":"deploy --token=top-secret"}]}`)
	first.replaceIntegrations(t, func(hooks *remotesync.IntegrationHooks) {
		hooks.OpenSnippets = func() ([]byte, error) { return logical, nil }
	})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	second := newInstallation(t, bucket, map[string]string{})
	mutations := 0
	second.replaceIntegrations(t, func(hooks *remotesync.IntegrationHooks) {
		hooks.OpenSnippets = func() ([]byte, error) { return nil, nil }
		hooks.SealSnippets = func(document []byte) ([]byte, error) {
			if !bytes.Equal(document, logical) {
				t.Fatalf("SealSnippets received %q", document)
			}
			return []byte("ciphertext-from-machine-b"), nil
		}
		hooks.SecretMutation = func(apply func() error) error {
			mutations++
			return apply()
		}
	})
	result, err := second.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.service.Apply(result); err != nil {
		t.Fatal(err)
	}
	if got := second.read(t, remotesync.SnippetsPath); got != "ciphertext-from-machine-b" {
		t.Fatalf("local snippet file = %q", got)
	}
	if mutations != 1 {
		t.Fatalf("secret mutation boundary entered %d times", mutations)
	}
}

func TestSnippetApplyRejectsALocalEditMadeAfterPreview(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{remotesync.SnippetsPath: "ciphertext-a"})
	remoteDocument := []byte(`{"schemaVersion":1,"snippets":[{"command":"remote"}]}`)
	first.replaceIntegrations(t, func(hooks *remotesync.IntegrationHooks) {
		hooks.OpenSnippets = func() ([]byte, error) { return remoteDocument, nil }
	})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	second := newInstallation(t, bucket, map[string]string{})
	var localDocument []byte
	second.replaceIntegrations(t, func(hooks *remotesync.IntegrationHooks) {
		hooks.OpenSnippets = func() ([]byte, error) { return localDocument, nil }
		hooks.SealSnippets = func(document []byte) ([]byte, error) { return append([]byte("sealed:"), document...), nil }
	})
	result, err := second.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	localDocument = []byte(`{"schemaVersion":1,"snippets":[{"command":"local edit"}]}`)
	second.write(t, remotesync.SnippetsPath, "ciphertext-local-edit")
	if err := second.service.Apply(result); err == nil {
		t.Fatal("Apply overwrote a snippet edit made after preview")
	} else {
		var conflict *storage.ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("Apply = %v, want storage conflict", err)
		}
	}
	if got := second.read(t, remotesync.SnippetsPath); got != "ciphertext-local-edit" {
		t.Fatalf("local snippet file changed to %q", got)
	}
}

func TestCollectDoesNotFollowSymbolicLinks(t *testing.T) {
	installation := newInstallation(t, &fakeBucket{}, map[string]string{"config": "Host bastion\n"})
	outsideDirectory := t.TempDir()
	outside := filepath.Join(outsideDirectory, "outside-private-key")
	if err := os.WriteFile(outside, []byte("must not travel"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(installation.home, ".ssh", "linked-private-key")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic links are not available: %v", err)
	}
	linkedDirectory := filepath.Join(installation.home, ".ssh", "linked-directory")
	if err := os.Symlink(outsideDirectory, linkedDirectory); err != nil {
		t.Fatal(err)
	}
	manifest, contents, err := installation.service.Collect()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Files {
		if entry.Path == "linked-private-key" || strings.HasPrefix(entry.Path, "linked-directory/") {
			t.Fatal("a symbolic link was added to the snapshot")
		}
	}
	if _, ok := contents["linked-private-key"]; ok {
		t.Fatal("a symbolic link target was read into the snapshot")
	}
	if _, ok := contents["linked-directory/outside-private-key"]; ok {
		t.Fatal("a symbolic directory target was read into the snapshot")
	}
}

func TestCollectRefusesAPathWindowsCannotRepresent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot create these source paths")
	}
	for _, name := range []string{"NUL.conf", "connections/COM1/key", "trailing.", "trailing "} {
		t.Run(name, func(t *testing.T) {
			installation := newInstallation(t, &fakeBucket{}, map[string]string{"config": "Host portable\n", name: "x"})
			if _, _, err := installation.service.Collect(); !errors.Is(err, remotesync.ErrUnsafePath) {
				t.Fatalf("Collect = %v, want ErrUnsafePath", err)
			}
		})
	}
}

func TestCollectRefusesCaseInsensitivePathCollisionsOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("this fixture requires a case-sensitive Linux filesystem")
	}
	for _, files := range []map[string]string{
		{"config": "lower", "CONFIG": "upper"},
		{"Connections/a.conf": "upper directory", "connections/b.conf": "lower directory"},
	} {
		installation := newInstallation(t, &fakeBucket{}, files)
		if _, _, err := installation.service.Collect(); !errors.Is(err, remotesync.ErrUnsafePath) {
			t.Fatalf("Collect(%v) = %v, want ErrUnsafePath", files, err)
		}
	}
}

func TestCollectUsesDefaultAndSharedExclusionRules(t *testing.T) {
	installation := newInstallation(t, &fakeBucket{}, map[string]string{
		"config":                     "Host bastion\n",
		".DS_Store":                  "finder metadata",
		"connections/work/Thumbs.db": "thumbnail cache",
		"keys/work/private.bak":      "backup key",
		"known_hosts.tmp":            "temporary known hosts",
		"active.lock":                "editor lock",
	})

	view, err := installation.service.Exclusions()
	if err != nil {
		t.Fatal(err)
	}
	if !view.UsingDefaults || view.Document != remotesync.DefaultIgnoreDocument {
		t.Fatalf("default exclusion view = %+v", view)
	}
	manifest, _, err := installation.service.Collect()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Files {
		if entry.Path != "config" {
			t.Errorf("default snapshot unexpectedly contains %q", entry.Path)
		}
	}

	custom := "*.tmp\n!known_hosts.tmp\n"
	saved, err := installation.service.SaveExclusions(custom)
	if err != nil {
		t.Fatal(err)
	}
	if saved.UsingDefaults || saved.Document != custom {
		t.Fatalf("saved exclusion view = %+v", saved)
	}
	manifest, _, err = installation.service.Collect()
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, entry := range manifest.Files {
		paths[entry.Path] = true
	}
	if !paths[remotesync.IgnorePath] || !paths["known_hosts.tmp"] || paths["keys/work/private.bak"] == false {
		t.Fatalf("custom snapshot paths = %v", paths)
	}
	if paths[".DS_Store"] == false || paths["connections/work/Thumbs.db"] == false || paths["active.lock"] == false {
		t.Fatalf("saving custom rules did not replace defaults: %v", paths)
	}
}

func TestIgnoredFilesDoNotConsumeTheSnapshotEntryLimit(t *testing.T) {
	files := map[string]string{"config": "Host bastion\n"}
	for index := 0; index < remotesync.MaxEntries+1; index++ {
		files[fmt.Sprintf("cache/%04d.tmp", index)] = "temporary"
	}
	installation := newInstallation(t, &fakeBucket{}, files)

	manifest, _, err := installation.service.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].Path != "config" {
		t.Fatalf("snapshot files = %+v", manifest.Files)
	}
}

func TestPullKeepsAFileNewlyExcludedByTheRemoteRules(t *testing.T) {
	bucket := &fakeBucket{}
	sender := newInstallation(t, bucket, map[string]string{
		"config": "Host sender\n", remotesync.IgnorePath: "", "local.cache": "first",
	})
	if _, err := sender.service.Push(context.Background(), syncPassphrase, "Initial snapshot"); err != nil {
		t.Fatal(err)
	}

	receiver := newInstallation(t, bucket, map[string]string{})
	initial, err := receiver.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveRemote)
	if err != nil {
		t.Fatal(err)
	}
	if err := receiver.service.Apply(initial); err != nil {
		t.Fatal(err)
	}
	receiver.write(t, "local.cache", "receiver must keep this")

	if _, err := sender.service.SaveExclusions("*.cache\n"); err != nil {
		t.Fatal(err)
	}
	sender.write(t, "config", "Host sender\n  ServerAliveInterval 30\n")
	sender.write(t, "local.cache", "sender cache must not travel")
	if _, err := sender.service.Push(context.Background(), syncPassphrase, "Ignore local caches"); err != nil {
		t.Fatal(err)
	}

	next, err := receiver.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveRemote)
	if err != nil {
		t.Fatal(err)
	}
	for _, removal := range next.Removed {
		if strings.HasSuffix(removal, "local.cache") {
			t.Fatalf("excluded path scheduled for removal: %+v", next.Removed)
		}
	}
	if err := receiver.service.Apply(next); err != nil {
		t.Fatal(err)
	}
	if got := receiver.read(t, "local.cache"); got != "receiver must keep this" {
		t.Fatalf("excluded local file = %q", got)
	}
	if got := receiver.read(t, remotesync.IgnorePath); got != "*.cache\n" {
		t.Fatalf("shared ignore file = %q", got)
	}
}

func TestTheObjectInTheBucketIsCiphertext(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{
		"config":               "Host bastion\n\tHostName 203.0.113.10\n",
		"keys/work/id_ed25519": "PRIVATE KEY MATERIAL",
	})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	for _, plaintext := range []string{"PRIVATE KEY MATERIAL", "bastion", "203.0.113.10", "manifest", "id_ed25519"} {
		if strings.Contains(string(bucket.object(remotesync.ObjectName)), plaintext) {
			t.Errorf("the uploaded object contains %q in clear", plaintext)
		}
	}
}

func TestAPushCannotOverwriteAnotherMachine(t *testing.T) {
	// compare-and-swap。これがなければ「自動」は「最後に保存したマシンが暗黙に勝つ」
	// という意味になってしまう。
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "first\n"})
	second := newInstallation(t, bucket, map[string]string{"config": "second\n"})

	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("the first push = %v", err)
	}
	// 二台目のマシンは一度も同期していないので、その push は If-None-Match: * を運び、
	// オブジェクトを置き換えるのではなく拒否されなければならない。
	if _, err := second.service.Push(context.Background(), syncPassphrase, ""); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("the second push = %v, want ErrRemoteMoved", err)
	}

	// そして、一度同期したあとに遅れをとったマシンも拒否される。
	first.write(t, "config", "first changed\n")
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("a changed second push from the same machine = %v", err)
	}
	// 別のマシンがライブのオブジェクトを書いたので、こちらの ETag は古い。
	bucket.replace(remotesync.ObjectName, `"somebody else"`)
	first.write(t, "config", "first changed again\n")
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("a stale push = %v, want ErrRemoteMoved", err)
	}
}

func TestAnUnchangedManualPushCreatesNoHistory(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	before, _ := bucket.uploads()

	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Duplicate"); !errors.Is(err, remotesync.ErrNothingToPush) {
		t.Fatalf("unchanged Push = %v, want ErrNothingToPush", err)
	}
	if after, _ := bucket.uploads(); after != before {
		t.Fatalf("unchanged Push created objects: %d then %d", before, after)
	}
}

func TestOwnerReadExecuteModeCollectsAsExecutableWithoutAnotherPush(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix executable permission bits")
	}
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n", "helper.sh": "#!/bin/sh\n"})
	helper := filepath.Join(machine.home, ".ssh", "helper.sh")
	if err := os.Chmod(helper, 0o500); err != nil {
		t.Fatal(err)
	}

	manifest, _, err := machine.service.Collect()
	if err != nil {
		t.Fatal(err)
	}
	wantExecutable := false
	for _, entry := range manifest.Files {
		if entry.Path == "helper.sh" {
			wantExecutable = entry.Mode == "0700"
		}
	}
	if !wantExecutable {
		t.Fatalf("0500 helper was not collected as logical mode 0700: %#v", manifest.Files)
	}
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial executable"); err != nil {
		t.Fatal(err)
	}
	before, _ := bucket.uploads()
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Duplicate executable"); !errors.Is(err, remotesync.ErrNothingToPush) {
		t.Fatalf("unchanged 0500 Push = %v, want ErrNothingToPush", err)
	}
	if after, _ := bucket.uploads(); after != before {
		t.Fatalf("unchanged 0500 Push created objects: %d then %d", before, after)
	}
}

func TestARejectedLiveCASRemovesItsHistoryCandidate(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	before, _ := bucket.uploads()
	machine.write(t, "config", "Host changed\n")
	bucket.refuseLiveConditional = true

	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Change host"); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("Push = %v, want ErrRemoteMoved", err)
	}
	if after, _ := bucket.uploads(); after != before {
		t.Fatalf("rejected Push left a history candidate: %d objects became %d", before, after)
	}
}

func TestPullRefusesTheWrongPassphraseAndWritesNothing(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	second := newInstallation(t, bucket, map[string]string{})
	if _, err := second.service.Pull(context.Background(), "a different passphrase entirely", remotesync.ResolveNone); err == nil {
		t.Fatal("Pull succeeded with the wrong passphrase")
	}
	if _, err := os.Stat(filepath.Join(second.home, ".ssh", "config")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a refused pull wrote a file")
	}
}

func TestPullAndApplyRejectsARemoteChangeAfterTheDownload(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host remote\n"})
	if _, err := producer.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{"config": "Host local\n"})
	preview, err := consumer.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveRemote)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	envelope.OnDerive = func(step func()) {
		once.Do(func() {
			close(started)
			<-release
		})
		step()
	}
	t.Cleanup(func() { envelope.OnDerive = nil })
	done := make(chan error, 1)
	go func() {
		_, err := consumer.service.PullAndApply(context.Background(), syncPassphrase, remotesync.ResolveRemote, "",
			preview.ETag, preview.Manifest.Revision)
		done <- err
	}()
	<-started
	bucket.replace(remotesync.ObjectName, `"changed-after-get"`)
	close(release)
	if err := <-done; !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("PullAndApply = %v, want ErrRemoteMoved", err)
	}
	if got := consumer.read(t, "config"); got != "Host local\n" {
		t.Fatalf("stale preview changed config to %q", got)
	}
}

func TestHistoryApplyRejectsAMissingLiveObjectWithoutWritingFiles(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host remote\n"})
	if _, err := producer.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	history, err := producer.service.History(context.Background(), syncPassphrase)
	if err != nil || len(history.Revisions) == 0 {
		t.Fatalf("History = (%#v, %v)", history, err)
	}
	bucket.removeObject(remotesync.ObjectName)
	consumer := newInstallation(t, bucket, map[string]string{"config": "Host local\n"})
	result, err := consumer.service.PullHistory(context.Background(), syncPassphrase, history.Revisions[0].Key, remotesync.ResolveRemote)
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.service.Apply(result); !errors.Is(err, remotesync.ErrRemoteDeleted) {
		t.Fatalf("Apply = %v, want ErrRemoteDeleted", err)
	}
	if got := consumer.read(t, "config"); got != "Host local\n" {
		t.Fatalf("history apply changed config to %q", got)
	}
}

func TestPullOnAnEmptyBucketSaysSo(t *testing.T) {
	machine := newInstallation(t, &fakeBucket{}, map[string]string{})
	if _, err := machine.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone); !errors.Is(err, remotesync.ErrNoSnapshot) {
		t.Fatalf("Pull = %v, want ErrNoSnapshot", err)
	}
}

// pull が作るディレクトリは、それを埋めるファイルと同じトランザクションに乗る。
//
// 以前は Apply がジャーナルの外で EnsureDirectory を呼んでおり、その mkdir とコミット
// のあいだで落ちれば空のディレクトリが残った。コメント自身がそれを認めていた。バリ
// データが拒否したリクエストがディスクに何も残さないのは、ディレクトリがその同じ
// リクエストの一部になったときだけである。
func TestARefusedPullLeavesNoDirectoryBehind(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{
		"config":                    "Include connections/work/*.conf\n",
		"connections/work/lon.conf": "Host lon\n",
	})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	second := newInstallation(t, bucket, map[string]string{})
	second.manager.Validate = func(storage.Request) error { return errors.New("refused") }

	result, err := second.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.service.Apply(result); err == nil {
		t.Fatal("Apply は、拒否するバリデータに対して成功した")
	}
	if _, err := os.Stat(filepath.Join(second.home, ".ssh", "connections", "work")); !errors.Is(err, os.ErrNotExist) {
		t.Error("拒否された pull が空のディレクトリを残した")
	}
}

func TestApplyRefusesWhileAnythingIsInConflict(t *testing.T) {
	// 半分だけ適用すれば、どちらの側とも一致しないワークスペースになる。
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "theirs\n"})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	second := newInstallation(t, bucket, map[string]string{"config": "mine\n"})
	result, err := second.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Conflicts) == 0 {
		t.Fatal("two machines with different contents produced no conflict")
	}
	if err := second.service.Apply(result); !errors.Is(err, remotesync.ErrConflicts) {
		t.Fatalf("Apply = %v, want ErrConflicts", err)
	}
	if got := second.read(t, "config"); got != "mine\n" {
		t.Errorf("a refused apply changed the file: %q", got)
	}
}

func TestAnUnconfiguredServiceRefusesRatherThanPanicking(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	service := remotesync.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader),
		func() string { return "" }, func() (string, error) { return "o", nil })

	if service.Configured() {
		t.Error("an unconfigured service reports itself configured")
	}
	if _, err := service.Push(context.Background(), syncPassphrase, ""); !errors.Is(err, remotesync.ErrNotConfigured) {
		t.Errorf("Push = %v, want ErrNotConfigured", err)
	}
	if _, err := service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone); !errors.Is(err, remotesync.ErrNotConfigured) {
		t.Errorf("Pull = %v, want ErrNotConfigured", err)
	}
}

func TestTheStateFileRecordsWhatWasSynced(t *testing.T) {
	// あとで「別のマシンで削除された」と「ここで作られた」を区別できる唯一のものなので、
	// これを書かない push は、次の pull にそれを判別させられなくして
	// しまう。
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	recorded := machine.read(t, remotesync.StatePath)
	if !strings.Contains(recorded, `"etag"`) || !strings.Contains(recorded, `"base"`) {
		t.Errorf("sync state = %s", recorded)
	}
	if strings.Contains(recorded, syncPassphrase) || strings.Contains(recorded, "secret") {
		t.Error("the sync state carries a credential")
	}
}

func TestPushRefusesAWorkspaceWithPendingRecoveryBeforeUploading(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fault-injection wrapper does not expose the Windows private-state reader")
	}
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "interrupted.conf")
	if err := os.WriteFile(target, []byte("Host interrupted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected removal failure")
	fileSystem := pendingRecoveryFileSystem{
		FileSystem: storage.OSFileSystem{}, target: target, failure: failure,
	}
	workspace, err := storage.NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	hooks := defaultIntegrationHooks()
	hooks.StableSnapshot = manager.WithSnapshot
	service, err := remotesync.NewIntegratedService(workspace, manager,
		func() string { return "2026-08-31T00:00:00Z" },
		func() (string, error) { return "origin-pending", nil }, hooks)
	if err != nil {
		t.Fatal(err)
	}
	bucket := &fakeBucket{}
	server := httptest.NewTLSServer(bucket.handler())
	t.Cleanup(server.Close)
	config := remotesync.Config{Endpoint: server.URL, Bucket: "sshc", Region: "auto", Direction: remotesync.DirectionBoth}
	credentials := objectstore.Credentials{AccessKeyID: "AKID", SecretAccessKey: "secret"}
	client := &objectstore.Client{
		HTTP: server.Client(), Endpoint: server.URL, Bucket: "sshc", Region: "auto", Creds: credentials,
	}
	if err := service.Configure(config, credentials, client); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Commit(storage.Request{
		Operation: "interrupted.remove",
		Removals: []storage.Removal{{
			Path: target, Precondition: storage.Precondition{Exists: true, Digest: storage.Digest([]byte("Host interrupted\n"))},
		}},
	}); !errors.Is(err, failure) {
		t.Fatalf("fixture Commit = %v, want injected failure", err)
	}

	if _, err := service.Push(context.Background(), syncPassphrase, "Blocked push"); !errors.Is(err, storage.ErrPendingTransaction) || !errors.Is(err, remotesync.ErrWorkspaceBusy) {
		t.Fatalf("Push = %v, want pending transaction/workspace busy", err)
	}
	if count, bytes := bucket.uploads(); count != 0 || bytes != 0 {
		t.Fatalf("blocked Push uploaded %d objects / %d bytes", count, bytes)
	}
}

type pendingRecoveryFileSystem struct {
	storage.FileSystem
	target  string
	failure error
}

func (fileSystem pendingRecoveryFileSystem) Remove(path string) error {
	if path == fileSystem.target {
		return fileSystem.failure
	}
	return fileSystem.FileSystem.Remove(path)
}

func TestPushReportsMeasuredBytesAndPersistsTheSuccessfulOperation(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{
		"config":             "Host bastion\n",
		"connections/x.conf": "Host x\n",
	})

	result, err := machine.service.Push(context.Background(), syncPassphrase, "")
	if err != nil {
		t.Fatal(err)
	}
	wantSource := int64(len("Host bastion\n") + len("Host x\n"))
	wantSnapshot := int64(len(bucket.object(remotesync.ObjectName)))
	if result.Summary.CreatedAt != "2026-08-05T00:00:00Z" || result.Summary.FileCount != 2 ||
		result.Summary.SourceBytes != wantSource || result.Summary.SnapshotBytes != wantSnapshot {
		t.Fatalf("summary = %#v, want source=%d snapshot=%d", result.Summary, wantSource, wantSnapshot)
	}
	if result.ObjectCount != 2 || result.UploadedBytes != wantSnapshot*2 ||
		result.CompletedAt != "2026-08-05T00:00:00Z" {
		t.Fatalf("push result = %#v", result)
	}
	view := machine.service.SyncState()
	if !view.Synced || view.LastOperation == nil || view.LastOperation.Kind != remotesync.OperationPush {
		t.Fatalf("sync state = %#v", view)
	}
	if view.LastOperation.UploadedBytes != result.UploadedBytes ||
		view.LastOperation.Summary.SourceBytes != wantSource {
		t.Fatalf("persisted operation = %#v", view.LastOperation)
	}
}

func TestPullReportsDownloadedAndExpandedBytesWithoutPersistingPreview(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host producer\n"})
	if _, err := producer.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	consumer := newInstallation(t, bucket, map[string]string{})
	result, err := consumer.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	wantSnapshot := int64(len(bucket.object(remotesync.ObjectName)))
	if result.DownloadedBytes != wantSnapshot || result.Summary.SnapshotBytes != wantSnapshot ||
		result.Summary.SourceBytes != int64(len("Host producer\n")) || result.Summary.FileCount != 1 ||
		result.Summary.CreatedAt != "2026-08-05T00:00:00Z" {
		t.Fatalf("pull result = %#v", result)
	}
	if view := consumer.service.SyncState(); view.Synced || view.LastOperation != nil {
		t.Fatalf("preview persisted state: %#v", view)
	}

	if err := consumer.service.Apply(result); err != nil {
		t.Fatal(err)
	}
	view := consumer.service.SyncState()
	if !view.Synced || view.LastOperation == nil || view.LastOperation.Kind != remotesync.OperationApply {
		t.Fatalf("applied state = %#v", view)
	}
	if view.LastOperation.DownloadedBytes != wantSnapshot || view.LastOperation.Written != 1 ||
		view.LastOperation.Removed != 0 {
		t.Fatalf("apply operation = %#v", view.LastOperation)
	}
	history, err := consumer.manager.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Operation != "sync.pull" {
		t.Fatalf("pull and sync state were not committed together: %#v", history)
	}
	wantState := filepath.Join(consumer.workspace.Root(), filepath.FromSlash(remotesync.StatePath))
	wantConfig := filepath.Join(consumer.workspace.Root(), "config")
	foundConfig, foundState := false, false
	for _, path := range history[0].Paths {
		foundConfig = foundConfig || path == wantConfig
		foundState = foundState || path == wantState
	}
	if !foundConfig || !foundState {
		t.Fatalf("atomic pull paths = %#v", history[0].Paths)
	}
}

func TestPullCommitsSyncStateAsTheTerminalWrite(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{
		"config": "Host first\n", "gone.conf": "Host gone\n",
	})
	if _, err := producer.service.Push(context.Background(), syncPassphrase, "Initial"); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{})
	initial, err := consumer.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.service.Apply(initial); err != nil {
		t.Fatal(err)
	}

	producer.write(t, "config", "Host second\n")
	producer.remove(t, "gone.conf")
	if _, err := producer.service.Push(context.Background(), syncPassphrase, "Remove obsolete config"); err != nil {
		t.Fatal(err)
	}
	next, err := consumer.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveRemote)
	if err != nil {
		t.Fatal(err)
	}
	var committed storage.Request
	consumer.manager.Validate = func(request storage.Request) error {
		if request.Operation == "sync.pull" {
			committed = request
		}
		return nil
	}
	if err := consumer.service.Apply(next); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(consumer.workspace.Root(), filepath.FromSlash(remotesync.StatePath))
	if len(committed.Removals) != 1 || len(committed.FinalChanges) != 1 || committed.FinalChanges[0].Path != statePath {
		t.Fatalf("pull transaction = removals %#v, final changes %#v", committed.Removals, committed.FinalChanges)
	}
	for _, change := range committed.Changes {
		if change.Path == statePath {
			t.Fatalf("sync state remained an ordinary write: %#v", committed.Changes)
		}
	}
}

func TestFailedPushReportsItsCompletedUploadAndPreservesPriorSuccess(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "one\n"})
	first, err := machine.service.Push(context.Background(), syncPassphrase, "")
	if err != nil {
		t.Fatal(err)
	}
	bucket.replace(remotesync.ObjectName, `"moved"`)
	machine.write(t, "config", "two\n")

	partial, err := machine.service.Push(context.Background(), syncPassphrase, "")
	if !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("Push = %v, want ErrRemoteMoved", err)
	}
	if partial.ObjectCount != 1 || partial.UploadedBytes != partial.Summary.SnapshotBytes {
		t.Fatalf("partial result = %#v", partial)
	}
	view := machine.service.SyncState()
	if view.LastOperation == nil || view.LastOperation.CompletedAt != first.CompletedAt ||
		view.LastOperation.UploadedBytes != first.UploadedBytes {
		t.Fatalf("failed push replaced prior success: %#v", view.LastOperation)
	}
}

func TestStateWithoutCurrentSchemaIsIgnored(t *testing.T) {
	machine := newInstallation(t, &fakeBucket{}, map[string]string{})
	old := `{
		"etag":"etag-old",
		"key":"workspace.tar.gz.enc",
		"base":{"schemaVersion":4,"createdAt":"2026-07-01T00:00:00Z","origin":"old","files":[]},
		"origin":"machine-old"
	}`
	if err := machine.workspace.EnsureDirectory(machine.workspace.StateDir()); err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, filepath.Join(machine.workspace.Root(), filepath.FromSlash(remotesync.StatePath)), []byte(old))

	view := machine.service.SyncState()
	if view.Synced || view.LastOperation != nil {
		t.Fatalf("old state = %#v, want an unsynced view", view)
	}
}

func TestASecondPushFromTheSameMachineSucceeds(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(machine.home, ".ssh", "config"), []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("the second push = %v", err)
	}

	other := newInstallation(t, bucket, map[string]string{})
	result, err := other.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.service.Apply(result); err != nil {
		t.Fatal(err)
	}
	if got := other.read(t, "config"); got != "two\n" {
		t.Errorf("config = %q, want the second push", got)
	}
}

func TestPullAcceptsReadOnlyLocalFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows maps os.Chmod without owner-write to the DOS read-only
		// attribute. It cannot represent the Unix owner-only 0400/0500 modes
		// whose normalization and precondition behavior this test exercises.
		t.Skip("Windows does not represent Unix owner-only read-only modes")
	}
	for _, test := range []struct {
		name       string
		localMode  os.FileMode
		remoteMode os.FileMode
	}{
		{name: "non executable", localMode: 0o400, remoteMode: 0o600},
		{name: "executable", localMode: 0o500, remoteMode: 0o700},
	} {
		t.Run(test.name, func(t *testing.T) {
			bucket := &fakeBucket{}
			writer := newInstallation(t, bucket, map[string]string{"config": "old\n"})
			writerPath := filepath.Join(writer.workspace.Root(), "config")
			if err := os.Chmod(writerPath, test.remoteMode); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.service.Push(context.Background(), syncPassphrase, "initial"); err != nil {
				t.Fatal(err)
			}

			reader := newInstallation(t, bucket, map[string]string{})
			initial, err := reader.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.service.Apply(initial); err != nil {
				t.Fatal(err)
			}
			readerPath := filepath.Join(reader.workspace.Root(), "config")
			if err := os.Chmod(readerPath, test.localMode); err != nil {
				t.Fatal(err)
			}

			writer.write(t, "config", "new\n")
			if _, err := writer.service.Push(context.Background(), syncPassphrase, "content update"); err != nil {
				t.Fatal(err)
			}
			update, err := reader.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
			if err != nil {
				t.Fatalf("Pull with local mode %04o = %v", test.localMode, err)
			}
			if len(update.Conflicts) != 0 {
				t.Fatalf("Pull with local mode %04o reported false conflicts: %+v", test.localMode, update.Conflicts)
			}
			if err := reader.service.Apply(update); err != nil {
				t.Fatalf("Apply with local mode %04o = %v", test.localMode, err)
			}
			if got := reader.read(t, "config"); got != "new\n" {
				t.Fatalf("config = %q, want updated contents", got)
			}
			info, err := os.Stat(readerPath)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm() & 0o700; got != test.remoteMode {
				t.Fatalf("applied mode = %04o, want %04o", got, test.remoteMode)
			}
		})
	}
}

func TestAReceiveOnlyMachineWillNotPush(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	machine.direct(remotesync.DirectionPull)

	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); !errors.Is(err, remotesync.ErrPushRefused) {
		t.Fatalf("Push = %v, want ErrPushRefused", err)
	}
	// リクエストのあとではなく前に拒否される。バケットには何も届いていない。
	if keys := bucket.keys(); len(keys) != 0 {
		t.Errorf("the bucket holds %v, pushed by a receive-only machine", keys)
	}
}

func TestASendOnlyMachineWillNotApply(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "from the other machine\n"})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	second := newInstallation(t, bucket, map[string]string{"config": "what is on this disk\n"})
	second.direct(remotesync.DirectionPush)

	// プレビューは引き続き動く。適用してはいけないマシンでも、どれだけ遅れているかを
	// 知ることは許される。見ることまで拒めば、この設定は防護ではなく目隠しに
	// なってしまう。
	result, err := second.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("Pull = %v, want a preview", err)
	}
	if len(result.Conflicts) == 0 {
		t.Fatal("the preview reported no conflict on a file both machines changed")
	}

	if err := second.service.Apply(result); !errors.Is(err, remotesync.ErrApplyRefused) {
		t.Fatalf("Apply = %v, want ErrApplyRefused", err)
	}
	if got := second.read(t, "config"); got != "what is on this disk\n" {
		t.Errorf("config = %q; a send-only machine had its file overwritten", got)
	}
}

func TestDirectionAcceptsOnlyTheThreeCurrentValues(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if got := machine.service.Direction(); got != remotesync.DirectionBoth {
		t.Errorf("Direction() = %q, want both", got)
	}
	for _, name := range []string{"both", "push", "pull"} {
		if _, ok := remotesync.ParseDirection(name); !ok {
			t.Errorf("ParseDirection(%q) refused a name it should accept", name)
		}
	}
	for _, name := range []string{"", "sideways"} {
		if _, ok := remotesync.ParseDirection(name); ok {
			t.Errorf("ParseDirection(%q) accepted a name that is not a direction", name)
		}
	}
}

// vault は移動する。バケットへの鍵は移動しない。
//
// 暗号化された設定は、まさにこのバケットのアクセスキーを保持している。したがって、
// それを運ぶスナップショットは、スナップショットをひとつ入手した者が以後のすべてを
// 取得できることを意味する。Collect は ~/.ssh 全体を歩くため、除外契約に漏れが
// 生じたら気づくのがこのテストである。
func TestASnapshotCarriesTheVaultAndNotTheKeyToItsOwnBucket(t *testing.T) {
	installation := newInstallation(t, &fakeBucket{}, map[string]string{
		"config":                       "Include sshc/sync-settings\nHost bastion\n",
		"sshc/secrets":                 "sealed vault bytes",
		"sshc/snippets.json":           "device-master-key ciphertext",
		"sshc/sync-settings":           "sealed access key",
		"SSHC/SYNC-SETTINGS":           "case-variant sealed access key",
		"sshc/cli":                     `{"url":"http://127.0.0.1:1","secret":"s"}`,
		"sshc/.cli.mutation.lock":      "runtime handoff lock state",
		"sshc/journal/entry":           "machine-local journal",
		"sshc/backups/entry":           "machine-local backup",
		"sshc/history/entry":           "machine-local history",
		"sshc/trash/entry":             "machine-local trash",
		"sshc/recent-connections.json": `{"schemaVersion":1,"entries":[]}`,
		"sshc/workspaces.json":         `{"schemaVersion":1}`,
		"sshc/mutation.lock":           "runtime lock state",
		"connections/.sshc-staged":     "transaction temporary file",
	})

	installation.replaceIntegrations(t, func(hooks *remotesync.IntegrationHooks) {
		hooks.OpenVault = func() ([]byte, error) { return []byte(`{"schemaVersion":4}`), nil }
		hooks.OpenSnippets = func() ([]byte, error) {
			return []byte(`{"schemaVersion":1,"snippets":[{"command":"top-secret"}]}`), nil
		}
	})
	exchanged, contents, err := installation.service.Collect()
	if err != nil {
		t.Fatalf("Collect = %v", err)
	}
	packed := map[string]bool{}
	for _, entry := range exchanged.Files {
		packed[entry.Path] = true
	}
	if packed["sshc/secrets"] {
		t.Error("the sealed vault travelled even though its contents do")
	}
	if !packed[remotesync.TravelPath] {
		t.Errorf("the vault contents did not travel: %v", packed)
	}
	if string(contents[remotesync.TravelPath]) != `{"schemaVersion":4}` {
		t.Errorf("what travelled was not the contents: %q", contents[remotesync.TravelPath])
	}
	if string(contents[remotesync.SnippetsPath]) != `{"schemaVersion":1,"snippets":[{"command":"top-secret"}]}` {
		t.Errorf("snippet snapshot used local ciphertext instead of the logical document: %q", contents[remotesync.SnippetsPath])
	}
	if bytes.Contains(contents[remotesync.SnippetsPath], []byte("device-master-key ciphertext")) {
		t.Fatal("device-specific snippet ciphertext travelled")
	}
	for _, excluded := range []string{
		secret.SettingsPath,
		"SSHC/SYNC-SETTINGS",
		"sshc/cli",
		"sshc/.cli.mutation.lock",
		"sshc/journal/entry",
		"sshc/backups/entry",
		"sshc/history/entry",
		"sshc/trash/entry",
		"sshc/recent-connections.json",
		"sshc/workspaces.json",
		"sshc/mutation.lock",
		"connections/.sshc-staged",
	} {
		if packed[excluded] {
			t.Errorf("the snapshot carries %s: %v", excluded, packed)
		}
		if _, ok := contents[excluded]; ok {
			t.Errorf("%s is in the archive even though the manifest omits it", excluded)
		}
	}
}

func TestIntegratedServiceRefusesAMissingVaultCodec(t *testing.T) {
	installation := newInstallation(t, &fakeBucket{}, map[string]string{"config": "Host bastion\n"})
	hooks := defaultIntegrationHooks()
	hooks.OpenVault = nil
	if _, err := remotesync.NewIntegratedService(installation.workspace, installation.manager,
		func() string { return "2026-08-05T00:00:00Z" },
		func() (string, error) { return "origin-test", nil }, hooks); err == nil {
		t.Fatal("NewIntegratedService accepted a missing vault codec")
	}
}

// バケットの登録は、まずそのバケットに尋ねる。
//
// 試されたことのない設定は、設定済みに見えて何時間もあとの最初の push で失敗する
// 設定であり、そのときユーザーは、タイプミスをした画面からとうに離れている。まだ
// スナップショットの入っていないバケットは機能しているバケットだ。404 は、正しくて
// 空の設定が返す結果である。
func TestCheckAcceptsAnEmptyBucketAndRefusesABadKey(t *testing.T) {
	bucket := &fakeBucket{}
	installation := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})

	if err := installation.service.Check(context.Background()); err != nil {
		t.Errorf("Check against an empty bucket = %v, want nil", err)
	}
	if _, err := installation.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("Push = %v", err)
	}
	if err := installation.service.Check(context.Background()); err != nil {
		t.Errorf("Check against a bucket holding a snapshot = %v, want nil", err)
	}
}

func TestCheckRefusesABucketThatWillNotAnswer(t *testing.T) {
	bucket := &fakeBucket{}
	installation := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	// 存在しないホストへ向けられたクライアント。エンドポイントの打ち間違いは、ここから
	// はこう見える。
	if err := installation.service.Configure(
		remotesync.Config{Endpoint: "https://127.0.0.1:1", Bucket: "sshc", Region: "auto", Direction: remotesync.DirectionBoth},
		installation.creds,
		&objectstore.Client{Endpoint: "https://127.0.0.1:1", Bucket: "sshc", Region: "auto", Creds: installation.creds},
	); err != nil {
		t.Fatal(err)
	}
	if err := installation.service.Check(context.Background()); err == nil {
		t.Error("Check against an unreachable endpoint returned nil")
	}
}

func TestCheckSaysWhenNothingIsConfigured(t *testing.T) {
	service := remotesync.NewService(nil, nil, nil, nil)
	if err := service.Check(context.Background()); !errors.Is(err, remotesync.ErrNotConfigured) {
		t.Errorf("Check with no configuration = %v, want ErrNotConfigured", err)
	}
}

// エンドポイントが正規化される前に保存された設定も正しく表示される。サービスは、
// 与えられたものがどこから来たかを信用せず、自分で切り詰めるからだ。
func TestAStoredTrailingSlashIsTrimmedWhenItIsConfigured(t *testing.T) {
	installation := newInstallation(t, &fakeBucket{}, map[string]string{"config": "Host bastion\n"})
	if err := installation.service.Configure(
		remotesync.Config{Endpoint: "https://s3.example.invalid/", Bucket: "b", Region: "auto", Direction: remotesync.DirectionBoth},
		installation.creds, installation.client); err != nil {
		t.Fatal(err)
	}

	if endpoint, _, _, _ := installation.service.Target(); endpoint != "https://s3.example.invalid" {
		t.Errorf("endpoint = %q, want the trailing slash gone", endpoint)
	}
}

// オブジェクトはユーザーが言った場所へ行き、それが何であるかにちなんで名付けられる。
func TestTheKeysFollowTheConfiguredPath(t *testing.T) {
	for _, test := range []struct{ path, object, dated string }{
		// 既定はバケットのルート。バケットはたいていすでにこのアプリケーションにちなんで
		// 名付けられているので、その中で同じ名前を繰り返すフォルダは、何もない階層を
		// ひとつ増やすだけである。
		{"", "workspace.tar.gz.enc", "snapshots/2026-08-05-000000.tar.gz.enc"},
		{"sshc", "sshc/workspace.tar.gz.enc", "sshc/snapshots/2026-08-05-000000.tar.gz.enc"},
		// どう綴られていても、意味するところはひとつである。
		{"/laptops/", "laptops/workspace.tar.gz.enc", "laptops/snapshots/2026-08-05-000000.tar.gz.enc"},
	} {
		config := remotesync.Config{Endpoint: "https://example.invalid", Bucket: "b", Path: test.path, Direction: remotesync.DirectionBoth}
		if got := remotesync.ObjectKeyFor(config); got != test.object {
			t.Errorf("ObjectKeyFor(%q) = %q, want %q", test.path, got, test.object)
		}
		got, err := remotesync.SnapshotKeyFor(config, "2026-08-05T00:00:00Z")
		if err != nil {
			t.Fatalf("SnapshotKeyFor(%q) = %v", test.path, err)
		}
		if got != test.dated {
			t.Errorf("SnapshotKeyFor(%q) = %q, want %q", test.path, got, test.dated)
		}
	}
}

func TestTwoPushesAtTheSameTimestampKeepTwoHistoryObjects(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}
	machine.write(t, "config", "Host bastion\n  Port 2222\n")
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	var history int
	for _, key := range bucket.keys() {
		if strings.HasPrefix(key, remotesync.SnapshotPrefix) {
			history++
		}
	}
	if history != 2 {
		t.Fatalf("history objects = %d, want 2; keys = %v", history, bucket.keys())
	}
}

func TestChangingToAnotherBucketDoesNotReuseThePreviousGeneration(t *testing.T) {
	firstBucket := &fakeBucket{}
	machine := newInstallation(t, firstBucket, map[string]string{"config": "Host bastion\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial workspace"); err != nil {
		t.Fatal(err)
	}

	secondBucket := &fakeBucket{}
	server := httptest.NewTLSServer(secondBucket.handler())
	t.Cleanup(server.Close)
	config := machine.config
	config.Endpoint = server.URL
	client := &objectstore.Client{
		HTTP: server.Client(), Endpoint: server.URL, Bucket: config.Bucket, Region: config.Region,
		Creds: machine.creds,
	}
	if err := machine.service.Configure(config, machine.creds, client); err != nil {
		t.Fatal(err)
	}
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Start the new bucket"); err != nil {
		t.Fatalf("Push to another bucket = %v", err)
	}
	if got := len(secondBucket.keys()); got != 2 {
		t.Fatalf("new bucket objects = %d, want history and live; keys = %v", got, secondBucket.keys())
	}
	if got := len(firstBucket.keys()); got != 2 {
		t.Fatalf("old bucket was changed: %v", firstBucket.keys())
	}
}

func TestBucketStatusReadsLiveAndDatedHistoryFromTheRemote(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}
	machine.write(t, "config", "Host two\n")
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	view, err := machine.service.BucketStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if view.Live == nil || view.Live.Key != remotesync.ObjectName || view.Live.Size == 0 {
		t.Fatalf("live = %#v", view.Live)
	}
	if len(view.History) != 2 {
		t.Fatalf("history = %#v", view.History)
	}
	if !view.LocalIsLive || view.CheckedAt == "" {
		t.Fatalf("bucket view = %#v", view)
	}

	bucket.replace(remotesync.ObjectName, `"someone-else"`)
	view, err = machine.service.BucketStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if view.LocalIsLive {
		t.Fatal("a changed remote generation was reported as locally acknowledged")
	}
}

func TestBucketStatusDoesNotHoldTheSyncOperationLockDuringS3Listing(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "initial"); err != nil {
		t.Fatal(err)
	}
	bucket.mu.Lock()
	bucket.listStarted = make(chan struct{}, 1)
	bucket.releaseList = make(chan struct{})
	started, release := bucket.listStarted, bucket.releaseList
	bucket.mu.Unlock()
	statusDone := make(chan error, 1)
	go func() {
		_, err := machine.service.BucketStatus(context.Background())
		statusDone <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("BucketStatus did not reach the remote listing")
	}
	machine.write(t, "config", "Host two\n")
	pushDone := make(chan error, 1)
	go func() {
		_, err := machine.service.Push(context.Background(), syncPassphrase, "second")
		pushDone <- err
	}()
	select {
	case err := <-pushDone:
		if err != nil {
			t.Fatalf("Push while BucketStatus was listing = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BucketStatus held operationMu across the S3 listing")
	}
	close(release)
	if err := <-statusDone; !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("BucketStatus after concurrent Push = %v, want ErrRemoteMoved", err)
	}
}

func TestHistoryDiffRestoreAndBranch(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{
		"config": "Host one\n", "connections/old.conf": "Host old\n",
	})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}
	first, err := machine.service.History(context.Background(), syncPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Revisions) != 1 || first.Revisions[0].Relation != remotesync.HistoryHead {
		t.Fatalf("first history = %#v", first)
	}
	rootRevision, rootKey := first.HeadRevision, first.Revisions[0].Key

	machine.write(t, "config", "Host two\n")
	machine.write(t, "connections/new.conf", "Host new\n")
	machine.remove(t, "connections/old.conf")
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	history, err := machine.service.History(context.Background(), syncPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if history.HeadRevision == rootRevision || len(history.Revisions) != 2 {
		t.Fatalf("second history = %#v", history)
	}
	var rootSeen, headSeen bool
	for _, revision := range history.Revisions {
		switch revision.Revision {
		case rootRevision:
			rootSeen = revision.Relation == remotesync.HistoryAncestor
		case history.HeadRevision:
			headSeen = revision.Relation == remotesync.HistoryHead && revision.ParentRevision == rootRevision
		}
	}
	if !rootSeen || !headSeen {
		t.Fatalf("relations = %#v", history.Revisions)
	}

	diff, err := machine.service.DiffHistory(context.Background(), syncPassphrase, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(diff.Added, ",") != "connections/new.conf" ||
		strings.Join(diff.Modified, ",") != "config" ||
		strings.Join(diff.Removed, ",") != "connections/old.conf" {
		t.Fatalf("diff = %#v", diff)
	}
	if _, err := machine.service.DiffHistory(context.Background(), syncPassphrase, "../workspace.tar.gz.enc"); !errors.Is(err, remotesync.ErrHistoryTarget) {
		t.Fatalf("invalid history key = %v", err)
	}

	restored, err := machine.service.PullHistory(context.Background(), syncPassphrase, rootKey, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if err := machine.service.Apply(restored); err != nil {
		t.Fatal(err)
	}
	if got := machine.read(t, "config"); got != "Host one\n" {
		t.Fatalf("restored config = %q", got)
	}
	machine.write(t, "config", "Host branch\n")
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	branched, err := machine.service.History(context.Background(), syncPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	var branchSeen, restoredAncestor, newHead bool
	for _, revision := range branched.Revisions {
		switch revision.Relation {
		case remotesync.HistoryBranch:
			branchSeen = true
		case remotesync.HistoryAncestor:
			if revision.Revision == rootRevision {
				restoredAncestor = true
			}
		case remotesync.HistoryHead:
			newHead = revision.ParentRevision == rootRevision
		}
	}
	if !branchSeen || !restoredAncestor || !newHead {
		t.Fatalf("branched history = %#v", branched.Revisions)
	}
}

func TestPushDraftAndEncryptedHistoryMessages(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	draft, err := machine.service.PushDraft()
	if err != nil {
		t.Fatal(err)
	}
	if draft.Added == 0 || draft.Message == "" {
		t.Fatalf("initial draft = %#v", draft)
	}
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial SSH setup"); err != nil {
		t.Fatal(err)
	}
	machine.write(t, "config", "Host two\n")
	draft, err = machine.service.PushDraft()
	if err != nil {
		t.Fatal(err)
	}
	if draft.Modified != 1 || draft.Message != "Update config" {
		t.Fatalf("updated draft = %#v", draft)
	}
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}
	history, err := machine.service.History(context.Background(), syncPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	messages := map[string]bool{}
	for _, revision := range history.Revisions {
		messages[revision.Message] = true
	}
	if !messages["Initial SSH setup"] || !messages["Update config"] {
		t.Fatalf("history messages = %#v", history.Revisions)
	}
}

func TestHistorySkipsAnUnreadableImmutableObject(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	bucket.putObject(remotesync.SnapshotPrefix+"unreadable.tar.gz.enc", []byte("not an envelope"), `"legacy"`)

	history, err := machine.service.History(context.Background(), syncPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if history.Skipped != 1 || len(history.Revisions) != 1 {
		t.Fatalf("history = %#v, want one revision and one skipped object", history)
	}
}

func TestHistoryRejectsAnObjectChangedAfterListing(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	bucket.listETagOverride = `"stale-list-entry"`
	if _, err := machine.service.History(context.Background(), syncPassphrase); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("History = %v, want ErrRemoteMoved", err)
	}
}

func TestHistorySerializesRemoteDerivationAndDiscardsItsStaleGraph(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	replacementStarted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHistory := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseHistory)
	var calls int
	var callsMu sync.Mutex
	envelope.OnDerive = func(step func()) {
		callsMu.Lock()
		calls++
		call := calls
		callsMu.Unlock()
		if call == 1 {
			close(started)
			<-release
		} else if call == 2 {
			close(replacementStarted)
		}
		step()
	}
	t.Cleanup(func() { envelope.OnDerive = nil })

	historyDone := make(chan error, 1)
	go func() {
		_, err := machine.service.History(context.Background(), syncPassphrase)
		historyDone <- err
	}()
	<-started
	rotationDone := make(chan error, 1)
	go func() {
		rotationDone <- machine.service.ReplaceKey(
			context.Background(), syncPassphrase, "a different strong shared synchronization key", true, func() error { return nil },
		)
	}()
	releaseHistory()
	select {
	case <-replacementStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("ReplaceKey did not continue after the bounded remote derivation finished")
	}
	if err := <-rotationDone; err != nil {
		t.Fatalf("ReplaceKey while History derives = %v", err)
	}
	if err := <-historyDone; !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("History after key replacement = %v, want ErrRemoteMoved", err)
	}
}

func TestDiffHistorySerializesRemoteDerivationAndDiscardsItsStaleDiff(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	history, err := machine.service.History(context.Background(), syncPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	key := history.Revisions[0].Key
	started := make(chan struct{})
	replacementStarted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseDiff := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseDiff)
	var calls int
	var callsMu sync.Mutex
	envelope.OnDerive = func(step func()) {
		callsMu.Lock()
		calls++
		call := calls
		callsMu.Unlock()
		if call == 1 {
			close(started)
			<-release
		} else if call == 2 {
			close(replacementStarted)
		}
		step()
	}
	t.Cleanup(func() { envelope.OnDerive = nil })

	diffDone := make(chan error, 1)
	go func() {
		_, err := machine.service.DiffHistory(context.Background(), syncPassphrase, key)
		diffDone <- err
	}()
	<-started
	rotationDone := make(chan error, 1)
	go func() {
		rotationDone <- machine.service.ReplaceKey(
			context.Background(), syncPassphrase, "a different strong shared synchronization key", true, func() error { return nil },
		)
	}()
	releaseDiff()
	select {
	case <-replacementStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("ReplaceKey did not continue after the bounded remote derivation finished")
	}
	if err := <-rotationDone; err != nil {
		t.Fatalf("ReplaceKey while DiffHistory derives = %v", err)
	}
	if err := <-diffDone; !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("DiffHistory after key replacement = %v, want ErrRemoteMoved", err)
	}
}

func TestHistoryDiscardsAResultFromAReconfiguredBinding(t *testing.T) {
	firstBucket := &fakeBucket{}
	machine := newInstallation(t, firstBucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	envelope.OnDerive = func(step func()) {
		once.Do(func() {
			close(started)
			<-release
		})
		step()
	}
	t.Cleanup(func() { envelope.OnDerive = nil })

	done := make(chan error, 1)
	go func() {
		_, err := machine.service.History(context.Background(), syncPassphrase)
		done <- err
	}()
	<-started
	other := newInstallation(t, &fakeBucket{}, map[string]string{})
	if err := machine.service.Configure(other.config, other.creds, other.client); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("History after Configure = %v, want ErrRemoteMoved", err)
	}
}

func TestConcurrentHistoryCallsShareTheDedicatedAdmissionLock(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls int
	var callsMu sync.Mutex
	envelope.OnDerive = func(step func()) {
		callsMu.Lock()
		calls++
		call := calls
		callsMu.Unlock()
		if call == 1 {
			close(started)
			<-release
		}
		step()
	}
	t.Cleanup(func() { envelope.OnDerive = nil })

	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := machine.service.History(context.Background(), syncPassphrase)
		firstDone <- err
	}()
	<-started
	downloads := bucket.downloads()
	go func() {
		_, err := machine.service.History(context.Background(), syncPassphrase)
		secondDone <- err
	}()
	time.Sleep(100 * time.Millisecond)
	if got := bucket.downloads(); got != downloads {
		t.Fatalf("second History started remote work concurrently: downloads %d then %d", downloads, got)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first History = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second History = %v", err)
	}
}

func TestReplaceKeyCommitsLocallyOnlyAfterRemoteCAS(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	const next = "a different strong shared synchronization key"
	committed := false
	if err := machine.service.ReplaceKey(context.Background(), syncPassphrase, next, true, func() error {
		committed = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("local key commit was not called")
	}
	reader := newInstallation(t, bucket, map[string]string{})
	if _, err := reader.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone); !errors.Is(err, remotesync.ErrWrongPassphrase) {
		t.Fatalf("old key Pull = %v, want ErrWrongPassphrase", err)
	}
	if _, err := reader.service.Pull(context.Background(), next, remotesync.ResolveNone); err != nil {
		t.Fatalf("new key Pull = %v", err)
	}
}

func TestReplaceKeyRequiresExplicitHistoryLossConfirmation(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	before := bucket.object(remotesync.ObjectName)
	committed := false
	err := machine.service.ReplaceKey(context.Background(), syncPassphrase,
		"a different strong shared synchronization key", false, func() error {
			committed = true
			return nil
		})
	if !errors.Is(err, remotesync.ErrHistoryKeyLossConfirmation) {
		t.Fatalf("ReplaceKey = %v, want ErrHistoryKeyLossConfirmation", err)
	}
	if committed || !bytes.Equal(before, bucket.object(remotesync.ObjectName)) {
		t.Fatal("unconfirmed replacement changed local or remote key state")
	}
}

func TestReplaceKeyRestoresRemoteWhenLocalCommitFails(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	commitErr := errors.New("local commit failed")
	err := machine.service.ReplaceKey(context.Background(), syncPassphrase, "a different strong shared synchronization key", true, func() error {
		return commitErr
	})
	if !errors.Is(err, commitErr) {
		t.Fatalf("ReplaceKey = %v, want local commit error", err)
	}
	reader := newInstallation(t, bucket, map[string]string{})
	if _, err := reader.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone); err != nil {
		t.Fatalf("rolled back object is not readable with old key: %v", err)
	}
	if err := machine.service.ReplaceKey(context.Background(), syncPassphrase, "a different strong shared synchronization key", true, func() error {
		return nil
	}); err != nil {
		t.Fatalf("state did not follow the rollback ETag: %v", err)
	}
}

func TestReplaceKeyRollbackSurvivesTheRequestCancellation(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	commitErr := errors.New("local commit failed")
	err := machine.service.ReplaceKey(ctx, syncPassphrase, "a different strong shared synchronization key", true, func() error {
		cancel()
		return commitErr
	})
	if !errors.Is(err, commitErr) {
		t.Fatalf("ReplaceKey = %v, want local commit error", err)
	}
	reader := newInstallation(t, bucket, map[string]string{})
	if _, err := reader.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone); err != nil {
		t.Fatalf("rollback reused the canceled request context: %v", err)
	}
}

func TestInterruptedKeyRotationCanBeRecoveredByReenteringTheNewKey(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	const next = "a different strong shared synchronization key"
	commitErr := errors.New("local commit failed")
	err := machine.service.ReplaceKey(context.Background(), syncPassphrase, next, true, func() error {
		bucket.refuseNextConditionalPut()
		return commitErr
	})
	if !errors.Is(err, remotesync.ErrRecoveryRequired) {
		t.Fatalf("ReplaceKey = %v, want ErrRecoveryRequired", err)
	}
	journal, err := os.ReadFile(filepath.Join(machine.home, ".ssh", filepath.FromSlash(remotesync.KeyRecoveryPath)))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(journal, []byte(syncPassphrase)) || bytes.Contains(journal, []byte(next)) {
		t.Fatal("recovery journal contains synchronization key material")
	}
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "blocked"); !errors.Is(err, remotesync.ErrRecoveryRequired) {
		t.Fatalf("Push after uncertain rotation = %v, want ErrRecoveryRequired", err)
	}
	other := machine.config
	other.Path = "other-target"
	if err := machine.service.Configure(other, machine.creds, machine.client); !errors.Is(err, remotesync.ErrRecoveryTargetChange) {
		t.Fatalf("Configure during recovery = %v, want ErrRecoveryTargetChange", err)
	}
	if handled, err := machine.service.ResolveKeyRecovery(context.Background(), "wrong but sufficiently long candidate key", func() error { return nil }); !handled || !errors.Is(err, remotesync.ErrWrongPassphrase) {
		t.Fatalf("wrong recovery key = (%v, %v)", handled, err)
	}
	committed := false
	if handled, err := machine.service.ResolveKeyRecovery(context.Background(), next, func() error {
		committed = true
		return nil
	}); !handled || err != nil {
		t.Fatalf("ResolveKeyRecovery = (%v, %v)", handled, err)
	}
	if !committed {
		t.Fatal("recovered key was not committed locally")
	}
	if _, err := os.Stat(filepath.Join(machine.home, ".ssh", filepath.FromSlash(remotesync.KeyRecoveryPath))); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("recovery journal remains: %v", err)
	}
	reader := newInstallation(t, bucket, map[string]string{})
	if _, err := reader.service.Pull(context.Background(), next, remotesync.ResolveNone); err != nil {
		t.Fatalf("advanced object is not readable with recovered key: %v", err)
	}
}

func TestPreparedKeyJournalRecoversAPutThatAdvancedBeforeTheJournal(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	const next = "a different strong shared synchronization key"
	recoveryWrites := 0
	machine.manager.Validate = func(request storage.Request) error {
		if request.Operation != "sync.key-recovery" {
			return nil
		}
		recoveryWrites++
		if recoveryWrites == 2 {
			bucket.refuseNextConditionalPut()
			return errors.New("simulated crash before advanced journal")
		}
		return nil
	}
	err := machine.service.ReplaceKey(context.Background(), syncPassphrase, next, true, func() error { return nil })
	if !errors.Is(err, remotesync.ErrRecoveryRequired) {
		t.Fatalf("ReplaceKey = %v, want ErrRecoveryRequired", err)
	}
	machine.manager.Validate = nil
	journal, err := os.ReadFile(filepath.Join(machine.home, ".ssh", filepath.FromSlash(remotesync.KeyRecoveryPath)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(journal, []byte(`"phase": "prepared"`)) ||
		!bytes.Contains(journal, []byte(`"newCiphertextSHA256"`)) {
		t.Fatalf("prepared journal lacks ciphertext evidence: %s", journal)
	}
	if bytes.Contains(journal, []byte(next)) || bytes.Contains(journal, []byte(syncPassphrase)) {
		t.Fatal("prepared recovery journal contains key material")
	}
	committed := false
	if handled, err := machine.service.ResolveKeyRecovery(context.Background(), next, func() error {
		committed = true
		return nil
	}); !handled || err != nil || !committed {
		t.Fatalf("ResolveKeyRecovery = (%v, %v), committed=%v", handled, err, committed)
	}
	reader := newInstallation(t, bucket, map[string]string{})
	if _, err := reader.service.Pull(context.Background(), next, remotesync.ResolveNone); err != nil {
		t.Fatalf("prepared recovery did not adopt the actual ETag: %v", err)
	}
}

func TestPreparedKeyJournalRecoversWhenTheInitialPutResponseIsLost(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	const next = "a different strong shared synchronization key"
	bucket.failAfterNextConditionalPut()
	err := machine.service.ReplaceKey(context.Background(), syncPassphrase, next, true, func() error { return nil })
	if !errors.Is(err, remotesync.ErrRecoveryRequired) {
		t.Fatalf("ReplaceKey = %v, want ErrRecoveryRequired", err)
	}
	bucket.restoreConditionalResponses()
	journalPath := filepath.Join(machine.home, ".ssh", filepath.FromSlash(remotesync.KeyRecoveryPath))
	journal, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(journal, []byte(`"phase": "prepared"`)) {
		t.Fatalf("journal = %s, want prepared phase", journal)
	}
	committed := false
	if handled, err := machine.service.ResolveKeyRecovery(context.Background(), next, func() error {
		committed = true
		return nil
	}); !handled || err != nil || !committed {
		t.Fatalf("ResolveKeyRecovery = (%v, %v), committed=%v", handled, err, committed)
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("recovery journal remains: %v", err)
	}
	reader := newInstallation(t, bucket, map[string]string{})
	if _, err := reader.service.Pull(context.Background(), next, remotesync.ResolveNone); err != nil {
		t.Fatalf("response-loss recovery did not adopt the observed ETag: %v", err)
	}
}

func TestKeyRecoveryFailsClosedWhenTheLiveCiphertextMatchesNeitherGeneration(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	const next = "a different strong shared synchronization key"
	bucket.failAfterNextConditionalPut()
	if err := machine.service.ReplaceKey(context.Background(), syncPassphrase, next, true, func() error { return nil }); !errors.Is(err, remotesync.ErrRecoveryRequired) {
		t.Fatalf("ReplaceKey = %v, want ErrRecoveryRequired", err)
	}
	bucket.restoreConditionalResponses()
	bucket.putObject(remotesync.ObjectName, []byte("third-party ciphertext"), `"third-party"`)

	committed := false
	handled, err := machine.service.ResolveKeyRecovery(context.Background(), next, func() error {
		committed = true
		return nil
	})
	if !handled || !errors.Is(err, remotesync.ErrRecoveryRequired) {
		t.Fatalf("ResolveKeyRecovery = (%v, %v), want fail-closed recovery", handled, err)
	}
	if committed {
		t.Fatal("unknown live ciphertext committed the candidate synchronization key")
	}
	journalPath := filepath.Join(machine.home, ".ssh", filepath.FromSlash(remotesync.KeyRecoveryPath))
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("fail-closed recovery removed its journal: %v", err)
	}
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "blocked"); !errors.Is(err, remotesync.ErrRecoveryRequired) {
		t.Fatalf("Push after unknown live ciphertext = %v, want ErrRecoveryRequired", err)
	}
}

func TestRollbackResponseLossConvergesWhenOldCiphertextKeepsItsETag(t *testing.T) {
	bucket := &fakeBucket{contentAddressedETag: true}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	const next = "a different strong shared synchronization key"
	commitErr := errors.New("local commit failed")
	err := machine.service.ReplaceKey(context.Background(), syncPassphrase, next, true, func() error {
		bucket.failAfterNextConditionalPut()
		return commitErr
	})
	if !errors.Is(err, remotesync.ErrRecoveryRequired) {
		t.Fatalf("ReplaceKey = %v, want ErrRecoveryRequired", err)
	}
	bucket.restoreConditionalResponses()
	journalPath := filepath.Join(machine.home, ".ssh", filepath.FromSlash(remotesync.KeyRecoveryPath))
	journal, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(journal, []byte(`"oldCiphertextSHA256"`)) {
		t.Fatalf("journal lacks old ciphertext evidence: %s", journal)
	}
	committed := false
	handled, err := machine.service.ResolveKeyRecovery(context.Background(), next, func() error {
		committed = true
		return nil
	})
	if handled || err != nil {
		t.Fatalf("ResolveKeyRecovery = (%v, %v), want old generation convergence", handled, err)
	}
	if committed {
		t.Fatal("old-ciphertext recovery committed the candidate new key")
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("recovery journal remains: %v", err)
	}
	machine.write(t, "config", "Host after-rollback\n")
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "After rollback"); err != nil {
		t.Fatalf("old key generation did not adopt the observed rollback ETag: %v", err)
	}
	reader := newInstallation(t, bucket, map[string]string{})
	if _, err := reader.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone); err != nil {
		t.Fatalf("rolled-back head is not readable with the old key: %v", err)
	}
}

func TestReplaceKeyRollsRemoteBackBeforeLocalCommitWhenStateWriteFails(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host one\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	stateWrites := 0
	machine.manager.Validate = func(request storage.Request) error {
		if request.Operation == "sync.state" {
			stateWrites++
			if stateWrites == 1 {
				return errors.New("state write failed")
			}
		}
		return nil
	}
	committed := false
	err := machine.service.ReplaceKey(context.Background(), syncPassphrase, "a different strong shared synchronization key", true, func() error {
		committed = true
		return nil
	})
	if err == nil {
		t.Fatal("ReplaceKey succeeded despite the refused state write")
	}
	if committed {
		t.Fatal("local key was committed before sync state")
	}
	reader := newInstallation(t, bucket, map[string]string{})
	if _, err := reader.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone); err != nil {
		t.Fatalf("state failure did not restore the old remote key: %v", err)
	}
	if err := machine.service.ReplaceKey(context.Background(), syncPassphrase, "a different strong shared synchronization key", true, func() error {
		return nil
	}); err != nil {
		t.Fatalf("state did not record the rollback ETag: %v", err)
	}
}

func TestForcePushReplacesOnlyTheConfirmedRemoteGeneration(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "Host first\n"})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}
	second := newInstallation(t, bucket, map[string]string{"config": "Host replacement\n"})
	if _, err := second.service.Push(context.Background(), syncPassphrase, ""); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("normal Push = %v, want ErrRemoteMoved", err)
	}
	confirmation, err := second.service.ForcePushConfirmation(context.Background(), remotesync.ForcePushTarget)
	if err != nil {
		t.Fatal(err)
	}
	if confirmation.ETag == "" || confirmation.Evidence == "" {
		t.Fatalf("confirmation = %#v", confirmation)
	}
	if _, err := second.service.ForcePush(context.Background(), syncPassphrase, remotesync.ForcePushConfirmation{
		ETag: confirmation.ETag, Evidence: confirmation.Evidence,
	}, ""); !errors.Is(err, remotesync.ErrForcePushTarget) {
		t.Fatalf("ForcePush with ETag-only confirmation = %v, want ErrForcePushTarget", err)
	}
	if _, err := second.service.ForcePush(context.Background(), syncPassphrase, confirmation, ""); err != nil {
		t.Fatalf("ForcePush = %v", err)
	}

	// The old installation must not silently accept the unrelated force-pushed
	// generation. A fresh installation can still adopt that confirmed head.
	if _, err := first.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveRemote); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("Pull after unrelated ForcePush = %v, want ErrRemoteMoved", err)
	}
	reader := newInstallation(t, bucket, map[string]string{})
	pulled, err := reader.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveRemote)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.service.Apply(pulled); err != nil {
		t.Fatal(err)
	}
	if got := reader.read(t, "config"); got != "Host replacement\n" {
		t.Fatalf("remote contents = %q", got)
	}

	stale, err := second.service.ForcePushConfirmation(context.Background(), remotesync.ForcePushTarget)
	if err != nil {
		t.Fatal(err)
	}
	bucket.replace(remotesync.ObjectName, `"moved-after-confirmation"`)
	if _, err := second.service.ForcePush(context.Background(), syncPassphrase, stale, ""); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("ForcePush after remote change = %v, want ErrRemoteMoved", err)
	}
}

func TestForcePushConfirmationCannotCrossConfiguredTargetsWithTheSameETag(t *testing.T) {
	firstBucket := &fakeBucket{}
	first := newInstallation(t, firstBucket, map[string]string{"config": "Host first-target\n"})
	if _, err := first.service.Push(context.Background(), syncPassphrase, "First target"); err != nil {
		t.Fatal(err)
	}
	secondBucket := &fakeBucket{}
	second := newInstallation(t, secondBucket, map[string]string{"config": "Host second-target\n"})
	if _, err := second.service.Push(context.Background(), syncPassphrase, "Second target"); err != nil {
		t.Fatal(err)
	}

	actor := newInstallation(t, firstBucket, map[string]string{"config": "Host replacement\n"})
	confirmation, err := actor.service.ForcePushConfirmation(context.Background(), remotesync.ForcePushTarget)
	if err != nil {
		t.Fatal(err)
	}
	secondETag, err := second.client.Head(context.Background(), remotesync.ObjectName)
	if err != nil {
		t.Fatal(err)
	}
	if confirmation.ETag != secondETag {
		t.Fatalf("fixture ETags differ: first %q, second %q", confirmation.ETag, secondETag)
	}
	beforeBody := append([]byte(nil), secondBucket.object(remotesync.ObjectName)...)
	beforeKeys := strings.Join(secondBucket.keys(), "\n")

	if err := actor.service.Configure(second.config, second.creds, second.client); err != nil {
		t.Fatal(err)
	}
	if _, err := actor.service.ForcePush(context.Background(), syncPassphrase, confirmation, "Wrong target"); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("ForcePush after target switch = %v, want ErrRemoteMoved", err)
	}
	if !bytes.Equal(secondBucket.object(remotesync.ObjectName), beforeBody) ||
		strings.Join(secondBucket.keys(), "\n") != beforeKeys {
		t.Fatal("force push wrote to the newly configured target")
	}
}

func TestForcePushConfirmationBindsTheConfiguredCredentialGeneration(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host source\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial snapshot"); err != nil {
		t.Fatal(err)
	}
	confirmation, err := machine.service.ForcePushConfirmation(context.Background(), remotesync.ForcePushTarget)
	if err != nil {
		t.Fatal(err)
	}
	beforeKeys := strings.Join(bucket.keys(), "\n")

	nextCredentials := objectstore.Credentials{AccessKeyID: "NEXT", SecretAccessKey: "next-secret"}
	nextClient := *machine.client
	nextClient.Creds = nextCredentials
	if err := machine.service.Configure(machine.config, nextCredentials, &nextClient); err != nil {
		t.Fatal(err)
	}
	next, err := machine.service.ForcePushConfirmation(context.Background(), remotesync.ForcePushTarget)
	if err != nil {
		t.Fatal(err)
	}
	if next.ETag != confirmation.ETag || next.Evidence == confirmation.Evidence {
		t.Fatalf("confirmation did not bind the new binding version: before %#v, after %#v", confirmation, next)
	}
	if _, err := machine.service.ForcePush(context.Background(), syncPassphrase, confirmation, "Stale binding"); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("ForcePush after credential reconfigure = %v, want ErrRemoteMoved", err)
	}
	if strings.Join(bucket.keys(), "\n") != beforeKeys {
		t.Fatal("stale confirmation created a history candidate")
	}
}

func TestReceiveOnlyCanExplicitlyAcceptAnUnrelatedRemoteHead(t *testing.T) {
	bucket := &fakeBucket{}
	receiver := newInstallation(t, bucket, map[string]string{"config": "Host original\n"})
	if _, err := receiver.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	receiver.direct(remotesync.DirectionPull)

	replacement := newInstallation(t, bucket, map[string]string{"config": "Host replacement\n"})
	confirmation, err := replacement.service.ForcePushConfirmation(context.Background(), remotesync.ForcePushTarget)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := replacement.service.ForcePush(context.Background(), syncPassphrase, confirmation, "Replace head"); err != nil {
		t.Fatal(err)
	}

	if _, err := receiver.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveRemote); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("ordinary Pull = %v, want ErrRemoteMoved", err)
	}
	preview, err := receiver.service.PullRemoteHead(context.Background(), syncPassphrase)
	if err != nil {
		t.Fatalf("PullRemoteHead = %v", err)
	}
	if preview.ETag == "" || preview.Manifest.Revision == "" {
		t.Fatalf("preview was not bound to one remote generation: %#v", preview)
	}
	if _, err := receiver.service.PullAndApplyRemoteHeadUsing(
		context.Background(),
		func() (string, error) { return syncPassphrase, nil },
		preview.ETag,
		preview.Manifest.Revision,
	); err != nil {
		t.Fatalf("PullAndApplyRemoteHeadUsing = %v", err)
	}
	if got := receiver.read(t, "config"); got != "Host replacement\n" {
		t.Fatalf("received config = %q", got)
	}
}

func TestExplicitRemoteHeadApplyRejectsAChangedGeneration(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host remote\n"})
	if _, err := producer.service.Push(context.Background(), syncPassphrase, "Remote setup"); err != nil {
		t.Fatal(err)
	}
	receiver := newInstallation(t, bucket, map[string]string{"config": "Host local\n"})
	receiver.direct(remotesync.DirectionPull)
	preview, err := receiver.service.PullRemoteHead(context.Background(), syncPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	bucket.replace(remotesync.ObjectName, `"changed-after-preview"`)
	_, err = receiver.service.PullAndApplyRemoteHeadUsing(
		context.Background(),
		func() (string, error) { return syncPassphrase, nil },
		preview.ETag,
		preview.Manifest.Revision,
	)
	if !errors.Is(err, remotesync.ErrPreviewStale) {
		t.Fatalf("changed remote apply = %v, want ErrPreviewStale", err)
	}
	if got := receiver.read(t, "config"); got != "Host local\n" {
		t.Fatalf("stale apply changed config to %q", got)
	}
}

func TestExplicitRemoteHeadRefusesSendOnlyDirection(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host local\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	machine.direct(remotesync.DirectionPush)
	if _, err := machine.service.PullRemoteHead(context.Background(), syncPassphrase); !errors.Is(err, remotesync.ErrApplyRefused) {
		t.Fatalf("PullRemoteHead in send-only mode = %v, want ErrApplyRefused", err)
	}
}

func TestStatefulSyncOperationsAreSerializedByTheService(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{})
	firstEntered := make(chan struct{})
	secondEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls int
	var callsMu sync.Mutex
	stableSnapshot := func(snapshot func() error) error {
		callsMu.Lock()
		calls++
		call := calls
		callsMu.Unlock()
		switch call {
		case 1:
			close(firstEntered)
			<-releaseFirst
		case 2:
			close(secondEntered)
		}
		return snapshot()
	}
	hooks := defaultIntegrationHooks()
	hooks.StableSnapshot = stableSnapshot
	service, err := remotesync.NewIntegratedService(machine.workspace, machine.manager,
		func() string { return "2026-08-25T02:00:00Z" },
		func() (string, error) { return "serialized-origin", nil }, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Configure(machine.config, machine.creds, machine.client); err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := service.Push(context.Background(), syncPassphrase, "")
		firstDone <- err
	}()
	<-firstEntered
	go func() {
		_, err := service.Push(context.Background(), syncPassphrase, "")
		secondDone <- err
	}()

	select {
	case <-secondEntered:
		t.Fatal("a second Push entered Collect while the first operation was running")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Push = %v", err)
	}
	if err := <-secondDone; !errors.Is(err, remotesync.ErrNothingToPush) {
		t.Fatalf("second Push = %v, want ErrNothingToPush after serialization", err)
	}
}

// Configure は実行中の同期が終わるまで成功しない。設定保存の応答後にも旧bucketへの
// 書き込みが続く状態を作らず、ひとつのoperationはひとつのbindingだけを使う。
func TestConfigureWaitsForAnInFlightPush(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)

	collecting := make(chan struct{})
	resume := make(chan struct{})
	var once sync.Once
	stableSnapshot := func(snapshot func() error) error {
		once.Do(func() { close(collecting) })
		<-resume
		return snapshot()
	}
	hooks := defaultIntegrationHooks()
	hooks.StableSnapshot = stableSnapshot
	service, err := remotesync.NewIntegratedService(workspace, manager,
		func() string { return "2026-08-05T00:00:00Z" },
		func() (string, error) { return "origin-A", nil }, hooks)
	if err != nil {
		t.Fatal(err)
	}

	oldBucket, newBucket := &fakeBucket{}, &fakeBucket{}
	oldServer := httptest.NewTLSServer(oldBucket.handler())
	newServer := httptest.NewTLSServer(newBucket.handler())
	t.Cleanup(oldServer.Close)
	t.Cleanup(newServer.Close)
	credentials := objectstore.Credentials{AccessKeyID: "AKID", SecretAccessKey: "secret"}
	client := func(server *httptest.Server) *objectstore.Client {
		return &objectstore.Client{
			HTTP: server.Client(), Endpoint: server.URL, Bucket: "sshc", Region: "auto",
			Creds: credentials,
		}
	}
	if err := service.Configure(remotesync.Config{
		Endpoint: oldServer.URL, Bucket: "sshc", Region: "auto", Path: "old", Direction: remotesync.DirectionBoth,
	}, credentials, client(oldServer)); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := service.Push(context.Background(), syncPassphrase, "")
		result <- err
	}()
	select {
	case <-collecting:
	case <-time.After(30 * time.Second):
		close(resume)
		t.Fatal("Push did not reach collection")
	}
	configured := make(chan struct{})
	go func() {
		if err := service.Configure(remotesync.Config{
			Endpoint: newServer.URL, Bucket: "sshc", Region: "auto", Path: "new", Direction: remotesync.DirectionBoth,
		}, credentials, client(newServer)); err != nil {
			panic(err)
		}
		close(configured)
	}()
	select {
	case <-configured:
		close(resume)
		t.Fatal("Configure completed while the old binding was still in use")
	case <-time.After(50 * time.Millisecond):
	}
	close(resume)

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Push = %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Push did not finish")
	}
	select {
	case <-configured:
	case <-time.After(30 * time.Second):
		t.Fatal("Configure did not finish after Push")
	}
	if got := oldBucket.keys(); len(got) != 2 || oldBucket.object("old/"+remotesync.ObjectName) == nil {
		t.Errorf("old binding holds %v, want its live object and dated copy", got)
	}
	if got := newBucket.keys(); len(got) != 0 {
		t.Errorf("new binding was used by an in-flight push: %v", got)
	}
}

func TestPushReadsTheSynchronizationKeyAfterAConcurrentRotation(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host initial\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	machine.write(t, "config", "Host changed\n")
	const next = "a different strong shared synchronization key"
	currentKey := syncPassphrase
	commitEntered := make(chan struct{})
	releaseCommit := make(chan struct{})
	rotationDone := make(chan error, 1)
	go func() {
		rotationDone <- machine.service.ReplaceKey(context.Background(), syncPassphrase, next, true, func() error {
			close(commitEntered)
			<-releaseCommit
			currentKey = next
			return nil
		})
	}()
	<-commitEntered
	providerCalled := make(chan struct{})
	pushDone := make(chan error, 1)
	go func() {
		_, err := machine.service.PushUsing(context.Background(), func() (string, error) {
			close(providerCalled)
			return currentKey, nil
		}, "After rotation")
		pushDone <- err
	}()
	select {
	case <-providerCalled:
		close(releaseCommit)
		t.Fatal("Push captured the old key before ReplaceKey released operationMu")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCommit)
	if err := <-rotationDone; err != nil {
		t.Fatal(err)
	}
	if err := <-pushDone; err != nil {
		t.Fatal(err)
	}
	reader := newInstallation(t, bucket, map[string]string{})
	result, err := reader.service.Pull(context.Background(), next, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("new head was not sealed by the current key: %v", err)
	}
	if err := reader.service.Apply(result); err != nil {
		t.Fatal(err)
	}
	if got := reader.read(t, "config"); got != "Host changed\n" {
		t.Fatalf("pushed config = %q", got)
	}
}

func TestForcePushReadsTheSynchronizationKeyAfterAConcurrentRotation(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host initial\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	confirmation, err := machine.service.ForcePushConfirmation(context.Background(), remotesync.ForcePushTarget)
	if err != nil {
		t.Fatal(err)
	}
	const next = "a different strong shared synchronization key"
	currentKey := syncPassphrase
	commitEntered := make(chan struct{})
	releaseCommit := make(chan struct{})
	rotationDone := make(chan error, 1)
	go func() {
		rotationDone <- machine.service.ReplaceKey(context.Background(), syncPassphrase, next, true, func() error {
			close(commitEntered)
			<-releaseCommit
			currentKey = next
			return nil
		})
	}()
	<-commitEntered
	providerCalled := make(chan string, 1)
	forceDone := make(chan error, 1)
	go func() {
		_, err := machine.service.ForcePushUsing(context.Background(), func() (string, error) {
			providerCalled <- currentKey
			return currentKey, nil
		}, confirmation, "Forced")
		forceDone <- err
	}()
	select {
	case key := <-providerCalled:
		close(releaseCommit)
		t.Fatalf("ForcePush captured %q before rotation completed", key)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCommit)
	if err := <-rotationDone; err != nil {
		t.Fatal(err)
	}
	if key := <-providerCalled; key != next {
		t.Fatalf("ForcePush key = %q, want rotated key", key)
	}
	if err := <-forceDone; !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("ForcePush = %v, want stale ETag refusal", err)
	}
}

func TestPullAndApplyReadsTheSynchronizationKeyAfterAConcurrentRotation(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host remote\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	preview, err := machine.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		t.Fatal(err)
	}
	const next = "a different strong shared synchronization key"
	currentKey := syncPassphrase
	commitEntered := make(chan struct{})
	releaseCommit := make(chan struct{})
	rotationDone := make(chan error, 1)
	go func() {
		rotationDone <- machine.service.ReplaceKey(context.Background(), syncPassphrase, next, true, func() error {
			close(commitEntered)
			<-releaseCommit
			currentKey = next
			return nil
		})
	}()
	<-commitEntered
	providerCalled := make(chan string, 1)
	applyDone := make(chan error, 1)
	go func() {
		_, err := machine.service.PullAndApplyUsing(context.Background(), func() (string, error) {
			providerCalled <- currentKey
			return currentKey, nil
		}, remotesync.ResolveNone, "", preview.ETag, preview.Manifest.Revision)
		applyDone <- err
	}()
	select {
	case key := <-providerCalled:
		close(releaseCommit)
		t.Fatalf("PullAndApply captured %q before rotation completed", key)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCommit)
	if err := <-rotationDone; err != nil {
		t.Fatal(err)
	}
	if key := <-providerCalled; key != next {
		t.Fatalf("PullAndApply key = %q, want rotated key", key)
	}
	if err := <-applyDone; !errors.Is(err, remotesync.ErrPreviewStale) {
		t.Fatalf("PullAndApply = %v, want stale preview refusal", err)
	}
}

func TestAutoReadsTheSynchronizationKeyAfterAConcurrentRotation(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host initial\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	machine.write(t, "config", "Host auto-change\n")
	const next = "a different strong shared synchronization key"
	currentKey := syncPassphrase
	commitEntered := make(chan struct{})
	releaseCommit := make(chan struct{})
	rotationDone := make(chan error, 1)
	go func() {
		rotationDone <- machine.service.ReplaceKey(context.Background(), syncPassphrase, next, true, func() error {
			close(commitEntered)
			<-releaseCommit
			currentKey = next
			return nil
		})
	}()
	<-commitEntered
	auto := remotesync.NewAuto(machine.service, time.Minute, func() string { return "2026-08-25T00:00:00Z" })
	auto.Enabled = func() bool { return true }
	providerCalled := make(chan struct{})
	auto.Key = func() (string, bool) {
		close(providerCalled)
		return currentKey, true
	}
	autoDone := make(chan remotesync.AutoView, 1)
	go func() { autoDone <- auto.Once(context.Background()) }()
	select {
	case <-providerCalled:
		close(releaseCommit)
		t.Fatal("Auto captured the old key before ReplaceKey released operationMu")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCommit)
	if err := <-rotationDone; err != nil {
		t.Fatal(err)
	}
	if view := <-autoDone; view.Phase != remotesync.AutoIdle {
		t.Fatalf("Auto = %+v", view)
	}
	reader := newInstallation(t, bucket, map[string]string{})
	if _, err := reader.service.Pull(context.Background(), next, remotesync.ResolveNone); err != nil {
		t.Fatalf("auto head was not sealed by the current key: %v", err)
	}
}

func TestReconfigurePersistsSettingsAndSwapsBindingBeforeAWaitingPush(t *testing.T) {
	oldBucket := &fakeBucket{}
	machine := newInstallation(t, oldBucket, map[string]string{"config": "Host local\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	oldObjects := len(oldBucket.keys())
	newBucket := &fakeBucket{}
	server := httptest.NewTLSServer(newBucket.handler())
	t.Cleanup(server.Close)
	config := machine.config
	config.Endpoint = server.URL
	config.Path = "new"
	client := &objectstore.Client{HTTP: server.Client(), Endpoint: server.URL, Bucket: config.Bucket, Region: config.Region, Creds: machine.creds}
	const newTargetKey = "a different strong shared synchronization key"
	currentKey := syncPassphrase
	persistEntered := make(chan struct{})
	releasePersist := make(chan struct{})
	reconfigureDone := make(chan error, 1)
	go func() {
		reconfigureDone <- machine.service.Reconfigure(config, machine.creds, client, func() error {
			currentKey = newTargetKey
			close(persistEntered)
			<-releasePersist
			return nil
		})
	}()
	<-persistEntered
	pushDone := make(chan error, 1)
	go func() {
		_, err := machine.service.PushUsing(context.Background(), func() (string, error) { return currentKey, nil }, "New target")
		pushDone <- err
	}()
	select {
	case err := <-pushDone:
		close(releasePersist)
		t.Fatalf("Push crossed the settings/binding transaction: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releasePersist)
	if err := <-reconfigureDone; err != nil {
		t.Fatal(err)
	}
	if err := <-pushDone; err != nil {
		t.Fatal(err)
	}
	if len(oldBucket.keys()) != oldObjects {
		t.Fatalf("old target changed during Reconfigure: %v", oldBucket.keys())
	}
	reader := newInstallation(t, newBucket, map[string]string{})
	readerConfig := reader.config
	readerConfig.Path = "new"
	if err := reader.service.Configure(readerConfig, reader.creds, reader.client); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.service.Pull(context.Background(), newTargetKey, remotesync.ResolveNone); err != nil {
		t.Fatalf("new target was not written with persisted key: %v", err)
	}
}

func TestSetKeyWaitsForReconfigurePersistenceAndReadsTheNewGeneration(t *testing.T) {
	oldBucket := &fakeBucket{}
	machine := newInstallation(t, oldBucket, map[string]string{"config": "Host local\n"})
	if _, err := machine.service.Push(context.Background(), syncPassphrase, "Initial setup"); err != nil {
		t.Fatal(err)
	}
	newBucket := &fakeBucket{}
	server := httptest.NewTLSServer(newBucket.handler())
	t.Cleanup(server.Close)
	config := machine.config
	config.Endpoint = server.URL
	config.Path = "new"
	client := &objectstore.Client{HTTP: server.Client(), Endpoint: server.URL, Bucket: config.Bucket, Region: config.Region, Creds: machine.creds}

	persistEntered := make(chan struct{})
	releasePersist := make(chan struct{})
	reconfigureDone := make(chan error, 1)
	go func() {
		reconfigureDone <- machine.service.Reconfigure(config, machine.creds, client, func() error {
			close(persistEntered)
			<-releasePersist
			return nil
		})
	}()
	<-persistEntered

	providerCalled := make(chan struct{})
	committedTarget := make(chan string, 1)
	setKeyDone := make(chan error, 1)
	go func() {
		setKeyDone <- machine.service.ReplaceKeyUsing(context.Background(),
			"a different strong shared synchronization key", true,
			func() (string, func() error, error) {
				close(providerCalled)
				return syncPassphrase, func() error {
					_, _, path, _ := machine.service.Target()
					committedTarget <- path
					return nil
				}, nil
			})
	}()
	select {
	case <-providerCalled:
		close(releasePersist)
		t.Fatal("SetKey read the old settings while Reconfigure persist was in progress")
	case <-time.After(50 * time.Millisecond):
	}
	close(releasePersist)
	if err := <-reconfigureDone; err != nil {
		t.Fatal(err)
	}
	if err := <-setKeyDone; err != nil {
		t.Fatal(err)
	}
	if path := <-committedTarget; path != "new" {
		t.Fatalf("SetKey committed against path %q, want new binding", path)
	}
	if got := newBucket.keys(); len(got) != 0 {
		t.Fatalf("first key setup for a new target unexpectedly rewrote remote objects: %v", got)
	}
}

// Preview evidence belongs to one binding generation. Reconfiguration makes it
// stale and Apply must not write files from the old bucket.
func TestApplyRejectsAPreviewFromAReconfiguredBinding(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := producer.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("producer Push = %v", err)
	}

	consumer := newInstallation(t, bucket, map[string]string{})
	result, err := consumer.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("consumer Pull = %v", err)
	}
	config := consumer.config
	config.Path = "new"
	if err := consumer.service.Configure(config, consumer.creds, consumer.client); err != nil {
		t.Fatal(err)
	}
	if err := consumer.service.Apply(result); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("consumer Apply = %v, want ErrRemoteMoved", err)
	}
	if _, err := consumer.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("Push after reconfiguration = %v", err)
	}
	if got := bucket.object("new/" + remotesync.ObjectName); got == nil {
		t.Errorf("nothing was written to the new key: %v", bucket.keys())
	}
}

// push ごとに日付付きコピーを残し、条件付き書き込み用の最新オブジェクトは固定キーに保つ。
func TestEveryPushLeavesADatedCopyBesideTheLiveObject(t *testing.T) {
	bucket := &fakeBucket{}
	installation := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})

	if _, err := installation.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("Push = %v", err)
	}
	if got := bucket.unconditionalHistoryPuts(); got != 0 {
		t.Fatalf("history objects created without If-None-Match: * = %d", got)
	}

	keys := bucket.keys()
	if len(keys) != 2 {
		t.Fatalf("the bucket holds %v, want the live object and one dated copy", keys)
	}
	live, dated := "", ""
	for _, key := range keys {
		if strings.Contains(key, "snapshots/") {
			dated = key
			continue
		}
		live = key
	}
	if !strings.HasSuffix(live, "workspace.tar.gz.enc") {
		t.Errorf("the live object is %q", live)
	}
	if !strings.HasSuffix(dated, ".tar.gz.enc") || !strings.Contains(dated, "2026-08-05") {
		t.Errorf("the dated copy is %q", dated)
	}
	// 同じバイト列なので、コピーのコストはアップロード 1 回で、二度目の暗号化込めは不要。
	if !bytes.Equal(bucket.object(live), bucket.object(dated)) {
		t.Error("the dated copy is not the snapshot that was pushed")
	}
}

// オブジェクトの名前を変えたり、別のパスへ移したりしても、すでに同期済みのマシンを
// 置き去りにしてはならない。
//
// state は、このマシンが最後に見たスナップショットの ETag を記録する。それがどの
// オブジェクトのものかは記録していなかったので、キーが変わったあとの次の push は、
// 存在しないオブジェクトの世代に対して If-Match を送り、「別のマシンが push した、
// まず pull せよ」として拒否され、そこでの pull は、pull すべきものを何も見つけ
// られなかった。そこから抜け出す方法は、state ファイルを手で削除する以外になかった。
func TestChangingTheObjectKeyDoesNotStrandAMachineThatHasSynced(t *testing.T) {
	bucket := &fakeBucket{}
	installation := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := installation.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("the first push = %v", err)
	}

	// 設定がパスを指定するようになったので、ライブのオブジェクトは別の場所にある。
	config := installation.config
	config.Path = "laptops"
	if err := installation.service.Configure(config, installation.creds, installation.client); err != nil {
		t.Fatal(err)
	}

	if _, err := installation.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("the push after the key changed = %v", err)
	}
	if got := bucket.object("laptops/" + remotesync.ObjectName); got == nil {
		t.Errorf("nothing was written to the new key: %v", bucket.keys())
	}
}

// withVault は、この設置に本物の保管庫を与え、同期へvault 変換関数を繋ぐ。
func withVault(t *testing.T, machine *installation, master string) *secret.Service {
	t.Helper()
	secrets := secret.NewService(machine.workspace, machine.manager, time.Now)
	if err := secrets.Initialise(master); err != nil {
		t.Fatalf("Initialise: %v", err)
	}
	machine.replaceIntegrations(t, func(hooks *remotesync.IntegrationHooks) {
		hooks.OpenVault = secrets.TravelDocument
		hooks.SealVault = secrets.AdoptTravelDocument
		hooks.EmptyVaultDocument = secrets.EmptyTravelDocument
		hooks.VaultAdopted = secrets.Reload
	})
	return secrets
}

// これがこの設計そのものである。保存したパスワードは端末をまたいで運ばれ、
// マスターパスワードは端末ごとに別のままである。同期するのは復号済み文書であり、
// というのはそういう意味である。
func TestSavedPasswordsTravelWhileMasterPasswordsStayLocal(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	sender := withVault(t, &first, "the first machine's master")
	const binding = "abababababababababababababababababababababababababababababababab"
	if err := sender.SetBound("bastion", "the password for bastion", binding); err != nil {
		t.Fatal(err)
	}
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatalf("Push = %v", err)
	}

	// 送り出したものの中に、暗号化された保管庫は入っていない。
	sealed := bucket.object(remotesync.ObjectName)
	archive, _, err := envelope.OpenWithin(sealed, syncPassphrase, envelope.AcceptedFromRemote)
	if err != nil {
		t.Fatal(err)
	}
	manifest, contents, err := remotesync.Read(archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := contents[remotesync.VaultPath]; present {
		t.Fatal("the sealed vault travelled; the other machine would need this machine's master password")
	}
	if _, present := contents[remotesync.TravelPath]; !present {
		t.Fatalf("the vault contents did not travel: %+v", manifest.Files)
	}

	// 2 台目は、自分のマスターパスワードで自分の保管庫を作る。
	second := newInstallation(t, bucket, map[string]string{})
	receiver := withVault(t, &second, "the second machine's own master")
	result, err := second.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatalf("Pull = %v", err)
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf("a machine that has saved nothing conflicted: %+v", result.Conflicts)
	}
	const receiverMaster = "the receiver's rotated master password"
	if err := receiver.ChangeMasterPassword("the second machine's own master", receiverMaster); err != nil {
		t.Fatalf("ChangeMasterPassword between Pull and Apply = %v", err)
	}
	if err := second.service.Apply(result); err != nil {
		t.Fatalf("Apply = %v", err)
	}

	// 運ばれてきた。そして読み直されている。次のロック解除まで待たされない。
	if got := receiver.BoundPasswordFor("bastion", binding); got != "the password for bastion" {
		t.Fatalf("the password did not travel: %q", got)
	}
	// そして 2 台目は、いまも自分のマスターパスワードで開く。
	receiver.Lock()
	if err := receiver.Unlock(receiverMaster); err != nil {
		t.Fatalf("the second machine can no longer open its own vault: %v", err)
	}
	if got := receiver.BoundPasswordFor("bastion", binding); got != "the password for bastion" {
		t.Fatalf("after unlocking with its own master password: %q", got)
	}
}

func TestAnExplicitEmptyVaultClearsCredentialsOnAnotherInstallation(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	sender := withVault(t, &first, "the first machine's master")
	const binding = "abababababababababababababababababababababababababababababababab"
	if err := sender.SetBound("bastion", "password to revoke", binding); err != nil {
		t.Fatal(err)
	}
	if _, err := first.service.Push(context.Background(), syncPassphrase, "Store credential"); err != nil {
		t.Fatal(err)
	}

	second := newInstallation(t, bucket, map[string]string{})
	receiver := withVault(t, &second, "the second machine's master")
	initial, err := second.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.service.Apply(initial); err != nil {
		t.Fatal(err)
	}
	if got := receiver.BoundPasswordFor("bastion", binding); got != "password to revoke" {
		t.Fatalf("initial password = %q", got)
	}

	if err := sender.Remove("bastion"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.service.Push(context.Background(), syncPassphrase, "Remove all credentials"); err != nil {
		t.Fatal(err)
	}
	archive, _, err := envelope.OpenWithin(bucket.object(remotesync.ObjectName), syncPassphrase, envelope.AcceptedFromRemote)
	if err != nil {
		t.Fatal(err)
	}
	_, contents, err := remotesync.Read(archive)
	if err != nil {
		t.Fatal(err)
	}
	logical, present := contents[remotesync.TravelPath]
	if !present || len(logical) != 0 {
		t.Fatalf("empty vault tombstone = %q, present %v", logical, present)
	}

	removal, err := second.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(removal.Conflicts) != 0 || len(removal.Removed) != 0 {
		t.Fatalf("empty vault preview = conflicts %+v, removals %+v", removal.Conflicts, removal.Removed)
	}
	if err := second.service.Apply(removal); err != nil {
		t.Fatal(err)
	}
	if got := receiver.BoundPasswordFor("bastion", binding); got != "" {
		t.Fatalf("revoked password survived remote tombstone: %q", got)
	}
	receiver.Lock()
	if err := receiver.Unlock("the second machine's master"); err != nil {
		t.Fatal(err)
	}
	if got := receiver.BoundPasswordFor("bastion", binding); got != "" {
		t.Fatalf("revoked password returned after unlock: %q", got)
	}
}

// 何も保存していない保管庫は、運ぶものを持たない。運べば、2 台目の最初の pull は
// 必ず衝突する。空であることは編集ではない。
func TestAnEmptyVaultDoesNotTravel(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	withVault(t, &machine, "a master password")
	if _, err := machine.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	archive, _, err := envelope.OpenWithin(bucket.object(remotesync.ObjectName), syncPassphrase, envelope.AcceptedFromRemote)
	if err != nil {
		t.Fatal(err)
	}
	_, contents, err := remotesync.Read(archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := contents[remotesync.TravelPath]; present {
		t.Fatal("an empty vault travelled")
	}
	if _, present := contents[remotesync.VaultPath]; present {
		t.Fatal("the vault file travelled")
	}
}

// 背景の画像も同期される。
//
// metadata が運ぶのは表記だけである。画像を置いていかれた端末は、
// 「選ばれているのに何も出ない」状態になる。そして Android はサンドボックスの
// 外を見られないので、あの端末へ画像を持ち込む道はこれしかない。
func TestASnapshotCarriesTheBackgroundImagesTheMetadataNames(t *testing.T) {
	installation := newInstallation(t, &fakeBucket{}, map[string]string{
		"config": "Host bastion\n",
		"sshc/metadata.json": `{"schemaVersion":3,"hosts":[{"identity":{"path":"config","alias":"bastion"},` +
			`"appearance":{"background":"office.png"}}]}`,
		"sshc/backgrounds/office.png": "\x89PNG\r\n\x1a\nbytes",
	})

	manifest, contents, err := installation.service.Collect()
	if err != nil {
		t.Fatalf("Collect = %v", err)
	}
	packed := map[string]bool{}
	for _, entry := range manifest.Files {
		packed[entry.Path] = true
	}
	if !packed["sshc/backgrounds/office.png"] {
		t.Fatalf("the background does not travel: %v", packed)
	}
	if string(contents["sshc/backgrounds/office.png"]) != "\x89PNG\r\n\x1a\nbytes" {
		t.Fatalf("the background travelled with the wrong bytes")
	}
}
