package application

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sync"

	"sshc/internal/config"
	"sshc/internal/configresolver"
	"sshc/internal/effective"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

const entryFileName = "config"

// ErrNotEditable は、設定として扱えないファイルへの操作を断る。
var ErrNotEditable = errors.New("file is not editable through this application")

type Service struct {
	workspace *storage.Workspace
	manager   *storage.Manager
	resolver  config.Resolver
	metadata  *MetadataStore
	entryPath string

	saveMutex      sync.Mutex
	keyPassphrases KeyPassphraseVerifier
	startupRenamer StartupRenamer
	startupRemover StartupRemover
	vault          *secret.Service
	// factsFor は、トークン展開と既定の GlobalKnownHostsFile に要るこのマシンの事実を読む。
	// テストは本物の /etc/ssh の known_hosts へ届かないように差し替える。
	factsFor func(home string) effective.LocalFacts
}

func resolverFor(workspace *storage.Workspace) config.Resolver {
	resolver := configresolver.ForWorkspace(workspace)
	resolver.GeneratedRegion = GeneratedRegion
	return resolver
}

func NewService(workspace *storage.Workspace, manager *storage.Manager) *Service {
	service := &Service{
		workspace: workspace,
		manager:   manager,
		resolver:  resolverFor(workspace),
		metadata:  NewMetadataStore(workspace),
		entryPath: filepath.Join(workspace.Root(), entryFileName),
		factsFor:  LocalFactsFor,
	}
	manager.Validate = service.validate
	return service
}

// SetVault は、設定と Vault を 1 つの変更として書くユースケースが使う Vault を
// 渡す。Vault のファイルがまだ無い構成でも渡す。nil のままだと、接続の作成・更新で
// Vault を書く変更は secret.ErrNoVault で、鍵のパス変更は ErrKeyPassphraseVaultMissing
// で断り、alias の改名は設定だけを書く。
//
// 渡した Vault には、このマシンの自動ロックの時間を移す。以後は、sshc エンジンの設定を
// 変える操作のたびに移し直す（applyVaultAutoLock）。
//
// 生成時ではなく setter で受けるのは、engine の組み立てで Vault がこの service のあとに
// 作られるからである。鍵の検証（SetKeyPassphraseVerifier）と起動スニペットの改名・削除
// （SetStartupRenamer・SetStartupRemover）も同じ理由で setter で受ける。鍵の一覧
// （keys.Inventory）は、要求のたびにディスクから読むスナップショットなので、呼び出しごとの
// 引数で受ける。
func (s *Service) SetVault(vault *secret.Service) {
	s.vault = vault
	s.applyVaultAutoLock()
}

func (s *Service) displayPath(absolute string) string {
	reference := NewFileRef(s.workspace.Root(), absolute)
	if reference.External {
		return reference.Absolute
	}
	return reference.Path
}

// preconditionFor は、readFile で読んだ内容を、あとの commit がディスクと比べる事前
// 条件にする。無かったファイルは、無いままであることを条件にする。
func preconditionFor(contents []byte, exists bool) storage.Precondition {
	if !exists {
		return storage.Precondition{}
	}
	return storage.Precondition{Exists: true, Digest: storage.Digest(contents)}
}

func (s *Service) readFile(absolute string) (contents []byte, exists bool, err error) {
	contents, err = s.workspace.FileSystem().ReadFile(absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return contents, true, nil
}

// localFacts は、トークン展開と既定の GlobalKnownHostsFile に要るこのプロセスの事実である。
func (s *Service) localFacts() effective.LocalFacts {
	return s.factsFor(s.workspace.Home())
}

func (s *Service) resolve() (*config.Graph, error) {
	return s.resolver.Resolve(s.entryPath)
}

func (s *Service) resolveWith(pending map[string][]byte) (*config.Graph, error) {
	resolver := s.resolver
	resolver.Loader = overlayLoader{base: s.resolver.Loader, pending: pending}
	return resolver.Resolve(s.entryPath)
}

func diskOrNil(contents []byte, exists bool) []byte {
	if !exists {
		return nil
	}
	if contents == nil {
		return []byte{}
	}
	return contents
}
