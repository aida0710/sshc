package remotesync

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"

	"sshc/internal/objectstore"
	"sshc/internal/storage"
)

// PullResult は、適用する前の、pull が行うであろう内容。
//
// 永続化トランザクションは remotesync の実装詳細であり、transport へ公開しない。
// Written と Removed は利用者へ表示できるワークスペース相対パスである。
type PullResult struct {
	Written         []string
	Removed         []string
	Conflicts       []Conflict
	Manifest        Manifest
	Summary         SnapshotSummary
	DownloadedBytes int64
	CompletedAt     string
	ETag            string
	Origin          string
	objectKey       string
	target          string
	sourceKey       string
	sourceETag      string
	bindingVersion  uint64
	localState      historyStateSnapshot
	liveMissing     bool
	request         storage.Request
}

func (s *Service) pullPaths(request storage.Request) (written, removed []string) {
	written = make([]string, 0, len(request.Changes))
	for _, change := range request.Changes {
		written = append(written, s.DisplayPath(change.Path))
	}
	removed = make([]string, 0, len(request.Removals))
	for _, removal := range request.Removals {
		removed = append(removed, s.DisplayPath(removal.Path))
	}
	return written, removed
}

// Pull はスナップショットを取得し、それを適用すると何が変わるかを算出する。
//
// 何も書かない。Apply を別の呼び出しにしてあるのは、書き込みの前に必ず見せる
// プレビューを、このアプリケーションの他の部分と同じくユーザーに見せるためである。
func (s *Service) Pull(ctx context.Context, passphrase string, resolve Resolution) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.pull(ctx, passphrase, resolve, "")
}

// PullRemoteHead previews the current live head after the user explicitly chose
// to trust it. This is the force-receive path for a legitimate force push which
// cannot be proven to descend from this installation's acknowledged revision.
// The snapshot is still authenticated and the later apply is bound to its exact
// ETag and revision. Send-only installations cannot apply any remote bytes.
func (s *Service) PullRemoteHead(ctx context.Context, passphrase string) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.pullWithRemoteAcceptance(ctx, passphrase, ResolveRemote, "", true)
}

// PullHistory previews one immutable dated snapshot. Applying it writes local
// files and records the current live ETag, but keeps the selected manifest as
// Base. The next push therefore creates a new head whose parent is the restored
// revision without unconditionally rewinding the remote object.
func (s *Service) PullHistory(ctx context.Context, passphrase, historyKey string, resolve Resolution) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.pull(ctx, passphrase, resolve, historyKey)
}

func (s *Service) pull(ctx context.Context, passphrase string, resolve Resolution, historyKey string) (PullResult, error) {
	return s.pullWithRemoteAcceptance(ctx, passphrase, resolve, historyKey, false)
}

func (s *Service) pullWithRemoteAcceptance(
	ctx context.Context,
	passphrase string,
	resolve Resolution,
	historyKey string,
	acceptRemoteHead bool,
) (PullResult, error) {
	if acceptRemoteHead && s.Direction() == DirectionPush {
		return PullResult{}, ErrApplyRefused
	}
	binding, bindingVersion, err := s.configuredBindingVersion()
	if err != nil {
		return PullResult{}, err
	}
	if err := s.ensureNoKeyRecovery(ctx, binding); err != nil {
		return PullResult{}, err
	}
	current, err := s.readState()
	if err != nil {
		return PullResult{}, err
	}
	localState := snapshotHistoryState(current)
	objectKey := ObjectKeyFor(binding.config)
	sourceKey := objectKey
	stateETag := ""
	liveMissing := false
	if historyKey != "" {
		sourceKey, err = historyObjectKey(binding.config, historyKey)
		if err != nil {
			return PullResult{}, err
		}
		live, statErr := binding.client.Stat(ctx, objectKey)
		if statErr != nil && !errors.Is(statErr, objectstore.ErrNotFound) {
			return PullResult{}, statErr
		}
		if statErr == nil {
			stateETag = live.ETag
		} else {
			liveMissing = true
		}
	}
	object, err := binding.client.Get(ctx, sourceKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return PullResult{}, ErrNoSnapshot
		}
		return PullResult{}, err
	}
	if historyKey == "" {
		stateETag = object.ETag
	}
	// この envelope の中のパラメータを選んだのは、それを書いた誰かであって、必ずしも
	// このインストールではない。別のユーザーのスナップショットが、パスフレーズの誤りが判明
	// する前にこのマシンへ 1 ギガバイトと 16 スレッドを費やさせられるようであっては
	// ならない。
	manifest, contents, err := openSnapshotObject(object, passphrase)
	if err != nil {
		return PullResult{}, err
	}
	ignoreRules, err := ignoreRulesFromSnapshot(contents)
	if err != nil {
		return PullResult{}, err
	}

	base := current.Base
	if !stateMatchesTarget(current, binding.config) {
		base = nil
	}
	// A bucket writer who does not know the synchronization key can still replay
	// an older, authentic ciphertext. Prove that a changed live head descends
	// from the locally acknowledged revision before treating it as an ordinary
	// pull. PullHistory remains the explicit rollback path.
	if historyKey == "" && !acceptRemoteHead {
		follows, lineageErr := s.liveSnapshotFollows(ctx, binding, passphrase, base, manifest, Digest(object.Body))
		if lineageErr != nil {
			return PullResult{}, lineageErr
		}
		if !follows {
			return PullResult{}, ErrRemoteMoved
		}
	}
	var local map[string]LocalEntry
	readLocal := func() error {
		var readErr error
		local, readErr = s.localDigests(manifest, base, ignoreRules)
		return readErr
	}
	if s.integrations.StableSnapshot != nil {
		err = s.integrations.StableSnapshot(readLocal)
	} else {
		err = readLocal()
	}
	if err != nil {
		return PullResult{}, err
	}

	request, conflicts, err := PlanEntriesWithIgnore(s.workspace.Root(), base, local, manifest, contents, resolve, ignoreRules.Match)
	if exchangeErr := s.stageVault(&request); exchangeErr != nil {
		return PullResult{}, exchangeErr
	}
	if err != nil && !errors.Is(err, ErrNothingToApply) {
		return PullResult{}, err
	}
	written, removed := s.pullPaths(request)
	return PullResult{
		Written: written, Removed: removed, Conflicts: conflicts, Manifest: manifest,
		Summary:         snapshotSummary(manifest, contents, len(object.Body)),
		DownloadedBytes: int64(len(object.Body)), CompletedAt: s.now(),
		ETag: stateETag, Origin: manifest.Origin, objectKey: objectKey, target: targetID(binding.config),
		sourceKey: sourceKey, sourceETag: object.ETag, bindingVersion: bindingVersion,
		localState: localState, liveMissing: liveMissing, request: request,
	}, err
}

// PullAndApply downloads, verifies and commits one exact preview generation as
// a single service operation. The expected values come from the earlier
// user-visible preview; this method then takes a fresh remote/local snapshot
// and keeps operationMu through the final workspace commit.
func (s *Service) PullAndApply(ctx context.Context, passphrase string, resolve Resolution, historyKey, expectedETag, expectedRevision string) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	return s.pullAndApply(ctx, passphrase, resolve, historyKey, expectedETag, expectedRevision, false)
}

func (s *Service) PullAndApplyUsing(ctx context.Context, key KeyProvider, resolve Resolution, historyKey, expectedETag, expectedRevision string) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	passphrase, err := currentOperationKey(key)
	if err != nil {
		return PullResult{}, err
	}
	return s.pullAndApply(ctx, passphrase, resolve, historyKey, expectedETag, expectedRevision, false)
}

// PullAndApplyRemoteHeadUsing applies only the exact live head returned by an
// earlier PullRemoteHead preview. It exists only as an explicit force-receive
// operation; ordinary pulls retain ancestry protection.
func (s *Service) PullAndApplyRemoteHeadUsing(
	ctx context.Context,
	key KeyProvider,
	expectedETag string,
	expectedRevision string,
) (PullResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	passphrase, err := currentOperationKey(key)
	if err != nil {
		return PullResult{}, err
	}
	return s.pullAndApply(ctx, passphrase, ResolveRemote, "", expectedETag, expectedRevision, true)
}

func (s *Service) pullAndApply(ctx context.Context, passphrase string, resolve Resolution, historyKey, expectedETag, expectedRevision string, acceptRemoteHead bool) (PullResult, error) {
	result, err := s.pullWithRemoteAcceptance(ctx, passphrase, resolve, historyKey, acceptRemoteHead)
	if err != nil && !errors.Is(err, ErrNothingToApply) {
		return PullResult{}, err
	}
	if expectedETag == "" || expectedRevision == "" ||
		result.ETag != expectedETag || result.Manifest.Revision != expectedRevision {
		return PullResult{}, ErrPreviewStale
	}
	if err := s.validatePullForApply(ctx, result); err != nil {
		return PullResult{}, err
	}
	if err := s.apply(result); err != nil {
		return PullResult{}, err
	}
	return result, nil
}

// Apply は pull をコミットする。どれかのファイルが衝突しているあいだは拒否する。
// 半分だけ適用すれば、どちらの側とも一致しないワークスペースになるからだ。
func (s *Service) Apply(result PullResult) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if err := s.validatePullForApply(context.Background(), result); err != nil {
		return err
	}
	return s.apply(result)
}

func (s *Service) validatePullForApply(ctx context.Context, result PullResult) error {
	if result.liveMissing || result.ETag == "" {
		return ErrRemoteDeleted
	}
	binding, version, err := s.configuredBindingVersion()
	if err != nil {
		return err
	}
	if version != result.bindingVersion || targetID(binding.config) != result.target ||
		ObjectKeyFor(binding.config) != result.objectKey {
		return ErrRemoteMoved
	}
	current, err := s.readState()
	if err != nil {
		return err
	}
	if snapshotHistoryState(current) != result.localState {
		return ErrRemoteMoved
	}
	remoteETag, err := binding.client.Head(ctx, result.objectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return ErrRemoteDeleted
		}
		return err
	}
	if remoteETag != result.ETag {
		return ErrRemoteMoved
	}
	if result.sourceKey != "" && result.sourceKey != result.objectKey {
		sourceETag, err := binding.client.Head(ctx, result.sourceKey)
		if err != nil {
			if errors.Is(err, objectstore.ErrNotFound) {
				return ErrRemoteMoved
			}
			return err
		}
		if sourceETag != result.sourceETag {
			return ErrRemoteMoved
		}
	}
	return nil
}

func (s *Service) apply(result PullResult) error {
	if s.integrations.SecretMutation != nil {
		return s.integrations.SecretMutation(func() error { return s.applyWithSecretGeneration(result) })
	}
	return s.applyWithSecretGeneration(result)
}

func (s *Service) applyWithSecretGeneration(result PullResult) error {
	// direction は Pull ではなくここで検査する。プレビューは何も書かないので、
	// 送信専用のマシンでも、どれだけ遅れているかは知ることができる。
	// これが、別のマシンのバイト列をこのディスクへ置く呼び出しである。
	if s.Direction() == DirectionPush {
		return ErrApplyRefused
	}
	if len(result.Conflicts) > 0 {
		return ErrConflicts
	}
	// PullResult is a reusable preview. Keep its logical plaintext request
	// untouched and seal a private copy only at the final commit boundary.
	result.request.Changes = slices.Clone(result.request.Changes)
	result.request.FinalChanges = slices.Clone(result.request.FinalChanges)
	result.request.Removals = slices.Clone(result.request.Removals)
	result.request.Directories = slices.Clone(result.request.Directories)
	if err := s.exchangeVault(&result.request); err != nil {
		return err
	}
	if err := s.exchangeSnippets(&result.request); err != nil {
		return err
	}
	written := len(result.request.Changes)
	removed := len(result.request.Removals)
	current, err := s.readState()
	if err != nil {
		return err
	}
	origin := current.Origin
	if origin == "" {
		if origin, err = s.newOrigin(); err != nil {
			return err
		}
	}
	manifest := result.Manifest
	operation := SyncOperation{
		Kind: OperationApply, Summary: result.Summary,
		DownloadedBytes: result.DownloadedBytes,
		Written:         written, Removed: removed,
		CompletedAt: s.now(),
	}
	stateChange, err := s.stateChange(state{
		ETag: result.ETag, Key: result.objectKey, Target: result.target, Base: &manifest, Origin: origin,
		LastOperation: &operation,
	})
	if err != nil {
		return err
	}
	if result.request.Operation == "" {
		result.request.Operation = "sync.pull"
	}
	// State is a terminal write in the same journal as the workspace files. The
	// storage transaction applies it only after every move and removal succeeds,
	// including when Complete resumes the transaction after a restart.
	result.request.FinalChanges = append(result.request.FinalChanges, stateChange)
	// 別のマシンからのスナップショットは、このマシンにはないかもしれない
	// ディレクトリ（connections/work/、keys/work/）を指定する。stateの親も含め、
	// すべて同じrequestへ載せる。
	changesWithState := append(slices.Clone(result.request.Changes), result.request.FinalChanges...)
	result.request.Directories = append(result.request.Directories,
		changeDirectories(s.workspace.Root(), changesWithState)...)
	if _, err := s.transactions.Commit(result.request); err != nil {
		return err
	}
	// 保管庫を置き換えたなら、それを配っている側に読み直させる。
	if s.integrations.VaultAdopted != nil && replacesVault(s.workspace.Root(), result.request) {
		if err := s.integrations.VaultAdopted(); err != nil {
			return err
		}
	}
	return nil
}

// exchangeSnippets maps the logical plaintext snapshot entry onto the same
// local path using this installation's master key. Previous ciphertext is not
// retained as a generation backup: snippets had no history before encryption,
// and keeping a nested old-key envelope would make password rotation unsafe.
func (s *Service) exchangeSnippets(request *storage.Request) error {
	local := filepath.Join(s.workspace.Root(), filepath.FromSlash(SnippetsPath))
	for index := range request.Changes {
		if request.Changes[index].Path != local {
			continue
		}
		if s.integrations.SealSnippets == nil {
			return ErrVaultCodec
		}
		if err := s.requireSnippetPrecondition(local, request.Changes[index].Precondition); err != nil {
			return err
		}
		sealed, err := s.integrations.SealSnippets(request.Changes[index].Contents)
		if err != nil {
			return err
		}
		precondition := storage.Precondition{}
		if body, err := s.workspace.FileSystem().ReadFile(local); err == nil {
			precondition = storage.Precondition{Exists: true, Digest: storage.Digest(body)}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		request.Changes[index] = storage.Change{
			Path: local, Contents: sealed, Precondition: precondition, SkipBackup: true,
		}
	}
	for index := range request.Removals {
		if request.Removals[index].Path != local {
			continue
		}
		if err := s.requireSnippetPrecondition(local, request.Removals[index].Precondition); err != nil {
			return err
		}
		body, err := s.workspace.FileSystem().ReadFile(local)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		request.Removals[index] = storage.Removal{
			Path: local, Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(body)},
		}
	}
	return nil
}

func (s *Service) requireSnippetPrecondition(path string, expected storage.Precondition) error {
	if s.integrations.OpenSnippets == nil {
		return ErrVaultCodec
	}
	document, err := s.integrations.OpenSnippets()
	if err != nil {
		return err
	}
	actual := ""
	if document != nil {
		actual = Digest(document)
	}
	if expected.Exists == (document != nil) && (!expected.Exists || expected.Digest == actual) {
		return nil
	}
	return &storage.ConflictError{Path: path, Expected: expected.Digest, Actual: actual}
}

// stageVault maps the travelling logical path to the local vault path while
// retaining plaintext and its logical precondition in the PullResult. Sealing
// is deliberately deferred until apply holds SecretMutation.
func (s *Service) stageVault(request *storage.Request) error {
	travelled := filepath.Join(s.workspace.Root(), filepath.FromSlash(TravelPath))
	local := filepath.Join(s.workspace.Root(), filepath.FromSlash(VaultPath))
	for index := range request.Changes {
		if request.Changes[index].Path == travelled {
			request.Changes[index].Path = local
		}
	}
	for _, removal := range request.Removals {
		if removal.Path == travelled {
			request.Changes = append(request.Changes, storage.Change{
				Path: local, Contents: nil, Precondition: removal.Precondition,
			})
		}
	}
	request.Removals = slices.DeleteFunc(request.Removals, func(removal storage.Removal) bool { return removal.Path == travelled })
	sort.Slice(request.Changes, func(i, j int) bool { return request.Changes[i].Path < request.Changes[j].Path })
	return nil
}

// exchangeVault validates the logical preview against the current unlocked
// vault, then seals it with the exact master-key generation held by apply.
func (s *Service) exchangeVault(request *storage.Request) error {
	if s.integrations.SealVault == nil {
		return ErrVaultCodec
	}
	local := filepath.Join(s.workspace.Root(), filepath.FromSlash(VaultPath))
	for index := range request.Changes {
		if request.Changes[index].Path != local {
			continue
		}
		if err := s.requireVaultPrecondition(local, request.Changes[index].Precondition); err != nil {
			return err
		}
		var sealed []byte
		var err error
		if len(request.Changes[index].Contents) == 0 {
			if s.integrations.EmptyVaultDocument == nil {
				return ErrVaultCodec
			}
			var empty []byte
			empty, err = s.integrations.EmptyVaultDocument()
			if err == nil {
				sealed, err = s.integrations.SealVault(empty)
			}
		} else {
			sealed, err = s.integrations.SealVault(request.Changes[index].Contents)
		}
		if err != nil {
			return err
		}
		precondition := storage.Precondition{}
		if body, err := s.workspace.FileSystem().ReadFile(local); err == nil {
			precondition = storage.Precondition{Exists: true, Digest: storage.Digest(body)}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		request.Changes[index] = storage.Change{
			Path: local, Contents: sealed, Precondition: precondition,
		}
	}
	return nil
}

func (s *Service) requireVaultPrecondition(path string, expected storage.Precondition) error {
	if s.integrations.OpenVault == nil {
		return ErrVaultCodec
	}
	document, err := s.integrations.OpenVault()
	if err != nil {
		return err
	}
	actual := Digest(document)
	if (!expected.Exists && document == nil) || (expected.Exists && expected.Digest == actual) {
		return nil
	}
	return &storage.ConflictError{Path: path, Expected: expected.Digest, Actual: actual}
}

// replacesVault は、このリクエストが保管庫のファイルを書き換えるかを返す。
func replacesVault(root string, request storage.Request) bool {
	vault := filepath.Join(root, filepath.FromSlash(VaultPath))
	for _, change := range request.Changes {
		if change.Path == vault {
			return true
		}
	}
	return false
}

// diverged は、このディスクが最後に同期したものと違うかを返す。
// 自動巡回がoperationMuを保持したまま「押し出すものがあるか」を判断する内部操作
// であり、この判断にHTTPは1本も要らない。
func (s *Service) diverged() (bool, error) {
	manifest, _, err := s.Collect()
	if err != nil {
		return false, err
	}
	current, err := s.readState()
	if err != nil {
		return false, err
	}
	if current.Base == nil {
		// 一度も同期していない。載せるものがあるなら、それは違いである。
		return len(manifest.Files) > 0, nil
	}
	binding, err := s.configuredBinding()
	if err != nil {
		return false, err
	}
	if !stateMatchesTarget(current, binding.config) {
		return len(manifest.Files) > 0, nil
	}
	return manifestChanged(current.Base, manifest), nil
}

type remoteGeneration struct {
	moved   bool
	etag    string
	deleted bool
	target  string
}

// inspectRemoteGeneration はHEADでETagだけを確認し、ライブオブジェクトが最後に同期した
// 世代から変わったかを返す。一度確認済みのliveが消えた場合も変更として扱い、空の
// bucketと区別する。呼び出し側はoperationMuを保持する。
func (s *Service) inspectRemoteGeneration(ctx context.Context) (remoteGeneration, error) {
	binding, err := s.configuredBinding()
	if err != nil {
		return remoteGeneration{}, err
	}
	current, err := s.readState()
	if err != nil {
		return remoteGeneration{}, err
	}
	objectKey := ObjectKeyFor(binding.config)
	target := targetID(binding.config)
	etag, err := binding.client.Head(ctx, objectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			if stateMatchesTarget(current, binding.config) && current.ETag != "" {
				return remoteGeneration{moved: true, deleted: true, target: target}, nil
			}
			// まだ誰も置いていない。受け取るものは無い。
			return remoteGeneration{target: target}, nil
		}
		return remoteGeneration{}, err
	}
	// 別のオブジェクトの世代は、このオブジェクトについて何も語らない。
	if !stateMatchesTarget(current, binding.config) {
		return remoteGeneration{moved: true, etag: etag, target: target}, nil
	}
	return remoteGeneration{moved: etag != current.ETag, etag: etag, target: target}, nil
}
