package remotesync

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"sshc/internal/envelope"
	"sshc/internal/objectstore"
)

func (s *Service) PushUsing(ctx context.Context, key KeyProvider, message string) (PushResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	// Preserve the public Push contract for a vault that has no remote target:
	// configuration is the first prerequisite, before a synchronization key.
	if _, err := s.configuredBinding(); err != nil {
		return PushResult{}, err
	}
	passphrase, err := currentOperationKey(key)
	if err != nil {
		return PushResult{}, err
	}
	return s.push(ctx, passphrase, "", message)
}

func (s *Service) ForcePushUsing(ctx context.Context, key KeyProvider, confirmation ForcePushConfirmation, message string) (PushResult, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	binding, err := s.validateForcePushBinding(confirmation)
	if err != nil {
		return PushResult{}, err
	}
	passphrase, err := currentOperationKey(key)
	if err != nil {
		return PushResult{}, err
	}
	if err := validateForcePushGeneration(ctx, binding, confirmation); err != nil {
		return PushResult{}, err
	}
	return s.push(ctx, passphrase, confirmation.ETag, message)
}

func (s *Service) validateForcePushBinding(confirmation ForcePushConfirmation) (remoteBinding, error) {
	if confirmation.ETag == "" || confirmation.bindingVersion == 0 || confirmation.targetID == "" ||
		confirmation.Evidence != forcePushEvidence(confirmation.bindingVersion, confirmation.targetID, confirmation.ETag) {
		return remoteBinding{}, ErrForcePushTarget
	}
	binding, version, err := s.configuredBindingVersion()
	if err != nil {
		return remoteBinding{}, err
	}
	if version != confirmation.bindingVersion || targetID(binding.config) != confirmation.targetID {
		return remoteBinding{}, ErrRemoteMoved
	}
	return binding, nil
}

func validateForcePushGeneration(ctx context.Context, binding remoteBinding, confirmation ForcePushConfirmation) error {
	etag, err := binding.client.Head(ctx, ObjectKeyFor(binding.config))
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return ErrRemoteMoved
		}
		return err
	}
	if etag != confirmation.ETag {
		return ErrRemoteMoved
	}
	return nil
}

func currentOperationKey(provider KeyProvider) (string, error) {
	if provider == nil {
		return "", errors.New("synchronization key provider is not configured")
	}
	key, err := provider()
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", ErrWeakPassphrase
	}
	return key, nil
}

func (s *Service) push(ctx context.Context, passphrase, forcedETag, message string) (PushResult, error) {
	binding, err := s.configuredBinding()
	if err != nil {
		return PushResult{}, err
	}
	if err := s.ensureNoKeyRecovery(ctx, binding); err != nil {
		return PushResult{}, err
	}
	if binding.config.Direction == DirectionPull {
		return PushResult{}, ErrPushRefused
	}
	client := binding.client
	current, err := s.readState()
	if err != nil {
		return PushResult{}, err
	}
	if current.Origin == "" {
		if current.Origin, err = s.newOrigin(); err != nil {
			return PushResult{}, err
		}
	}

	manifest, contents, err := s.Collect()
	if err != nil {
		return PushResult{}, err
	}
	manifest.Origin = current.Origin
	objectKey := ObjectKeyFor(binding.config)
	sameTarget := stateMatchesTarget(current, binding.config)
	if !sameTarget {
		// 別のremote headに保存されたbaseを、新しいbucketの親として記録しない。
		current.ETag = ""
		current.Base = nil
	}
	if forcedETag == "" && sameTarget && current.Base != nil && !manifestChanged(current.Base, manifest) {
		return PushResult{}, ErrNothingToPush
	}
	parentRevision := ""
	if current.Base != nil {
		parentRevision = current.Base.Revision
		if parentRevision == "" {
			parentRevision, err = RevisionFor(*current.Base)
			if err != nil {
				return PushResult{}, err
			}
		}
	}
	manifest.Ancestors = manifestAncestors(current.Base)
	if strings.TrimSpace(message) == "" {
		message = draftFor(current.Base, manifest).Message
	}
	manifest.Message = message
	if err := FinalizeManifest(&manifest, parentRevision); err != nil {
		return PushResult{}, err
	}
	archive, err := Build(manifest, contents)
	if err != nil {
		return PushResult{}, err
	}
	key, err := envelope.Derive(passphrase)
	if err != nil {
		return PushResult{}, err
	}
	sealed, err := key.Seal(archive)
	key.Destroy()
	if err != nil {
		return PushResult{}, err
	}
	result := PushResult{Summary: snapshotSummary(manifest, contents, len(sealed))}

	ifMatch, ifNoneMatch := forcedETag, ""
	if ifMatch == "" {
		ifMatch = current.ETag
		if ifMatch == "" {
			ifNoneMatch = "*"
		}
	}
	// 日付付きの候補が先。それが失敗すればライブは更新しない。ライブの条件付き
	// 書き込みが競争に負けたことを確定できた場合は、この候補だけを削除する。
	s.historySeq++
	dated, err := snapshotKeyFor(binding.config, manifest.CreatedAt, current.Origin, sealed, s.historySeq)
	if err != nil {
		return result, err
	}
	if _, err := client.Put(ctx, dated, sealed, "", "*"); err != nil {
		return result, err
	}
	result.ObjectCount++
	result.UploadedBytes += int64(len(sealed))

	etag, err := client.Put(ctx, objectKey, sealed, ifMatch, ifNoneMatch)
	if err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			if cleanupErr := client.Delete(ctx, dated); cleanupErr != nil {
				return result, errors.Join(ErrRemoteMoved, fmt.Errorf("remove the rejected history candidate: %w", cleanupErr))
			}
			return result, ErrRemoteMoved
		}
		return result, err
	}
	result.ObjectCount++
	result.UploadedBytes += int64(len(sealed))
	result.CompletedAt = s.now()
	operation := SyncOperation{
		Kind: OperationPush, Summary: result.Summary,
		ObjectCount: result.ObjectCount, UploadedBytes: result.UploadedBytes,
		CompletedAt: result.CompletedAt,
	}
	if err := s.writeState(state{
		ETag: etag, Key: objectKey, Target: targetID(binding.config), Base: &manifest, Origin: current.Origin,
		LastOperation: &operation,
	}); err != nil {
		return result, err
	}
	return result, nil
}

// PushDraft returns the same generated message used by unattended pushes.
func (s *Service) PushDraft() (PushDraft, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	manifest, _, err := s.Collect()
	if err != nil {
		return PushDraft{}, err
	}
	current, err := s.readState()
	if err != nil {
		return PushDraft{}, err
	}
	binding, err := s.configuredBinding()
	if err != nil {
		return PushDraft{}, err
	}
	base := current.Base
	if !stateMatchesTarget(current, binding.config) {
		base = nil
	}
	return draftFor(base, manifest), nil
}

func (s *Service) liveSnapshotFollows(
	ctx context.Context,
	binding remoteBinding,
	passphrase string,
	base *Manifest,
	incoming Manifest,
	incomingCiphertextDigest string,
) (bool, error) {
	if base == nil {
		return true, nil
	}
	if incoming.Revision == base.Revision || incoming.ParentRevision == base.Revision {
		return true, nil
	}
	if slices.Contains(incoming.Ancestors, base.Revision) {
		return true, nil
	}
	infos, _, err := binding.client.ListNewest(
		ctx, joinKey(binding.config.Path, SnapshotPrefix), maxHistoryGraphRevisions,
	)
	if err != nil {
		return false, err
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].LastModified.Equal(infos[j].LastModified) {
			return infos[i].Key > infos[j].Key
		}
		return infos[i].LastModified.After(infos[j].LastModified)
	})
	manifests := make(map[string]Manifest, len(infos))
	type cachedOpen struct {
		manifest Manifest
		valid    bool
	}
	opened := make(map[string]cachedOpen, len(infos)+1)
	// Push publishes the exact same sealed bytes to the dated and live keys.
	// Seed the cache with the live object which pull already authenticated, so
	// encountering its immutable copy does not spend a second KDF attempt.
	opened[incomingCiphertextDigest] = cachedOpen{manifest: incoming, valid: true}
	openAttempts := 0
	var downloaded int64
	for _, info := range infos {
		if info.Size <= 0 || downloaded+info.Size > maxHistoryGraphBytes {
			return false, nil
		}
		object, err := binding.client.Get(ctx, info.Key)
		if err != nil {
			if errors.Is(err, objectstore.ErrNotFound) {
				// LIST described this immutable history object but it disappeared
				// before GET. Its absence cannot prove ancestry and is a changed
				// remote graph, not a generic connectivity failure.
				return false, ErrRemoteMoved
			}
			return false, err
		}
		if info.ETag == "" || object.ETag != info.ETag {
			return false, ErrRemoteMoved
		}
		downloaded += int64(len(object.Body))
		ciphertextDigest := Digest(object.Body)
		cached, seen := opened[ciphertextDigest]
		if !seen {
			if openAttempts >= maxLiveLineageOpenAttempts {
				return false, nil
			}
			openAttempts++
			manifest, _, openErr := openSnapshotObject(object, passphrase)
			cached = cachedOpen{manifest: manifest, valid: openErr == nil}
			opened[ciphertextDigest] = cached
		}
		if !cached.valid {
			// A missing or unreadable link cannot prove ancestry. Fail closed rather
			// than treating a partial graph as permission to apply the live object.
			continue
		}
		manifests[cached.manifest.Revision] = cached.manifest
		if lineageReaches(manifests, incoming.ParentRevision, base.Revision) {
			return true, nil
		}
	}
	return false, nil
}

func manifestAncestors(base *Manifest) []string {
	if base == nil || !validRevision(base.Revision) {
		return nil
	}
	ancestors := make([]string, 0, min(MaxManifestAncestors, 1+len(base.Ancestors)))
	ancestors = append(ancestors, base.Revision)
	for _, revision := range base.Ancestors {
		if len(ancestors) == MaxManifestAncestors || slices.Contains(ancestors, revision) {
			break
		}
		ancestors = append(ancestors, revision)
	}
	return ancestors
}

func lineageReaches(manifests map[string]Manifest, revision, wanted string) bool {
	seen := map[string]bool{}
	for revision != "" && !seen[revision] {
		if revision == wanted {
			return true
		}
		seen[revision] = true
		ancestor, ok := manifests[revision]
		if !ok {
			return false
		}
		revision = ancestor.ParentRevision
	}
	return false
}

type ForcePushConfirmation struct {
	ETag     string
	Evidence string

	bindingVersion uint64
	targetID       string
}

// ForcePushConfirmation binds one action token to the current configured
// binding, target identity, and live ETag. ForcePush validates the complete
// confirmation again while holding operationMu, then retains the remote CAS as
// the final guard against a writer outside this process.
func (s *Service) ForcePushConfirmation(ctx context.Context, target string) (ForcePushConfirmation, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if target != ForcePushTarget {
		return ForcePushConfirmation{}, ErrForcePushTarget
	}
	binding, bindingVersion, err := s.configuredBindingVersion()
	if err != nil {
		return ForcePushConfirmation{}, err
	}
	objectKey := ObjectKeyFor(binding.config)
	live, err := binding.client.Stat(ctx, objectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return ForcePushConfirmation{}, ErrNoSnapshot
		}
		return ForcePushConfirmation{}, err
	}
	targetIdentity := targetID(binding.config)
	evidence := forcePushEvidence(bindingVersion, targetIdentity, live.ETag)
	return ForcePushConfirmation{
		ETag: live.ETag, Evidence: evidence,
		bindingVersion: bindingVersion, targetID: targetIdentity,
	}, nil
}

func forcePushEvidence(bindingVersion uint64, target, etag string) string {
	return Digest([]byte(strings.Join([]string{
		"force-push-v2", fmt.Sprintf("%d", bindingVersion), target, etag,
	}, "\x00")))
}

func snapshotSummary(manifest Manifest, contents map[string][]byte, snapshotBytes int) SnapshotSummary {
	var sourceBytes int64
	for _, body := range contents {
		sourceBytes += int64(len(body))
	}
	return SnapshotSummary{
		CreatedAt: manifest.CreatedAt, FileCount: len(manifest.Files),
		SourceBytes: sourceBytes, SnapshotBytes: int64(snapshotBytes),
	}
}
