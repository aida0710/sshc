package secret_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

func newService(t *testing.T) (*secret.Service, string) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	return secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now), home
}

func legacyVault(t *testing.T, password string) []byte {
	t.Helper()
	key, err := envelope.Derive(password)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := key.Seal([]byte(`{"schemaVersion":3,"passwords":{},"keyPassphrases":{},"hosts":{},"keys":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func recoveryService(t *testing.T) (*secret.Service, *storage.Workspace, *storage.Manager) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := secret.NewService(workspace, manager, time.Now)
	manager.Seal = service.SealBackup
	manager.Unseal = service.OpenBackup
	return service, workspace, manager
}

func TestUnsupportedVaultCanRecoverTheNewestCompatibleGeneration(t *testing.T) {
	service, workspace, manager := recoveryService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(service, "bastion", "hunter2"); err != nil {
		t.Fatal(err)
	}
	vaultPath := filepath.Join(workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	current, err := os.ReadFile(vaultPath)
	if err != nil {
		t.Fatal(err)
	}
	legacy := legacyVault(t, passphrase)
	if _, err := manager.Commit(storage.Request{
		Operation: "test.install-legacy-vault",
		Changes: []storage.Change{{
			Path: vaultPath, Contents: legacy,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(current)},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if err := service.Unlock(passphrase); !errors.Is(err, secret.ErrOlderSchema) {
		t.Fatalf("Unlock = %v, want ErrOlderSchema", err)
	}

	if err := service.RecoverCompatibleBackup(passphrase); err != nil {
		t.Fatal(err)
	}
	if got := testPasswordFor(service, "bastion"); got != "hunter2" {
		t.Fatalf("recovered password = %q", got)
	}
	service.Lock()
	if err := service.Unlock(passphrase); err != nil {
		t.Fatalf("recovered vault did not survive restart: %v", err)
	}
}

func TestCompatibleBackupRecoveryReSealsProtectedDocumentsIntoTheRecoveredKeyGeneration(t *testing.T) {
	service, workspace, manager := recoveryService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{Bucket: "previous"}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{Bucket: "current"}); err != nil {
		t.Fatal(err)
	}
	protectedPath := filepath.Join(workspace.Root(), "sshc", "snippets.json")
	plaintext := []byte(`{"schemaVersion":1,"snippets":[]}`)
	if err := service.RegisterProtectedDocument(secret.ProtectedDocument{
		Path: protectedPath,
		Validate: func(document []byte) error {
			if !bytes.Equal(document, plaintext) {
				return errors.New("invalid protected document")
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	// The protected document can legitimately belong to another salt generation:
	// for example, a user may have restored only the vault from history. Recovery
	// must use the supplied master password as a fallback instead of mistaking the
	// different derived key for a legacy plaintext document.
	documentGeneration, err := secret.Create(passphrase)
	if err != nil {
		t.Fatal(err)
	}
	sealedDocument, err := documentGeneration.SealBytes(plaintext)
	documentGeneration.Destroy()
	if err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, protectedPath, sealedDocument)

	candidate, err := secret.Create(passphrase)
	if err != nil {
		t.Fatal(err)
	}
	candidateSealed, err := candidate.Seal()
	if err != nil {
		t.Fatal(err)
	}
	unsupported, err := service.SealDocument([]byte(`{"schemaVersion":3,"passwords":{},"keyPassphrases":{},"hosts":{},"keys":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	vaultPath := filepath.Join(workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	acltest.WritePrivateFile(t, vaultPath, candidateSealed)
	if _, err := manager.Commit(storage.Request{
		Operation: "test.install-unsupported-vault",
		Changes: []storage.Change{{
			Path: vaultPath, Contents: unsupported,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(candidateSealed)},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	backupRoot := filepath.Join(workspace.StateDir(), storage.BackupDirectoryName)
	existingBackups := []string{}
	if err := filepath.Walk(backupRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info != nil && !info.IsDir() {
			existingBackups = append(existingBackups, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := service.RecoverCompatibleBackup(passphrase); err != nil {
		t.Fatal(err)
	}
	resealed, err := os.ReadFile(protectedPath)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := service.OpenDocument(resealed)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("protected document after recovery = %q, %v", opened, err)
	}
	settings, err := service.SyncSettings()
	if err != nil || settings.Bucket != "current" {
		t.Fatalf("sync settings after recovery = %#v, %v", settings, err)
	}
	for _, path := range existingBackups {
		outer, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		inner, openErr := service.OpenBackup(outer)
		if openErr != nil {
			t.Errorf("existing backup %s was not moved to the recovered generation: %v", path, openErr)
			continue
		}
		if strings.HasSuffix(filepath.Clean(path), filepath.FromSlash(secret.WorkspacePath)) ||
			strings.HasSuffix(filepath.Clean(path), filepath.FromSlash(secret.SettingsPath)) ||
			strings.HasSuffix(filepath.Clean(path), filepath.Join("sshc", "snippets.json")) {
			if _, innerErr := service.OpenDocument(inner); innerErr != nil {
				t.Errorf("existing key-bound backup %s retained its old inner key: %v", path, innerErr)
			}
		}
	}
}

func TestUnsupportedVaultCanBeResetWithoutRemovingSSHFiles(t *testing.T) {
	service, workspace, _ := recoveryService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	protectedPath := filepath.Join(workspace.Root(), "sshc", "snippets.json")
	protectedPlaintext := []byte(`{"schemaVersion":1,"snippets":[{"command":"deploy --token=top-secret"}]}`)
	if err := service.RegisterProtectedDocument(secret.ProtectedDocument{
		Path: protectedPath,
		Validate: func(document []byte) error {
			if !bytes.Equal(document, protectedPlaintext) {
				return errors.New("invalid protected document")
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	legacy := legacyVault(t, passphrase)
	_, legacyKey, err := envelope.Open(legacy, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	protectedSealed, err := legacyKey.Seal(protectedPlaintext)
	if err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, protectedPath, protectedSealed)
	if err := service.SetSyncSettings(secret.SyncSettings{
		Endpoint: "https://objects.example", Bucket: "workspace", AccessKeyID: "id", SecretAccessKey: "secret",
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{
		Endpoint: "https://objects.example", Bucket: "workspace-new", AccessKeyID: "id", SecretAccessKey: "secret",
	}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workspace.Root(), "config")
	acltest.WritePrivateFile(t, configPath, []byte("Host bastion\n"))
	vaultPath := filepath.Join(workspace.Root(), filepath.FromSlash(secret.WorkspacePath))
	acltest.WritePrivateFile(t, vaultPath, legacy)
	service.Lock()
	backupRoot := filepath.Join(workspace.StateDir(), storage.BackupDirectoryName)
	existingBackups := []string{}
	if err := filepath.Walk(backupRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info != nil && !info.IsDir() {
			existingBackups = append(existingBackups, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := service.ResetUnsupported(passphrase); err != nil {
		t.Fatal(err)
	}
	if !service.Unlocked() {
		t.Fatal("reset vault is not unlocked")
	}
	settings, err := service.SyncSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings != (secret.SyncSettings{}) {
		t.Fatalf("sync settings survived reset: %#v", settings)
	}
	if body, err := os.ReadFile(configPath); err != nil || string(body) != "Host bastion\n" {
		t.Fatalf("SSH config = %q, %v", body, err)
	}
	resealed, err := os.ReadFile(protectedPath)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := service.OpenDocument(resealed)
	if err != nil || !bytes.Equal(opened, protectedPlaintext) {
		t.Fatalf("protected document after reset = %q, %v", opened, err)
	}
	for _, path := range existingBackups {
		outer, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		inner, openErr := service.OpenBackup(outer)
		if openErr != nil {
			t.Errorf("existing backup %s was not moved to the reset generation: %v", path, openErr)
			continue
		}
		if strings.HasSuffix(filepath.Clean(path), filepath.FromSlash(secret.SettingsPath)) {
			if _, innerErr := service.OpenDocument(inner); innerErr != nil {
				t.Errorf("existing settings backup %s retained its old inner key: %v", path, innerErr)
			}
		}
	}
	service.Lock()
	if err := service.Unlock(passphrase); err != nil {
		t.Fatalf("reset vault did not survive restart: %v", err)
	}
}

const testAuthenticationBinding = "abababababababababababababababababababababababababababababababab"

func setTestPassword(service *secret.Service, alias, password string) error {
	return service.SetBound(alias, password, testAuthenticationBinding)
}

func testPasswordFor(service *secret.Service, alias string) string {
	return service.BoundPasswordFor(alias, testAuthenticationBinding)
}

func TestEmptyTravelDocumentUsesTheCurrentUnlockedKeyGeneration(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	empty, err := service.EmptyTravelDocument()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(empty, []byte(`"schemaVersion":4`)) {
		t.Fatalf("EmptyTravelDocument = %q", empty)
	}
	sealed, err := service.AdoptTravelDocument(empty)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := service.OpenDocument(sealed)
	if err != nil || !bytes.Equal(opened, empty) {
		t.Fatalf("adopted empty document = %q, %v", opened, err)
	}
	service.Lock()
	if _, err := service.EmptyTravelDocument(); !errors.Is(err, secret.ErrLocked) {
		t.Fatalf("EmptyTravelDocument while locked = %v, want ErrLocked", err)
	}
}

func TestAdoptTravelDocumentRefusesCiphertextItsBoundedReaderCannotReopen(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	prefix := `{"schemaVersion":4,"passwords":{},"dedicatedPasswords":{"host":"`
	suffix := `"},"keyPassphrases":{},"hosts":{},"keys":{}}`
	valueLength := storage.MaxFileSize - 1 - len(prefix) - len(suffix)
	plain := []byte(prefix + strings.Repeat("x", valueLength) + suffix)
	if len(plain) != storage.MaxFileSize-1 {
		t.Fatalf("test document size = %d", len(plain))
	}
	if _, err := service.AdoptTravelDocument(plain); !errors.Is(err, storage.ErrFileTooLarge) {
		t.Fatalf("AdoptTravelDocument = %v, want ErrFileTooLarge", err)
	}
}

func assignTestPasswordCredential(service *secret.Service, subject, name string) error {
	return service.AssignPasswordCredential(subject, name, testAuthenticationBinding)
}

type syncCASFileSystem struct {
	storage.FileSystem
	path        string
	replacement []byte
	armed       bool
	reads       int
}

func (f *syncCASFileSystem) ReadFile(path string) ([]byte, error) {
	body, err := f.FileSystem.ReadFile(path)
	if err != nil || !f.armed || path != f.path {
		return body, err
	}
	f.reads++
	if f.reads != 2 {
		return body, nil
	}
	if err := os.WriteFile(path, f.replacement, 0o600); err != nil {
		return nil, err
	}
	return slices.Clone(f.replacement), nil
}

// newClockedService は時間を所有するので、アイドルな 1 日を、丸一日待つのでは
// なくテストの 1 行にできる。
func newClockedService(t *testing.T, now func() time.Time) (*secret.Service, string) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	return secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), now), home
}

func vaultPath(home string) string {
	return filepath.Join(home, ".ssh", filepath.FromSlash(secret.WorkspacePath))
}

type stateFaultFileSystem struct {
	storage.FileSystem
	lstatErr  error
	renameErr error
}

// rekeyFaultFileSystem stops master-key rotation at deterministic durability
// boundaries without observing or logging any file contents.
type rekeyFaultFileSystem struct {
	storage.FileSystem
	targets          map[string]bool
	failRenames      map[int]bool
	failTargetSyncAt int
	renames          int
	targetSyncs      int
	applying         bool
	failCleanupSync  bool
	cleanupRemoved   bool
	failure          error
}

func (f *rekeyFaultFileSystem) Rename(oldPath, newPath string) error {
	if f.targets[newPath] {
		f.applying = true
		f.renames++
		if f.failRenames[f.renames] {
			return f.failure
		}
	}
	return f.FileSystem.Rename(oldPath, newPath)
}

func (f *rekeyFaultFileSystem) SyncDir(path string) error {
	if f.cleanupRemoved && f.failCleanupSync {
		f.failCleanupSync = false
		return f.failure
	}
	if f.applying {
		for target := range f.targets {
			if filepath.Dir(target) == path {
				f.applying = false
				f.targetSyncs++
				if f.failTargetSyncAt > 0 && f.targetSyncs == f.failTargetSyncAt {
					return f.failure
				}
				break
			}
		}
	}
	return f.FileSystem.SyncDir(path)
}

func (f *rekeyFaultFileSystem) Remove(path string) error {
	if f.failCleanupSync && strings.Contains(path, string(filepath.Separator)+storage.BackupDirectoryName+string(filepath.Separator)) &&
		!strings.HasPrefix(filepath.Base(path), ".sshc-") {
		f.cleanupRemoved = true
	}
	return f.FileSystem.Remove(path)
}

type blockingStateFileSystem struct {
	storage.FileSystem
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (f *blockingStateFileSystem) Rename(oldPath, newPath string) error {
	f.once.Do(func() { close(f.entered) })
	<-f.release
	return f.FileSystem.Rename(oldPath, newPath)
}

func (f *stateFaultFileSystem) Lstat(path string) (fs.FileInfo, error) {
	if f.lstatErr != nil {
		return nil, f.lstatErr
	}
	return f.FileSystem.Lstat(path)
}

func (f *stateFaultFileSystem) Rename(oldPath, newPath string) error {
	if f.renameErr != nil {
		return f.renameErr
	}
	return f.FileSystem.Rename(oldPath, newPath)
}

func newServiceWithStateFaults(t *testing.T) (*secret.Service, *stateFaultFileSystem) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	fileSystem := &stateFaultFileSystem{FileSystem: storage.OSFileSystem{}}
	workspace, err := storage.NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	return secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now), fileSystem
}

func TestStateReportsExistenceAndUnlockTogether(t *testing.T) {
	service, _ := newService(t)

	state, err := service.State()
	if err != nil || state.Exists || state.Unlocked {
		t.Fatalf("new state = %+v, %v", state, err)
	}
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	state, err = service.State()
	if err != nil || !state.Exists || !state.Unlocked {
		t.Fatalf("initialised state = %+v, %v", state, err)
	}
	service.Lock()
	state, err = service.State()
	if err != nil || !state.Exists || state.Unlocked {
		t.Fatalf("locked state = %+v, %v", state, err)
	}
}

func TestStateReturnsStorageErrors(t *testing.T) {
	service, fileSystem := newServiceWithStateFaults(t)
	want := errors.New("lstat failed without secret details")
	fileSystem.lstatErr = want

	if _, err := service.State(); !errors.Is(err, want) {
		t.Fatalf("State error = %v, want storage error", err)
	}
}

func TestStateCannotReportUnlockedWhenTheVaultFileIsMissing(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(vaultPath(home)); err != nil {
		t.Fatal(err)
	}

	state, err := service.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.Exists || state.Unlocked {
		t.Fatalf("state after external removal = %+v, want missing and locked", state)
	}
	if service.Unlocked() {
		t.Fatal("missing vault remained unlocked in memory")
	}
}

func TestFailedInitialiseCannotLeaveAnUnlockedMissingVault(t *testing.T) {
	service, fileSystem := newServiceWithStateFaults(t)
	fileSystem.renameErr = errors.New("commit failed")
	if err := service.Initialise(passphrase); err == nil {
		t.Fatal("Initialise succeeded through a failing commit")
	}
	fileSystem.renameErr = nil

	state, err := service.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.Exists || state.Unlocked {
		t.Fatalf("failed initialise state = %+v, want missing and locked", state)
	}
}

func TestStateWaitsForInitialiseToPublishDiskAndMemoryTogether(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	fileSystem := &blockingStateFileSystem{
		FileSystem: storage.OSFileSystem{},
		entered:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	workspace, err := storage.NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	service := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	initialised := make(chan error, 1)
	go func() { initialised <- service.Initialise(passphrase) }()
	<-fileSystem.entered

	stateDone := make(chan struct {
		state secret.State
		err   error
	}, 1)
	go func() {
		state, err := service.State()
		stateDone <- struct {
			state secret.State
			err   error
		}{state: state, err: err}
	}()
	select {
	case got := <-stateDone:
		t.Fatalf("State observed an in-progress initialise: %+v, %v", got.state, got.err)
	case <-time.After(100 * time.Millisecond):
	}

	close(fileSystem.release)
	if err := <-initialised; err != nil {
		t.Fatal(err)
	}
	got := <-stateDone
	if got.err != nil || !got.state.Exists || !got.state.Unlocked {
		t.Fatalf("published state = %+v, %v", got.state, got.err)
	}
}

func TestNothingIsReadableUntilTheVaultIsUnlocked(t *testing.T) {
	service, _ := newService(t)

	if service.Unlocked() {
		t.Fatal("a new service reports itself unlocked")
	}
	if err := setTestPassword(service, "bastion", "hunter2"); !errors.Is(err, secret.ErrLocked) {
		t.Errorf("Set while locked = %v, want ErrLocked", err)
	}
	if err := service.Remove("bastion"); !errors.Is(err, secret.ErrLocked) {
		t.Errorf("Remove while locked = %v, want ErrLocked", err)
	}
	if service.Has("bastion") {
		t.Error("Has reported true while locked")
	}
	if service.Aliases() != nil {
		t.Error("Aliases returned something while locked")
	}
}

func TestInitialiseWritesASealedFileAndUnlockReadsItBack(t *testing.T) {
	service, home := newService(t)

	if err := service.Initialise(passphrase); err != nil {
		t.Fatalf("Initialise = %v", err)
	}
	if err := setTestPassword(service, "bastion", "hunter2"); err != nil {
		t.Fatalf("Set = %v", err)
	}

	sealed, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatalf("the vault was not written: %v", err)
	}
	if strings.Contains(string(sealed), "hunter2") || strings.Contains(string(sealed), "bastion") {
		t.Error("the written file contains the password or the alias in clear")
	}

	// 同じワークスペースに対する二つ目のサービスは、アプリケーションの二度目の実行で
	// あり、それが重要な場合である。
	reopened := mustReopen(t, home)
	if err := reopened.Unlock(passphrase); err != nil {
		t.Fatalf("Unlock = %v", err)
	}
	if !reopened.Has("bastion") {
		t.Error("the reopened vault has no password for bastion")
	}
}

func mustReopen(t *testing.T, home string) *secret.Service {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	return secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
}

func TestInitialiseRefusesToReplaceAnExistingVault(t *testing.T) {
	// 置き換えれば保存済みのパスワードがすべて破壊されるし、鍵が失われた暗号化
	// ファイルに復旧の道はない。
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(service, "bastion", "hunter2"); err != nil {
		t.Fatal(err)
	}

	second := mustReopen(t, home)
	if err := second.Initialise("a completely different passphrase"); !errors.Is(err, secret.ErrAlreadyExists) {
		t.Fatalf("Initialise = %v, want ErrAlreadyExists", err)
	}

	third := mustReopen(t, home)
	if err := third.Unlock(passphrase); err != nil {
		t.Fatalf("the original vault no longer opens: %v", err)
	}
	if !third.Has("bastion") {
		t.Error("the stored password is gone")
	}
}

func TestUnlockReportsNoVaultRatherThanAWrongPassphrase(t *testing.T) {
	service, _ := newService(t)
	if err := service.Unlock(passphrase); !errors.Is(err, secret.ErrNoVault) {
		t.Fatalf("Unlock = %v, want ErrNoVault", err)
	}
}

func TestUnlockRefusesTheWrongPassphraseAndStaysLocked(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}

	second := mustReopen(t, home)
	if err := second.Unlock(passphrase + "x"); !errors.Is(err, secret.ErrWrongPassphrase) {
		t.Fatalf("Unlock = %v, want ErrWrongPassphrase", err)
	}
	if second.Unlocked() {
		t.Error("a failed unlock left the service unlocked")
	}
}

func TestRemoveWritesTheVaultBack(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(service, "bastion", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := service.Remove("bastion"); err != nil {
		t.Fatalf("Remove = %v", err)
	}

	reopened := mustReopen(t, home)
	if err := reopened.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	if reopened.Has("bastion") {
		t.Error("the password came back after a restart")
	}
}

func TestRenameCarriesThePasswordThroughAWrite(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(service, "bastion", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := service.Rename("bastion", "edge"); err != nil {
		t.Fatalf("Rename = %v", err)
	}
	if err := service.Rename("absent", "elsewhere"); err != nil {
		t.Errorf("renaming a host with no password = %v, want nil", err)
	}

	reopened := mustReopen(t, home)
	if err := reopened.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	if reopened.Has("bastion") || !reopened.Has("edge") {
		t.Errorf("aliases after rename = %#v", reopened.Aliases())
	}
}

// vault は他のすべてのファイルと同じく世代を保持し、そのどれもが
// 読めない。
//
// 以前はひとつも保持していなかった。パスワードストアの古いコピーが残されるのは
// 誰も望まないだろう、という理屈である。その代償が取り消しだった。事故で壊れた
// vault には、戻る先が何もなかったのである。いまバックアップはこの vault 自身の
// 鍵で暗号化されているので、古い世代が明かすものは、現行ファイルのコピーが明かす
// もの以上ではない。
func TestTheVaultKeepsGenerationsAndNoneOfThemIsReadable(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	for _, password := range []string{"first", "second", "third"} {
		if err := setTestPassword(service, "bastion", password); err != nil {
			t.Fatal(err)
		}
	}

	backups := filepath.Join(home, ".ssh", "sshc", "backups")
	found := 0
	_ = filepath.Walk(backups, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil //nolint:nilerr // バックアップディレクトリがなければ下で失敗する
		}
		if !strings.Contains(filepath.ToSlash(path), "secrets") {
			return nil
		}
		found++
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("read %s: %v", path, readErr)
			return nil
		}
		for _, plain := range []string{"first", "second", "bastion", passphrase} {
			if strings.Contains(string(contents), plain) {
				t.Errorf("%s carries %q in the clear", path, plain)
			}
		}
		return nil
	})
	if found == 0 {
		t.Error("the vault kept no generation, so an accident to it cannot be undone")
	}
}

// サービスの資格情報まわりの面。すべての画面とすべてのルートが通る場所である。
// ロックされた vault は空リストではなく ErrLocked を返す。「見えない」と
// 「存在しない」は別の事実だからだ。
func TestCredentialsThroughTheService(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}

	if err := service.SetCredential(secret.KindPassword, "office", "s3cret"); err != nil {
		t.Fatalf("SetCredential = %v", err)
	}
	if err := assignTestPasswordCredential(service, "web-1", "office"); err != nil {
		t.Fatalf("AssignCredential = %v", err)
	}
	if err := assignTestPasswordCredential(service, "web-2", "office"); err != nil {
		t.Fatal(err)
	}

	listed, err := service.Credentials()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := listed[secret.KindPassword]["office"]
	if !ok || !slices.Equal(entry, []string{"web-1", "web-2"}) {
		t.Fatalf("credentials = %#v, want office used by both", listed)
	}

	// 名前の要点そのもの。エントリはひとつ、ローテーションは一度、対象は両方のマシン。
	if err := service.SetCredential(secret.KindPassword, "office", "rotated"); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"web-1", "web-2"} {
		if got := testPasswordFor(service, alias); got != "rotated" {
			t.Errorf("%s reads %q after one rotation", alias, got)
		}
	}

	if err := service.DeleteCredential(secret.KindPassword, "office"); !errors.Is(err, secret.ErrCredentialInUse) {
		t.Errorf("DeleteCredential of a used name = %v, want ErrCredentialInUse", err)
	}

	service.Lock()
	if _, err := service.Credentials(); !errors.Is(err, secret.ErrLocked) {
		t.Errorf("Credentials while locked = %v, want ErrLocked", err)
	}
	if err := service.SetCredential(secret.KindPassword, "x", "y"); !errors.Is(err, secret.ErrLocked) {
		t.Errorf("SetCredential while locked = %v, want ErrLocked", err)
	}
}

func TestUpdateCredentialRenamesAndReplacesAccountPasswordWithoutLosingAssignments(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office", "first-secret"); err != nil {
		t.Fatal(err)
	}
	if err := assignTestPasswordCredential(service, "web-1", "office"); err != nil {
		t.Fatal(err)
	}

	if err := service.UpdateCredential(secret.KindPassword, "office", "shared-office", "second-secret"); err != nil {
		t.Fatalf("UpdateCredential = %v", err)
	}
	if _, err := service.Credential(secret.KindPassword, "office"); !errors.Is(err, secret.ErrUnknownCredential) {
		t.Fatalf("old Credential = %v, want ErrUnknownCredential", err)
	}
	if got, err := service.Credential(secret.KindPassword, "shared-office"); err != nil || got != "second-secret" {
		t.Fatalf("new Credential = %q, %v", got, err)
	}
	if got := service.BoundPasswordFor("web-1", testAuthenticationBinding); got != "second-secret" {
		t.Fatalf("assigned password = %q", got)
	}

	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	reopened := secret.NewService(workspace, storage.NewManager(workspace, time.Now, rand.Reader), time.Now)
	if err := reopened.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.Credential(secret.KindPassword, "shared-office"); err != nil || got != "second-secret" {
		t.Fatalf("persisted Credential = %q, %v", got, err)
	}
}

func TestUpdateCredentialRenamesAKeyPassphraseAndRefusesToOverwriteAnotherName(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindKeyPassphrase, "team", "first-phrase"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindKeyPassphrase, "existing", "must-survive"); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignCredential(secret.KindKeyPassphrase, "keys/id_team", "team"); err != nil {
		t.Fatal(err)
	}

	if err := service.UpdateCredential(secret.KindKeyPassphrase, "team", "existing", "replacement"); !errors.Is(err, secret.ErrCredentialAlreadyExists) {
		t.Fatalf("colliding UpdateCredential = %v", err)
	}
	if got, _ := service.Credential(secret.KindKeyPassphrase, "team"); got != "first-phrase" {
		t.Fatalf("source changed after refusal: %q", got)
	}
	if got, _ := service.Credential(secret.KindKeyPassphrase, "existing"); got != "must-survive" {
		t.Fatalf("destination changed after refusal: %q", got)
	}

	if err := service.UpdateCredential(secret.KindKeyPassphrase, "team", "build", "second-phrase"); err != nil {
		t.Fatal(err)
	}
	if got, ok := service.KeyPassphraseFor("keys/id_team"); !ok || got != "second-phrase" {
		t.Fatalf("assigned key passphrase = %q, %t", got, ok)
	}
}

// 分離を、今度はサービスで確かめる。ルートが到達するのは vault 直接ではなく
// ここだからである。
func TestTheServiceWillNotCrossTheNamespaces(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	_ = service.SetCredential(secret.KindKeyPassphrase, "build", "phrase")

	if err := assignTestPasswordCredential(service, "web-1", "build"); err == nil {
		t.Error("a host was pointed at a key passphrase through the service")
	}
}

func TestKeyPassphraseRelocationPersists(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindKeyPassphrase, "work", "phrase"); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignCredential(secret.KindKeyPassphrase, "keys/work/id_work", "work"); err != nil {
		t.Fatal(err)
	}
	if err := service.RelocateKeyPassphrases(map[string]string{
		"keys/work/id_work": "keys/client-a/id_work",
	}); err != nil {
		t.Fatalf("RelocateKeyPassphrases = %v", err)
	}

	reopened := mustReopen(t, home)
	if err := reopened.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.KeyPassphraseFor("keys/work/id_work"); ok {
		t.Error("the old key path still resolves")
	}
	if value, ok := reopened.KeyPassphraseFor("keys/client-a/id_work"); !ok || value != "phrase" {
		t.Errorf("relocated key passphrase = %q, %v", value, ok)
	}
}

// オブジェクトストアの設定は同じマスターパスワードで暗号化され、vault の中ではなく
// 隣に置かれる。vault は移動する、remotesync.Collect は sshc/secrets を明示的に
// 指定する。バケットへの鍵をバケット内に保存してはならない。
func TestSyncSettingsAreSealedBesideTheVaultAndNotInIt(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}

	settings := secret.SyncSettings{
		Endpoint: "https://s3.example", Bucket: "b", Region: "auto",
		AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "s3cret-key", Direction: "both",
	}
	if err := service.SetSyncSettings(settings); err != nil {
		t.Fatalf("SetSyncSettings = %v", err)
	}

	read, err := service.SyncSettings()
	if err != nil {
		t.Fatalf("SyncSettings = %v", err)
	}
	if read != settings {
		t.Errorf("settings = %#v, want %#v", read, settings)
	}

	// vault の中にはなく、どちらのファイルからも読めない。
	vault, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatal(err)
	}
	sealedSettings, err := os.ReadFile(filepath.Join(home, ".ssh", filepath.FromSlash(secret.SettingsPath)))
	if err != nil {
		t.Fatalf("the settings file is not there: %v", err)
	}
	for _, absent := range []string{"AKIAEXAMPLE", "s3cret-key", "s3.example"} {
		if strings.Contains(string(vault), absent) {
			t.Errorf("the vault carries %q", absent)
		}
		if strings.Contains(string(sealedSettings), absent) {
			t.Errorf("the settings file carries %q in the clear", absent)
		}
	}
}

// 一度も設定されていないことは失敗ではなく状態である。設定を与えられていない
// マシンはゼロ値を返すので、画面はエラーではなく空のフォームを表示
// できる。
func TestSyncSettingsAnswerEmptyBeforeTheyAreEverSet(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}

	settings, err := service.SyncSettings()
	if err != nil {
		t.Fatalf("SyncSettings = %v", err)
	}
	if settings != (secret.SyncSettings{}) {
		t.Errorf("settings = %#v, want the zero value", settings)
	}
}

func TestSyncSettingsRefuseAShutVault(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	service.Lock()

	if _, err := service.SyncSettings(); !errors.Is(err, secret.ErrLocked) {
		t.Errorf("SyncSettings while locked = %v, want ErrLocked", err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{Bucket: "b"}); !errors.Is(err, secret.ErrLocked) {
		t.Errorf("SetSyncSettings while locked = %v, want ErrLocked", err)
	}
}

// プロセスの寿命のあいだ開いたままの vault は、ノートパソコンが鞄の中にあるあいだも
// 開いている vault である。起動時には何も尋ねないので、閉じることの代償は次に使う
// ときのマスターパスワード 1 回であり、開いたままにすることが及ぶ範囲は、すべての
// パスワードとすべての鍵のパスフレーズである。
func TestAVaultLeftUntouchedShutsItself(t *testing.T) {
	clock := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	service, _ := newClockedService(t, func() time.Time { return clock })
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(service, "bastion", "hunter2"); err != nil {
		t.Fatal(err)
	}

	clock = clock.Add(secret.IdleTimeout + time.Minute)
	if service.Unlocked() {
		t.Error("the vault is still open after a whole idle day")
	}
	if service.HasKeyPassphrase("id_ed25519") {
		t.Error("a locked vault still answered about a stored key passphrase")
	}
	// そして再び開けるのはマスターパスワードであって、単に尋ねることではない。
	if err := service.Unlock(passphrase); err != nil {
		t.Fatalf("Unlock = %v", err)
	}
	if !service.Has("bastion") {
		t.Error("the reopened vault lost what it held")
	}
}

func TestUsingASecretPutsTheClockBackToZero(t *testing.T) {
	clock := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	service, _ := newClockedService(t, func() time.Time { return clock })
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(service, "bastion", "hunter2"); err != nil {
		t.Fatal(err)
	}

	// 4 分の 3 まで進んだところを 4 回。使われている 1 日は、合計でどれだけ経とうと
	// アイドルな 1 日ではない。
	for range 4 {
		clock = clock.Add(secret.IdleTimeout - secret.IdleTimeout/4)
		if got := testPasswordFor(service, "bastion"); got != "hunter2" {
			t.Fatalf("PasswordFor = %q after %v, want the password", got, clock)
		}
	}
	if !service.Unlocked() {
		t.Error("a vault used all day shut itself anyway")
	}
}

// 開いたブラウザタブは、画面がマウントされるたびにステータスを読む。それが使用と
// 数えられるなら、忘れられたタブひとつが、マシンの電源が入っているあいだじゅう
// vault を開いたままにしてしまう。タイムアウトはまさにそれを止めるためにある。
func TestReadingTheStatusDoesNotHoldTheVaultOpen(t *testing.T) {
	clock := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	service, _ := newClockedService(t, func() time.Time { return clock })
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(service, "bastion", "hunter2"); err != nil {
		t.Fatal(err)
	}

	for range 4 {
		clock = clock.Add(secret.IdleTimeout - secret.IdleTimeout/4)
		if _, err := service.State(); err != nil {
			t.Fatal(err)
		}
		service.Unlocked()
		service.Aliases()
		service.Has("bastion")
	}
	if service.Unlocked() {
		t.Error("polling the status held the vault open")
	}
}

// Verify は「これはマスターパスワードか」に、何も変えずに返す。
//
// スナップショットが二つ目のパスワードではなくマスターパスワードを使えるのは、
// これのおかげだ。打ち込まれたパスワードは、鍵として使う前に検査できる。だから
// 打ち間違いは、誰にも開けないアーカイブではなく、ここでの拒否になる。
func TestVerifyAnswersWhetherThatIsTheMasterPassword(t *testing.T) {
	service, _ := newService(t)
	if _, err := service.Verify(passphrase); !errors.Is(err, secret.ErrNoVault) {
		t.Errorf("Verify with no vault = %v, want ErrNoVault", err)
	}

	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	ok, err := service.Verify(passphrase)
	if err != nil || !ok {
		t.Errorf("Verify with the right password = %v, %v", ok, err)
	}
	ok, err = service.Verify("not the master password at all")
	if err != nil || ok {
		t.Errorf("Verify with the wrong password = %v, %v, want false and no error", ok, err)
	}

	// そしてファイルから返すので、閉じた vault にも尋ねられる。尋ねる画面は、
	// vault が閉じていると告げられたばかりの画面である。
	service.Lock()
	if ok, err := service.Verify(passphrase); err != nil || !ok {
		t.Errorf("Verify on a locked vault = %v, %v", ok, err)
	}
}

// すべての世代バックアップは暗号文であり、それを開くのが vault である。
//
// 秘密鍵のバックアップは、以前はその鍵のコピーが ~/.ssh/sshc/backups/ に置かれる
// ことを意味していた。だからこそ、それを生みうる書き込みはバックアップをまったく
// 求めず、その結果、決して取り消せなかった。暗号化することが、その取り消しを買い
// 戻している。
func TestBackupsAreSealedWithTheMasterPasswordAndOpenedWithIt(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}

	plain := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nnot really\n")
	sealed, err := service.SealBackup(plain)
	if err != nil {
		t.Fatalf("SealBackup = %v", err)
	}
	if bytes.Contains(sealed, []byte("BEGIN OPENSSH")) {
		t.Error("the sealed backup carries the key in the clear")
	}

	opened, err := service.OpenBackup(sealed)
	if err != nil {
		t.Fatalf("OpenBackup = %v", err)
	}
	if !bytes.Equal(opened, plain) {
		t.Errorf("OpenBackup returned %q", opened)
	}

	// 閉じた vault は何も暗号化せず、何も開かない。アプリケーションがマスターパスワードの
	// 後ろにあるのは、まさに何かが書かれている最中にこれが起きないようにするためで
	// あり、平文で書く代わりに大きな音を立てて失敗する。
	service.Lock()
	if _, err := service.SealBackup(plain); !errors.Is(err, secret.ErrLocked) {
		t.Errorf("SealBackup while shut = %v, want ErrLocked", err)
	}
	if _, err := service.OpenBackup(sealed); !errors.Is(err, secret.ErrLocked) {
		t.Errorf("OpenBackup while shut = %v, want ErrLocked", err)
	}
}

// マスターパスワードの変更は、古いパスワードが保持していたすべてを暗号化し直す。
//
// vault も、暗号化された同期設定も、すべての世代バックアップも、そこから導出された
// 鍵で暗号化されている。したがって vault だけを置き換える変更は、残りを、もう誰も
// 使わないパスワードで開ける状態のまま残す、それは失うのと同じ
// ことだ。
func TestChangingTheMasterPasswordReSealsTheVaultTheSettingsAndTheBackups(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office-vm", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{Bucket: "b", AccessKeyID: "AKID"}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{Bucket: "b", AccessKeyID: "AKID-previous"}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{Bucket: "b", AccessKeyID: "AKID"}); err != nil {
		t.Fatal(err)
	}
	// 二度目の書き込み。これにより、暗号化し直すべき vault の世代バックアップが存在する。
	if err := service.SetCredential(secret.KindPassword, "office-vm", "hunter3"); err != nil {
		t.Fatal(err)
	}

	const next = "a different master password"
	if err := service.ChangeMasterPassword(passphrase, next); err != nil {
		t.Fatalf("ChangeMasterPassword = %v", err)
	}

	// 古いものは何も開かず、新しいものはすべてを開く。
	reopened := mustReopen(t, home)
	if err := reopened.Unlock(passphrase); !errors.Is(err, secret.ErrWrongPassphrase) {
		t.Errorf("the old password still opens the vault: %v", err)
	}
	if err := reopened.Unlock(next); err != nil {
		t.Fatalf("the new password does not open the vault: %v", err)
	}
	listed, err := reopened.Credentials()
	if err != nil {
		t.Fatalf("Credentials = %v", err)
	}
	if _, ok := listed[secret.KindPassword]["office-vm"]; !ok {
		t.Errorf("the credential did not survive: %#v", listed)
	}
	settings, err := reopened.SyncSettings()
	if err != nil || settings.AccessKeyID != "AKID" {
		t.Errorf("the settings did not survive: %+v, %v", settings, err)
	}

	// そしてすべてのバックアップが新しいもので開く。
	backups := filepath.Join(home, ".ssh", "sshc", "backups")
	found := 0
	keyBound := map[string]int{}
	if err := filepath.Walk(backups, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || info.IsDir() {
			return nil //nolint:nilerr // ディレクトリがなければ下のカウントで失敗する
		}
		found++
		sealed, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		opened, openErr := reopened.OpenBackup(sealed)
		if openErr != nil {
			t.Errorf("%s cannot be opened with the new master password: %v", path, openErr)
			return nil
		}
		for _, original := range []string{secret.WorkspacePath, secret.SettingsPath} {
			if !strings.HasSuffix(filepath.Clean(path), filepath.FromSlash(original)) {
				continue
			}
			keyBound[original]++
			if _, innerErr := reopened.OpenDocument(opened); innerErr != nil {
				t.Errorf("the nested %s backup still uses the old key generation: %v", original, innerErr)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if found == 0 {
		t.Error("no backup was there to re-seal, so this proved nothing")
	}
	for _, original := range []string{secret.WorkspacePath, secret.SettingsPath} {
		if keyBound[original] == 0 {
			t.Errorf("no nested %s backup was available to verify", original)
		}
	}
}

func TestChangingTheMasterPasswordAlsoSealsRegisteredApplicationDocuments(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(filepath.Join(home, ".ssh"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "sshc", "snippets.json")
	plain := []byte(`{"schemaVersion":1,"snippets":[{"command":"deploy --token=top-secret"}]}`)
	if err := service.RegisterProtectedDocument(secret.ProtectedDocument{
		Path: path,
		Validate: func(document []byte) error {
			if !bytes.Contains(document, []byte(`"schemaVersion":1`)) {
				return errors.New("invalid protected document")
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	// A legacy plaintext document can exist before its screen is first opened.
	acltest.WritePrivateFile(t, path, plain)
	sealedPath := filepath.Join(root, "sshc", "snippets-secondary")
	if err := service.RegisterProtectedDocument(secret.ProtectedDocument{
		Path: sealedPath, Validate: func(document []byte) error {
			if !bytes.Equal(document, plain) {
				return errors.New("invalid protected document")
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	oldSealed, err := service.SealDocument(plain)
	if err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, sealedPath, oldSealed)
	backupPath := filepath.Join(root, "sshc", "snippets-with-backup")
	if err := service.RegisterProtectedDocument(secret.ProtectedDocument{
		Path: backupPath, Validate: func(document []byte) error {
			if !bytes.Contains(document, []byte(`"schemaVersion":1`)) {
				return errors.New("invalid protected document")
			}
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, backupPath, oldSealed)
	changedPlain := []byte(`{"schemaVersion":1,"snippets":[{"command":"deploy safely"}]}`)
	changedSealed, err := service.SealDocument(changedPlain)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	manager.Seal = service.SealBackup
	manager.Unseal = service.OpenBackup
	result, err := manager.Commit(storage.Request{
		Operation: "test.update-protected-document",
		Changes: []storage.Change{{
			Path: backupPath, Contents: changedSealed,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(oldSealed)},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	protectedBackup := filepath.Join(result.BackupDir, "sshc", "snippets-with-backup")

	const next = "a different master password"
	if err := service.ChangeMasterPassword(passphrase, next); err != nil {
		t.Fatal(err)
	}
	sealed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("top-secret")) {
		t.Fatal("registered document remained plaintext after master password rotation")
	}
	service.Lock()
	if err := service.Unlock(next); err != nil {
		t.Fatal(err)
	}
	opened, err := service.OpenDocument(sealed)
	if err != nil || !bytes.Equal(opened, plain) {
		t.Fatalf("OpenDocument = %q, %v", opened, err)
	}
	resealed, err := os.ReadFile(sealedPath)
	if err != nil {
		t.Fatal(err)
	}
	opened, err = service.OpenDocument(resealed)
	if err != nil || !bytes.Equal(opened, plain) {
		t.Fatalf("OpenDocument(resealed) = %q, %v", opened, err)
	}
	backupOuter, err := os.ReadFile(protectedBackup)
	if err != nil {
		t.Fatal(err)
	}
	backupInner, err := service.OpenBackup(backupOuter)
	if err != nil {
		t.Fatalf("OpenBackup(protected document) = %v", err)
	}
	opened, err = service.OpenDocument(backupInner)
	if err != nil || !bytes.Equal(opened, plain) {
		t.Fatalf("nested protected document backup was not rekeyed: %q, %v", opened, err)
	}
}

func TestChangingTheMasterPasswordIsAtomicWhenAnyBackupCannotBeOpened(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office-vm", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{Bucket: "b", AccessKeyID: "AKID"}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office-vm", "hunter3"); err != nil {
		t.Fatal(err)
	}

	backupRoot := filepath.Join(home, ".ssh", "sshc", storage.BackupDirectoryName)
	corrupt := ""
	if err := filepath.Walk(backupRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if corrupt == "" && info != nil && !info.IsDir() {
			corrupt = path
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if corrupt == "" {
		t.Fatal("no backup was available to corrupt")
	}
	if err := os.WriteFile(corrupt, []byte("not a current encrypted backup"), 0o600); err != nil {
		t.Fatal(err)
	}

	before := map[string][]byte{}
	for _, path := range []string{
		vaultPath(home),
		filepath.Join(home, ".ssh", filepath.FromSlash(secret.SettingsPath)),
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		before[path] = body
	}
	if err := filepath.Walk(backupRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info == nil || info.IsDir() {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		before[path] = body
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	const next = "a different master password"
	if err := service.ChangeMasterPassword(passphrase, next); err == nil {
		t.Fatal("ChangeMasterPassword accepted an unreadable backup")
	}
	for path, want := range before {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s changed after the refused rotation", path)
		}
	}
	afterPaths := map[string]bool{}
	if err := filepath.Walk(backupRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info != nil && !info.IsDir() {
			afterPaths[path] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for path := range before {
		if strings.Contains(path, string(filepath.Separator)+storage.BackupDirectoryName+string(filepath.Separator)) && !afterPaths[path] {
			t.Errorf("backup %s disappeared after the refused rotation", path)
		}
	}

	service.Lock()
	if err := service.Unlock(passphrase); err != nil {
		t.Fatalf("the old password no longer opens the unchanged vault: %v", err)
	}
	service.Lock()
	if err := service.Unlock(next); !errors.Is(err, secret.ErrWrongPassphrase) {
		t.Fatalf("the refused new password opens the vault: %v", err)
	}
}

func newRekeyFaultHarness(t *testing.T) (*secret.Service, *storage.Manager, *rekeyFaultFileSystem, string, map[string][]byte) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	faults := &rekeyFaultFileSystem{FileSystem: storage.OSFileSystem{}, failure: errors.New("injected rekey durability failure")}
	workspace, err := storage.NewWorkspace(faults, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := secret.NewService(workspace, manager, time.Now)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office-vm", "first password"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{Bucket: "bucket", AccessKeyID: "account"}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office-vm", "second password"); err != nil {
		t.Fatal(err)
	}

	// Windows の EvalSymlinks は drive letter と各 path component の大小文字を
	// canonical form へ直す。障害注入対象は呼び出し元の home 表記ではなく、実際に
	// transaction が使用する解決済み workspace path で保持する。
	targets := map[string][]byte{}
	for _, path := range []string{
		filepath.Join(workspace.Root(), filepath.FromSlash(secret.WorkspacePath)),
		filepath.Join(workspace.Root(), filepath.FromSlash(secret.SettingsPath)),
	} {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		targets[path] = body
	}
	backupRoot := filepath.Join(workspace.StateDir(), storage.BackupDirectoryName)
	if err := filepath.Walk(backupRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info != nil && !info.IsDir() {
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			targets[path] = body
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	faults.targets = map[string]bool{}
	for path := range targets {
		faults.targets[path] = true
	}
	return service, manager, faults, home, targets
}

func assertRekeyGeneration(t *testing.T, service *secret.Service, targets map[string][]byte, oldPassword, newPassword string) {
	t.Helper()
	for path, want := range targets {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read target after rekey failure: %v", err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("target %s did not return to its old generation", path)
		}
	}
	service.Lock()
	if err := service.Unlock(oldPassword); err != nil {
		t.Errorf("old password no longer opens the running vault: %v", err)
	}
	service.Lock()
	if err := service.Unlock(newPassword); !errors.Is(err, secret.ErrWrongPassphrase) {
		t.Errorf("failed new password opens the running vault: %v", err)
	}
}

func TestChangingTheMasterPasswordRollsBackEveryTargetAfterApplyFailure(t *testing.T) {
	const next = "a different master password"
	for _, test := range []struct {
		name      string
		configure func(*rekeyFaultFileSystem)
	}{
		{name: "rename", configure: func(f *rekeyFaultFileSystem) { f.failRenames = map[int]bool{2: true} }},
		{name: "directory sync", configure: func(f *rekeyFaultFileSystem) { f.failTargetSyncAt = 2 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, _, faults, _, before := newRekeyFaultHarness(t)
			test.configure(faults)
			if err := service.ChangeMasterPassword(passphrase, next); !errors.Is(err, faults.failure) {
				t.Fatalf("ChangeMasterPassword = %v, want injected failure", err)
			}
			assertRekeyGeneration(t, service, before, passphrase, next)
		})
	}
}

func TestChangingTheMasterPasswordRecoversTheOldGenerationAfterProcessCrash(t *testing.T) {
	const next = "a different master password"
	service, manager, faults, home, before := newRekeyFaultHarness(t)
	// The first failure interrupts forward apply; the second interrupts the
	// automatic rollback, leaving exactly the state a process crash would expose.
	faults.failRenames = map[int]bool{2: true, 3: true}
	if err := service.ChangeMasterPassword(passphrase, next); !errors.Is(err, faults.failure) {
		t.Fatalf("ChangeMasterPassword = %v, want injected failure", err)
	}
	pending, err := manager.Pending()
	if err != nil || len(pending) != 1 || !pending[0].CanRollback {
		t.Fatalf("Pending = %#v, %v", pending, err)
	}

	// A fresh manager represents restart. Raw rollback material needs no live
	// vault callback, so recovery can restore the old generation first.
	faults.failRenames = nil
	workspace, err := storage.NewWorkspace(faults, home)
	if err != nil {
		t.Fatal(err)
	}
	restartedManager := storage.NewManager(workspace, time.Now, rand.Reader)
	if err := restartedManager.Rollback(pending[0].ID); err != nil {
		t.Fatalf("Rollback after restart = %v", err)
	}
	restarted := secret.NewService(workspace, restartedManager, time.Now)
	assertRekeyGeneration(t, restarted, before, passphrase, next)
}

func TestChangingTheMasterPasswordFinalizesTheNewGenerationAfterCommitPointCrash(t *testing.T) {
	const next = "a different master password"
	service, manager, faults, home, _ := newRekeyFaultHarness(t)
	faults.failCleanupSync = true
	if err := service.ChangeMasterPassword(passphrase, next); err != nil {
		t.Fatalf("ChangeMasterPassword = %v", err)
	}
	service.Lock()
	if err := service.Unlock(next); err != nil {
		t.Fatalf("new password does not open running vault: %v", err)
	}
	pending, err := manager.Pending()
	if err != nil || len(pending) != 1 || !pending[0].CanComplete || pending[0].CanRollback {
		t.Fatalf("Pending after commit point = %#v, %v", pending, err)
	}

	workspace, err := storage.NewWorkspace(faults, home)
	if err != nil {
		t.Fatal(err)
	}
	restartedManager := storage.NewManager(workspace, time.Now, rand.Reader)
	if err := restartedManager.Complete(pending[0].ID); err != nil {
		t.Fatalf("Complete after restart = %v", err)
	}
	restarted := secret.NewService(workspace, restartedManager, time.Now)
	if err := restarted.Unlock(passphrase); !errors.Is(err, secret.ErrWrongPassphrase) {
		t.Fatalf("old password opens committed generation: %v", err)
	}
	if err := restarted.Unlock(next); err != nil {
		t.Fatalf("new password does not open committed generation: %v", err)
	}
}

func TestChangingTheMasterPasswordRefusesTheWrongCurrentOne(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.ChangeMasterPassword("not the master password", "a new master password"); !errors.Is(err, secret.ErrWrongPassphrase) {
		t.Errorf("ChangeMasterPassword with the wrong current = %v, want ErrWrongPassphrase", err)
	}
	// そして、それが持っていた鍵でいまも開く。
	if err := service.Unlock(passphrase); err != nil {
		t.Errorf("the vault was disturbed by a refused change: %v", err)
	}
}

// 誤った推測は、だんだん遅くなる。
//
// vault ファイルはコピーしてオフラインで攻撃できるので、これは攻撃者と中身の
// あいだに立つものではない、それは Argon2id である。これが止めるのは安価な場合、
// すなわち、動作中のアプリケーションに対して、判定できる限りの速さでパスワードを
// 試すローカルのプロセスである。
func TestWrongMasterPasswordsAreAnsweredMoreSlowly(t *testing.T) {
	clock := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	var waited []time.Duration
	service, _ := newClockedService(t, func() time.Time { return clock })
	service.SetSleep(func(d time.Duration) { waited = append(waited, d) })
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}

	for range 4 {
		if err := service.Unlock("not the master password"); !errors.Is(err, secret.ErrWrongPassphrase) {
			t.Fatalf("Unlock = %v", err)
		}
	}
	if len(waited) != 4 {
		t.Fatalf("waits = %v, want one per refusal", waited)
	}
	// 各拒否は前回より長く待つ。上限まで。
	for index := 1; index < len(waited); index++ {
		if waited[index] < waited[index-1] {
			t.Errorf("wait %d (%v) is shorter than wait %d (%v)", index, waited[index], index-1, waited[index-1])
		}
	}
	if waited[len(waited)-1] > secret.MaxUnlockDelay {
		t.Errorf("the wait grew past its ceiling: %v", waited[len(waited)-1])
	}

	// 正しいパスワードは即座に受理され、誤入力による遅延をリセットする。
	waited = nil
	if err := service.Unlock(passphrase); err != nil {
		t.Fatalf("Unlock with the right password = %v", err)
	}
	if len(waited) != 0 {
		t.Errorf("a correct password waited: %v", waited)
	}
}

func TestPasswordMutationsCommitEachSupportedSource(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office", "shared-secret"); err != nil {
		t.Fatal(err)
	}

	commit := func(change storage.Change) (storage.Result, error) {
		workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
		if err != nil {
			return storage.Result{}, err
		}
		manager := storage.NewManager(workspace, time.Now, rand.Reader)
		return manager.Commit(storage.Request{Operation: "test.password-mutation", Changes: []storage.Change{change}})
	}
	cases := []struct {
		name     string
		mutation secret.PasswordMutation
		want     string
	}{
		{
			name: "dedicated",
			mutation: secret.PasswordMutation{
				Kind: secret.PasswordMutationDedicated, Binding: testAuthenticationBinding, Alias: "edge-1", Password: "connection-only",
			},
			want: "connection-only",
		},
		{
			name: "saved reusable",
			mutation: secret.PasswordMutation{
				Kind: secret.PasswordMutationSaved, Binding: testAuthenticationBinding, Alias: "edge-2", Credential: "office",
			},
			want: "shared-secret",
		},
		{
			name: "new reusable",
			mutation: secret.PasswordMutation{
				Kind: secret.PasswordMutationNewShared, Binding: testAuthenticationBinding, Alias: "edge-3", Credential: "lab", Password: "lab-secret",
			},
			want: "lab-secret",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, err := service.WithPasswordMutation(test.mutation, commit)
			if err != nil {
				t.Fatalf("WithPasswordMutation = %v", err)
			}
			if result.ID == "" {
				t.Error("commit returned no transaction ID")
			}
			if got := testPasswordFor(service, test.mutation.Alias); got != test.want {
				t.Errorf("PasswordFor(%s) = %q, want %q", test.mutation.Alias, got, test.want)
			}
		})
	}

	listed, err := service.Credentials()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := listed[secret.KindPassword]["edge-1"]; ok {
		t.Error("the dedicated password appeared in reusable credentials")
	}
	if uses := listed[secret.KindPassword]["lab"]; !slices.Equal(uses, []string{"edge-3"}) {
		t.Errorf("new shared credential uses = %#v", uses)
	}

	reopened := mustReopen(t, home)
	if err := reopened.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		if got := testPasswordFor(reopened, test.mutation.Alias); got != test.want {
			t.Errorf("reopened PasswordFor(%s) = %q, want %q", test.mutation.Alias, got, test.want)
		}
	}
}

func TestPasswordMutationRemoveDeletesDedicatedAndOnlyUnassignsReusable(t *testing.T) {
	tests := []struct {
		name    string
		alias   string
		prepare func(*testing.T, *secret.Service)
		verify  func(*testing.T, *secret.Service)
	}{
		{
			name:  "dedicated",
			alias: "edge-dedicated",
			prepare: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if err := setTestPassword(service, "edge-dedicated", "connection-only"); err != nil {
					t.Fatal(err)
				}
			},
			verify: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if got := testPasswordFor(service, "edge-dedicated"); got != "" {
					t.Errorf("dedicated password survived removal: %q", got)
				}
			},
		},
		{
			name:  "reusable",
			alias: "edge-shared",
			prepare: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if err := service.SetCredential(secret.KindPassword, "office", "shared-secret"); err != nil {
					t.Fatal(err)
				}
				if err := assignTestPasswordCredential(service, "edge-shared", "office"); err != nil {
					t.Fatal(err)
				}
			},
			verify: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if got := testPasswordFor(service, "edge-shared"); got != "" {
					t.Errorf("shared assignment survived removal: %q", got)
				}
				listed, err := service.Credentials()
				if err != nil {
					t.Fatal(err)
				}
				uses, exists := listed[secret.KindPassword]["office"]
				if !exists || len(uses) != 0 {
					t.Errorf("shared credential after unassign = %#v, exists %t", uses, exists)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, home := newService(t)
			if err := service.Initialise(passphrase); err != nil {
				t.Fatal(err)
			}
			test.prepare(t, service)
			workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
			if err != nil {
				t.Fatal(err)
			}
			manager := storage.NewManager(workspace, time.Now, rand.Reader)
			_, err = service.WithPasswordMutation(secret.PasswordMutation{
				Kind: secret.PasswordMutationRemove, Alias: test.alias,
			}, func(change storage.Change) (storage.Result, error) {
				return manager.Commit(storage.Request{
					Operation: "test.password-remove", Changes: []storage.Change{change},
				})
			})
			if err != nil {
				t.Fatalf("WithPasswordMutation(remove) = %v", err)
			}
			test.verify(t, service)
			reopened := mustReopen(t, home)
			if err := reopened.Unlock(passphrase); err != nil {
				t.Fatal(err)
			}
			test.verify(t, reopened)
		})
	}
}

func TestPasswordMutationRemovePreservesAnUnrelatedAliasNamedCredential(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "edge", "unrelated-secret"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office", "assigned-secret"); err != nil {
		t.Fatal(err)
	}
	if err := assignTestPasswordCredential(service, "edge", "office"); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	_, err = service.WithPasswordMutation(secret.PasswordMutation{
		Kind: secret.PasswordMutationRemove, Alias: "edge",
	}, func(change storage.Change) (storage.Result, error) {
		return manager.Commit(storage.Request{Operation: "test.password-remove", Changes: []storage.Change{change}})
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := service.Credentials()
	if err != nil {
		t.Fatal(err)
	}
	if uses, exists := listed[secret.KindPassword]["edge"]; !exists || len(uses) != 0 {
		t.Fatalf("unrelated alias-named credential = %#v, exists %t", uses, exists)
	}
}

func TestPasswordMutationNewSharedRefusesAnExistingCredential(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office", "must-survive"); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err := service.WithPasswordMutation(secret.PasswordMutation{
		Kind: secret.PasswordMutationNewShared, Binding: testAuthenticationBinding, Alias: "edge", Credential: "office", Password: "replacement",
	}, func(change storage.Change) (storage.Result, error) {
		called = true
		return storage.Result{}, nil
	})
	if !errors.Is(err, secret.ErrCredentialAlreadyExists) {
		t.Fatalf("existing new-shared mutation = %v, want ErrCredentialAlreadyExists", err)
	}
	if called {
		t.Error("commit callback ran for a colliding credential")
	}
	if err := assignTestPasswordCredential(service, "probe", "office"); err != nil {
		t.Fatal(err)
	}
	if got := testPasswordFor(service, "probe"); got != "must-survive" {
		t.Fatalf("existing credential was overwritten: %q", got)
	}
}

func TestAccountPasswordAssignmentsRequireAuthenticationBinding(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office", "shared"); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignCredential(secret.KindPassword, "edge", "office"); !errors.Is(err, secret.ErrPasswordBindingRequired) {
		t.Fatalf("AssignCredential(password) = %v, want ErrPasswordBindingRequired", err)
	}

	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	called := false
	_, err = service.WithPasswordMutation(secret.PasswordMutation{
		Kind: secret.PasswordMutationSaved, Alias: "edge", Credential: "office",
	}, func(change storage.Change) (storage.Result, error) {
		called = true
		return manager.Commit(storage.Request{Operation: "test.unbound-password", Changes: []storage.Change{change}})
	})
	if !errors.Is(err, secret.ErrPasswordBindingRequired) {
		t.Fatalf("WithPasswordMutation(unbound) = %v, want ErrPasswordBindingRequired", err)
	}
	if called || service.Has("edge") {
		t.Fatal("unbound password mutation changed the vault")
	}
}

func TestPasswordMutationRejectsSemanticNoOps(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, *secret.Service)
		mutation secret.PasswordMutation
	}{
		{
			name: "same dedicated password",
			prepare: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if err := setTestPassword(service, "edge", "unchanged"); err != nil {
					t.Fatal(err)
				}
			},
			mutation: secret.PasswordMutation{Kind: secret.PasswordMutationDedicated, Binding: testAuthenticationBinding, Alias: "edge", Password: "unchanged"},
		},
		{
			name: "same reusable assignment",
			prepare: func(t *testing.T, service *secret.Service) {
				t.Helper()
				if err := service.SetCredential(secret.KindPassword, "office", "shared"); err != nil {
					t.Fatal(err)
				}
				if err := assignTestPasswordCredential(service, "edge", "office"); err != nil {
					t.Fatal(err)
				}
			},
			mutation: secret.PasswordMutation{Kind: secret.PasswordMutationSaved, Binding: testAuthenticationBinding, Alias: "edge", Credential: "office"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _ := newService(t)
			if err := service.Initialise(passphrase); err != nil {
				t.Fatal(err)
			}
			test.prepare(t, service)
			called := false
			_, err := service.WithPasswordMutation(test.mutation, func(change storage.Change) (storage.Result, error) {
				called = true
				return storage.Result{}, nil
			})
			if !errors.Is(err, secret.ErrNoPasswordMutation) {
				t.Fatalf("WithPasswordMutation = %v, want ErrNoPasswordMutation", err)
			}
			if called {
				t.Error("commit callback ran for a semantic no-op")
			}
		})
	}
}

func TestPasswordMutationRemoveRejectsAnUnassignedAlias(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err := service.WithPasswordMutation(secret.PasswordMutation{
		Kind: secret.PasswordMutationRemove, Alias: "absent",
	}, func(change storage.Change) (storage.Result, error) {
		called = true
		return storage.Result{}, nil
	})
	if !errors.Is(err, secret.ErrNoPassword) {
		t.Fatalf("WithPasswordMutation(remove absent) = %v, want ErrNoPassword", err)
	}
	if called {
		t.Error("remove callback ran for an unassigned alias")
	}
}

func TestFailedPasswordRemovalPublishesNeitherMemoryNorDisk(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(service, "edge", "must-survive"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("commit refused")

	_, err = service.WithPasswordMutation(secret.PasswordMutation{
		Kind: secret.PasswordMutationRemove, Alias: "edge",
	}, func(storage.Change) (storage.Result, error) {
		return storage.Result{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithPasswordMutation(remove) = %v, want commit error", err)
	}
	if got := testPasswordFor(service, "edge"); got != "must-survive" {
		t.Errorf("failed removal changed memory: %q", got)
	}
	after, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Error("failed removal changed the sealed vault on disk")
	}
}

func TestFailedPasswordMutationPublishesNeitherMemoryNorDisk(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("commit refused")

	_, err = service.WithPasswordMutation(secret.PasswordMutation{
		Kind: secret.PasswordMutationDedicated, Binding: testAuthenticationBinding, Alias: "edge", Password: "must-not-survive",
	}, func(change storage.Change) (storage.Result, error) {
		if bytes.Contains(change.Contents, []byte("must-not-survive")) {
			t.Error("the staged encrypted vault contains the password in clear")
		}
		return storage.Result{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithPasswordMutation = %v, want commit error", err)
	}
	if got := testPasswordFor(service, "edge"); got != "" {
		t.Errorf("failed mutation was published in memory: %q", got)
	}
	after, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Error("failed mutation changed the sealed vault on disk")
	}
}

func TestPasswordMutationCanCommitAConfigBackupWithoutDeadlocking(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := secret.NewService(workspace, manager, time.Now)
	manager.Seal = service.SealBackup
	manager.Unseal = service.OpenBackup
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(workspace.Root(), "config")
	if err := os.WriteFile(configPath, []byte("Host old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, mutationErr := service.WithPasswordMutation(secret.PasswordMutation{
			Kind: secret.PasswordMutationDedicated, Binding: testAuthenticationBinding, Alias: "edge", Password: "connection-only",
		}, func(vaultChange storage.Change) (storage.Result, error) {
			return manager.Commit(storage.Request{
				Operation: "test.connection-create",
				Changes: []storage.Change{
					{
						Path: configPath, Contents: []byte("Host old\nHost edge\n"),
						Precondition: storage.Precondition{Exists: true, Digest: storage.Digest([]byte("Host old\n"))},
					},
					vaultChange,
				},
			})
		})
		done <- mutationErr
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WithPasswordMutation = %v", err)
		}
	// Windows runnerではvaultの鍵導出とsealが数秒掛かる。ここで検査したいのは
	// lock循環による停止であり、暗号化の処理時間ではないため十分な上限を置く。
	case <-time.After(15 * time.Second):
		t.Fatal("password mutation deadlocked while storage sealed the config backup")
	}
}

func TestPasswordMutationUsesTheRekeyedVaultAsItsBaseline(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	const nextPassphrase = "the next correct horse battery staple"
	if err := service.ChangeMasterPassword(passphrase, nextPassphrase); err != nil {
		t.Fatalf("ChangeMasterPassword = %v", err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)

	_, err = service.WithPasswordMutation(secret.PasswordMutation{
		Kind: secret.PasswordMutationDedicated, Binding: testAuthenticationBinding, Alias: "edge", Password: "after-rekey",
	}, func(change storage.Change) (storage.Result, error) {
		return manager.Commit(storage.Request{Operation: "test.after-rekey", Changes: []storage.Change{change}})
	})
	if err != nil {
		t.Fatalf("WithPasswordMutation after rekey = %v", err)
	}

	reopened := mustReopen(t, home)
	if err := reopened.Unlock(nextPassphrase); err != nil {
		t.Fatal(err)
	}
	if got := testPasswordFor(reopened, "edge"); got != "after-rekey" {
		t.Errorf("reopened password = %q", got)
	}
}

func TestConnectionSecretsMutationCommitsDedicatedKeyPassphraseWithoutChangingSharedUsers(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindKeyPassphrase, "team", "shared-old"); err != nil {
		t.Fatal(err)
	}
	for _, subject := range []string{"keys/id_a", "keys/id_b"} {
		if err := service.AssignCredential(secret.KindKeyPassphrase, subject, "team"); err != nil {
			t.Fatal(err)
		}
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	callbackCalls := 0
	_, err = service.WithConnectionSecretsMutation(secret.ConnectionSecretsMutation{
		KeyPassphrase: &secret.KeyPassphraseMutation{RelativePath: "keys/id_a", Passphrase: "dedicated-new"},
	}, func(change storage.Change) (storage.Result, error) {
		callbackCalls++
		if bytes.Contains(change.Contents, []byte("dedicated-new")) {
			t.Error("the staged vault contains the passphrase in clear")
		}
		return manager.Commit(storage.Request{Operation: "test.connection-secrets", Changes: []storage.Change{change}})
	})
	if err != nil {
		t.Fatalf("WithConnectionSecretsMutation = %v", err)
	}
	if callbackCalls != 1 {
		t.Fatalf("commit callback calls = %d, want 1", callbackCalls)
	}
	if got, ok := service.KeyPassphraseFor("keys/id_a"); !ok || got != "dedicated-new" {
		t.Fatalf("id_a = %q, %v", got, ok)
	}
	if got, ok := service.KeyPassphraseFor("keys/id_b"); !ok || got != "shared-old" {
		t.Fatalf("id_b = %q, %v; shared user changed", got, ok)
	}
	if got := service.DedicatedKeyPassphrases(); !slices.Equal(got, []string{"keys/id_a"}) {
		t.Fatalf("DedicatedKeyPassphrases = %#v", got)
	}

	reopened := mustReopen(t, home)
	if err := reopened.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	if got, ok := reopened.KeyPassphraseFor("keys/id_a"); !ok || got != "dedicated-new" {
		t.Fatalf("reopened id_a = %q, %v", got, ok)
	}
}

func TestConnectionSecretsMutationSealsPasswordAndKeyTogether(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	callbackCalls := 0
	_, err = service.WithConnectionSecretsMutation(secret.ConnectionSecretsMutation{
		Password: &secret.PasswordMutation{
			Kind: secret.PasswordMutationDedicated, Binding: testAuthenticationBinding, Alias: "edge", Password: "account-secret",
		},
		KeyPassphrase: &secret.KeyPassphraseMutation{
			RelativePath: "keys/id_edge", Passphrase: "key-secret",
		},
	}, func(change storage.Change) (storage.Result, error) {
		callbackCalls++
		return manager.Commit(storage.Request{Operation: "test.connection-secrets", Changes: []storage.Change{change}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if callbackCalls != 1 {
		t.Fatalf("commit callback calls = %d, want one sealed write", callbackCalls)
	}
	if got := testPasswordFor(service, "edge"); got != "account-secret" {
		t.Fatalf("account password = %q", got)
	}
	if got, ok := service.KeyPassphraseFor("keys/id_edge"); !ok || got != "key-secret" {
		t.Fatalf("key passphrase = %q, %v", got, ok)
	}
}

func TestConnectionSecretsMutationRejectsSameDedicatedKeyPassphrase(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	mutation := secret.ConnectionSecretsMutation{KeyPassphrase: &secret.KeyPassphraseMutation{
		RelativePath: "keys/id_a", Passphrase: "unchanged",
	}}
	if _, err := service.WithConnectionSecretsMutation(mutation, func(change storage.Change) (storage.Result, error) {
		return manager.Commit(storage.Request{Operation: "test.connection-secrets", Changes: []storage.Change{change}})
	}); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err = service.WithConnectionSecretsMutation(mutation, func(storage.Change) (storage.Result, error) {
		called = true
		return storage.Result{}, nil
	})
	if !errors.Is(err, secret.ErrNoPasswordMutation) {
		t.Fatalf("same value mutation = %v, want ErrNoPasswordMutation", err)
	}
	if called {
		t.Error("commit callback ran for a semantic no-op")
	}
}

func TestConnectionSecretsTransactionCommitsConfigWhenPasswordRemovalIsANoop(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	result, err := service.WithConnectionSecretsTransaction(secret.ConnectionSecretsMutation{
		Password: &secret.PasswordMutation{Kind: secret.PasswordMutationRemove, Alias: "edge"},
	}, func(change *storage.Change) (storage.Result, error) {
		called++
		if change != nil {
			t.Fatal("absent password produced a vault change")
		}
		return storage.Result{ID: "config-only"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if called != 1 || result.ID != "config-only" {
		t.Fatalf("callback calls/result = %d / %#v", called, result)
	}
	after, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Error("no-op removal resealed the vault")
	}
}

func TestConnectionSecretsTransactionKeepsAReusableCredentialWhenRemovingOneUse(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetCredential(secret.KindPassword, "office", "shared-secret"); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"edge", "nas"} {
		if err := assignTestPasswordCredential(service, alias, "office"); err != nil {
			t.Fatal(err)
		}
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	_, err = service.WithConnectionSecretsTransaction(secret.ConnectionSecretsMutation{
		Password: &secret.PasswordMutation{Kind: secret.PasswordMutationRemove, Alias: "edge"},
	}, func(change *storage.Change) (storage.Result, error) {
		if change == nil {
			t.Fatal("assigned password produced no vault change")
		}
		return manager.Commit(storage.Request{Operation: "test.cleanup", Changes: []storage.Change{*change}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := testPasswordFor(service, "edge"); got != "" {
		t.Errorf("removed edge password = %q", got)
	}
	if got := testPasswordFor(service, "nas"); got != "shared-secret" {
		t.Errorf("other shared user = %q", got)
	}
	listed, err := service.Credentials()
	if err != nil {
		t.Fatal(err)
	}
	if uses, ok := listed[secret.KindPassword]["office"]; !ok || !slices.Equal(uses, []string{"nas"}) {
		t.Fatalf("office credential after cleanup = %#v, exists %t", uses, ok)
	}
}

func TestConnectionSecretsTransactionSerializesAWouldBeNoopWithVaultWriters(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	transactionDone := make(chan error, 1)
	go func() {
		_, err := service.WithConnectionSecretsTransaction(secret.ConnectionSecretsMutation{
			Password: &secret.PasswordMutation{Kind: secret.PasswordMutationRemove, Alias: "edge"},
		}, func(change *storage.Change) (storage.Result, error) {
			if change != nil {
				return storage.Result{}, errors.New("unexpected vault change")
			}
			close(entered)
			<-release
			return storage.Result{}, nil
		})
		transactionDone <- err
	}()
	<-entered

	writerDone := make(chan error, 1)
	go func() { writerDone <- setTestPassword(service, "edge", "arrived-later") }()
	select {
	case err := <-writerDone:
		t.Fatalf("writer overtook transaction: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-transactionDone; err != nil {
		t.Fatal(err)
	}
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	if got := testPasswordFor(service, "edge"); got != "arrived-later" {
		t.Fatalf("serialized writer value = %q", got)
	}
}

func TestConnectionSecretsMutationRequiresAnUnlockedExistingVault(t *testing.T) {
	service, _ := newService(t)
	called := false
	_, err := service.WithConnectionSecretsMutation(secret.ConnectionSecretsMutation{
		KeyPassphrase: &secret.KeyPassphraseMutation{RelativePath: "keys/id_a", Passphrase: "secret"},
	}, func(storage.Change) (storage.Result, error) {
		called = true
		return storage.Result{}, nil
	})
	if !errors.Is(err, secret.ErrNoVault) {
		t.Fatalf("missing-vault mutation = %v, want ErrNoVault", err)
	}
	if called {
		t.Error("commit callback ran while locked")
	}
	if got := service.DedicatedKeyPassphrases(); got != nil {
		t.Fatalf("locked dedicated subjects = %#v, want nil", got)
	}
}

func TestConnectionSecretsMutationFailurePublishesNeitherKeyMemoryNorDisk(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("commit refused")
	_, err = service.WithConnectionSecretsMutation(secret.ConnectionSecretsMutation{
		KeyPassphrase: &secret.KeyPassphraseMutation{RelativePath: "keys/id_a", Passphrase: "must-not-survive"},
	}, func(storage.Change) (storage.Result, error) {
		return storage.Result{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithConnectionSecretsMutation = %v, want commit error", err)
	}
	if _, ok := service.KeyPassphraseFor("keys/id_a"); ok {
		t.Error("failed mutation was published in memory")
	}
	after, err := os.ReadFile(vaultPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Error("failed mutation changed the sealed vault on disk")
	}
}

func TestDedicatedKeyPassphraseRelocationPersists(t *testing.T) {
	service, home := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	_, err = service.WithConnectionSecretsMutation(secret.ConnectionSecretsMutation{
		KeyPassphrase: &secret.KeyPassphraseMutation{RelativePath: "keys/work/id_a", Passphrase: "dedicated"},
	}, func(change storage.Change) (storage.Result, error) {
		return manager.Commit(storage.Request{Operation: "test.connection-secrets", Changes: []storage.Change{change}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RelocateKeyPassphrases(map[string]string{
		"keys/work/id_a": "keys/client/id_a",
	}); err != nil {
		t.Fatal(err)
	}
	if got := service.DedicatedKeyPassphrases(); !slices.Equal(got, []string{"keys/client/id_a"}) {
		t.Fatalf("dedicated subjects after relocation = %#v", got)
	}
	reopened := mustReopen(t, home)
	if err := reopened.Unlock(passphrase); err != nil {
		t.Fatal(err)
	}
	if got, ok := reopened.KeyPassphraseFor("keys/client/id_a"); !ok || got != "dedicated" {
		t.Fatalf("reopened relocated passphrase = %q, %v", got, ok)
	}
}

// bucket を編集しただけで、リモートのスナップショットが誰にも開けなくなっては
// ならない。設定の form は鍵の欄を持たない。鍵を見せるのは作った一度だけで、
// 以後は伏せ字である。ので、空で来た鍵は「消せ」ではなく「触るな」である。
func TestSettingsWithoutAKeyKeepTheKeyThatIsAlreadyStored(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{
		Endpoint: "https://s3.example", Bucket: "b", Region: "auto",
		AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "s3cret-key", Direction: "both",
	}); err != nil {
		t.Fatalf("SetSyncSettings = %v", err)
	}
	if err := service.SetSyncKey("AB12-CD34-EF56-GH78-JK90-MN12"); err != nil {
		t.Fatalf("SetSyncKey = %v", err)
	}

	// bucket だけを編集した form が戻ってくる。
	if err := service.SetSyncSettings(secret.SyncSettings{
		Endpoint: "https://s3.example", Bucket: "other", Region: "auto",
		AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "s3cret-key", Direction: "both",
	}); err != nil {
		t.Fatalf("SetSyncSettings = %v", err)
	}

	read, err := service.SyncSettings()
	if err != nil {
		t.Fatalf("SyncSettings = %v", err)
	}
	if read.Key != "AB12-CD34-EF56-GH78-JK90-MN12" {
		t.Fatalf("the stored key is %q, want the one that was set", read.Key)
	}
	if read.Bucket != "other" {
		t.Fatalf("the bucket is %q, want the edited one", read.Bucket)
	}
}

// 鍵だけを置き換える道が、他の設定を巻き添えにしない。
func TestSettingTheKeyLeavesEveryOtherSettingAlone(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	settings := secret.SyncSettings{
		Endpoint: "https://s3.example", Bucket: "b", Path: "p", Region: "auto",
		AccessKeyID: "AKIAEXAMPLE", SecretAccessKey: "s3cret-key", Direction: "push",
	}
	if err := service.SetSyncSettings(settings); err != nil {
		t.Fatalf("SetSyncSettings = %v", err)
	}
	if err := service.SetSyncKey("AB12-CD34-EF56-GH78-JK90-MN12"); err != nil {
		t.Fatalf("SetSyncKey = %v", err)
	}

	read, err := service.SyncSettings()
	if err != nil {
		t.Fatalf("SyncSettings = %v", err)
	}
	settings.Key = "AB12-CD34-EF56-GH78-JK90-MN12"
	if read != settings {
		t.Fatalf("settings are %+v, want %+v", read, settings)
	}
}

func TestSyncSettingsCASUsesTheDocumentWhichWasMutated(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Embed the FileSystem contract rather than OSFileSystem itself. On Windows,
	// embedding the concrete type would also promote ReadPrivateFile, causing the
	// workspace's authenticated private-state reader to bypass this ReadFile fault
	// injector entirely.
	fileSystem := &syncCASFileSystem{
		FileSystem:  storage.OSFileSystem{},
		replacement: []byte("externally replaced settings\n"),
	}
	workspace, err := storage.NewWorkspace(fileSystem, home)
	if err != nil {
		t.Fatal(err)
	}
	// Use the canonical path returned by Workspace. Windows EvalSymlinks can
	// normalize drive and component case, while the caller's home keeps its
	// original spelling.
	settingsPath := filepath.Join(workspace.Root(), filepath.FromSlash(secret.SettingsPath))
	fileSystem.path = settingsPath
	manager := storage.NewManager(workspace, time.Now, rand.Reader)
	service := secret.NewService(workspace, manager, time.Now)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncSettings(secret.SyncSettings{Bucket: "first", Direction: "both"}); err != nil {
		t.Fatal(err)
	}
	fileSystem.armed = true
	fileSystem.reads = 0
	err = service.SetSyncAuto(true)
	var conflict *storage.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("SetSyncAuto = %v, want ConflictError", err)
	}
	body, readErr := os.ReadFile(settingsPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(body, fileSystem.replacement) {
		t.Fatal("the externally written settings were overwritten")
	}
}

func TestRotatedSyncKeyCannotCommitToAReconfiguredTarget(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	original := secret.SyncSettings{
		Endpoint: "https://first.example", Bucket: "first", Region: "auto",
		AccessKeyID: "first-id", SecretAccessKey: "first-secret", Direction: "both", Key: "old-key",
	}
	if err := service.SetSyncSettings(original); err != nil {
		t.Fatal(err)
	}
	changed := original
	changed.Endpoint = "https://second.example"
	changed.Bucket = "second"
	if err := service.SetSyncSettings(changed); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSyncKeyIfSettingsMatch(original, "new-key"); !errors.Is(err, secret.ErrSyncSettingsChanged) {
		t.Fatalf("SetSyncKeyIfSettingsMatch = %v, want ErrSyncSettingsChanged", err)
	}
	stored, err := service.SyncSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Bucket != "second" || stored.Key != "old-key" {
		t.Fatalf("settings = %+v", stored)
	}
}

// 保管庫が閉じているなら、鍵は書けない。書ける設計は、閉じていることの意味を
// 失わせる。
func TestSettingTheKeyNeedsAnOpenVault(t *testing.T) {
	service, _ := newService(t)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if err := service.SetSyncKey("AB12-CD34-EF56-GH78-JK90-MN12"); !errors.Is(err, secret.ErrLocked) {
		t.Fatalf("SetSyncKey on a locked vault = %v, want ErrLocked", err)
	}
}

// 巡回が保管庫を開けっぱなしにしてはならない。1 分ごとに設定を読む読み手が
// アイドルの時計を戻し続ければ、自動ロックは永久に来ない。誰も居ない机の上で、
// 鍵がプロセスの記憶に残り続けることになる。
func TestAnUnattendedReaderDoesNotKeepTheVaultOpen(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	service := secret.NewService(workspace, storage.NewManager(workspace, now, rand.Reader), now)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}

	// 時計を数字で書き写さない。書き写せば、時計を延ばした日にこの検査は
	// 何も言わないまま緑になる。実際、8 時間から延ばしたときにそうなった。
	minutes := int((secret.IdleTimeout + time.Hour).Minutes())
	for minute := 0; minute < minutes; minute++ {
		clock = clock.Add(time.Minute)
		service.Unattended(func() { _, _ = service.SyncSettings() })
	}
	if service.Unlocked() {
		t.Fatal("the vault stayed open because the loop kept reading it")
	}
}

// 逆に、ユーザーが読んだのなら時計は戻る。止めているのは呼び出し側の性質であって、
// 読んだという事実ではない。
func TestAReaderThatIsNotTheLoopKeepsTheVaultOpen(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	service := secret.NewService(workspace, storage.NewManager(workspace, now, rand.Reader), now)
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}

	for minute := 0; minute < 9*60; minute++ {
		clock = clock.Add(time.Minute)
		if _, err := service.SyncSettings(); err != nil {
			t.Fatal(err)
		}
	}
	if !service.Unlocked() {
		t.Fatal("the vault closed even though it was being used")
	}
}

// 頼まれれば、時計を待たずに閉じる。
//
// 時計はどこでも同じになったが、ロックそのものが消えたわけではない。
func TestAVaultClosesTheMomentItIsAskedTo(t *testing.T) {
	clock := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	service, _ := newClockedService(t, func() time.Time { return clock })
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(service, "bastion", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if !service.Unlocked() {
		t.Fatal("開いていない")
	}

	service.Lock()
	if service.Unlocked() {
		t.Error("頼まれても閉じなかった")
	}
}

// 使用されない vault はタイムアウトでロックする。
//
// どこで走っていても同じである。`sshc engine` は systemd の下で何週間も走り、
// 蓋も画面ロックも無い。そして窓を閉じても実行を続ける engine も、同じ状況である。
func TestAVaultLeftAloneShutsItself(t *testing.T) {
	clock := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	service, _ := newClockedService(t, func() time.Time { return clock })
	if err := service.Initialise(passphrase); err != nil {
		t.Fatal(err)
	}
	if err := setTestPassword(service, "bastion", "hunter2"); err != nil {
		t.Fatal(err)
	}

	clock = clock.Add(secret.IdleTimeout + time.Minute)
	if service.Unlocked() {
		t.Error("既定のまま開き続けた: headless には OS の境界が無い")
	}
}
