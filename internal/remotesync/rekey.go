package remotesync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/objectstore"
	"sshc/internal/storage"
)

// KeyRecoveryPath records only remote generation metadata for an interrupted
// key rotation. Key material remains solely in the encrypted secret service.
const KeyRecoveryPath = "sshc/sync-key-recovery.json"

type keyRecoveryJournal struct {
	SchemaVersion       int    `json:"schemaVersion"`
	Phase               string `json:"phase"`
	Target              string `json:"target"`
	ObjectKey           string `json:"objectKey"`
	OldETag             string `json:"oldETag"`
	NewETag             string `json:"newETag,omitempty"`
	OldCiphertextSHA256 string `json:"oldCiphertextSHA256"`
	NewCiphertextSHA256 string `json:"newCiphertextSHA256"`
}

func (s *Service) ReplaceKeyUsing(ctx context.Context, newKey string, confirmHistoryLoss bool, provider KeyReplacementProvider) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if provider == nil {
		return errors.New("sync key replacement provider is not configured")
	}
	oldKey, commit, err := provider()
	if err != nil {
		return err
	}
	return s.replaceKey(ctx, oldKey, newKey, confirmHistoryLoss, commit)
}

func (s *Service) replaceKey(ctx context.Context, oldKey, newKey string, confirmHistoryLoss bool, commit func() error) error {
	if commit == nil {
		return errors.New("sync key commit is not configured")
	}
	if !s.Configured() {
		return commit()
	}
	binding, err := s.configuredBinding()
	if err != nil {
		return err
	}
	if handled, err := s.resolveKeyRecovery(ctx, binding, newKey, commit); handled || err != nil {
		return err
	}
	if oldKey == "" || oldKey == newKey {
		return commit()
	}
	if !confirmHistoryLoss {
		return ErrHistoryKeyLossConfirmation
	}
	current, err := s.readState()
	if err != nil {
		return err
	}
	if !stateMatchesTarget(current, binding.config) || current.ETag == "" {
		return commit()
	}
	objectKey := ObjectKeyFor(binding.config)
	object, err := binding.client.Get(ctx, objectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return ErrRemoteMoved
		}
		return err
	}
	if object.ETag != current.ETag {
		return ErrRemoteMoved
	}
	archive, openedKey, err := envelope.OpenWithin(object.Body, oldKey, envelope.AcceptedFromRemote)
	if err != nil {
		return err
	}
	openedKey.Destroy()
	// Rotation is deliberately not a legacy reader. Refuse to bless ciphertext
	// whose snapshot schema this binary would not otherwise accept.
	if _, _, err := Read(archive); err != nil {
		return err
	}
	key, err := envelope.Derive(newKey)
	if err != nil {
		return err
	}
	resealed, err := key.Seal(archive)
	key.Destroy()
	if err != nil {
		return err
	}
	recovery := keyRecoveryJournal{
		SchemaVersion: keyRecoverySchemaVersion, Phase: keyRecoveryPrepared,
		Target: targetID(binding.config), ObjectKey: objectKey, OldETag: object.ETag,
		OldCiphertextSHA256: Digest(object.Body),
		NewCiphertextSHA256: Digest(resealed),
	}
	if err := s.writeKeyRecovery(recovery); err != nil {
		return err
	}
	newETag, err := binding.client.Put(ctx, objectKey, resealed, object.ETag, "")
	if err != nil {
		if errors.Is(err, objectstore.ErrPreconditionFailed) {
			if cleanupErr := s.removeKeyRecovery(); cleanupErr != nil {
				return errors.Join(err, ErrRecoveryRequired, fmt.Errorf("clear sync key recovery journal: %w", cleanupErr))
			}
			return ErrRemoteMoved
		}
		// A timeout, disconnect, or 5xx can arrive after the store committed the
		// conditional PUT. Keep the prepared evidence until a fresh GET proves
		// whether the live body is exactly the old or new ciphertext.
		return errors.Join(err, ErrRecoveryRequired)
	}
	recovery.Phase = keyRecoveryRemoteAdvanced
	recovery.NewETag = newETag
	rollback := func(cause error) error {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		rollbackETag, rollbackErr := binding.client.Put(rollbackCtx, objectKey, object.Body, newETag, "")
		if rollbackErr != nil {
			if errors.Is(rollbackErr, objectstore.ErrPreconditionFailed) {
				rollbackErr = ErrRemoteMoved
			}
			return errors.Join(cause, ErrRecoveryRequired, fmt.Errorf("restore remote sync key: %w", rollbackErr))
		}
		restored := current
		restored.ETag = rollbackETag
		if stateErr := s.writeState(restored); stateErr != nil {
			return errors.Join(cause, ErrRecoveryRequired, fmt.Errorf("record restored remote sync key: %w", stateErr))
		}
		if removeErr := s.removeKeyRecovery(); removeErr != nil {
			return errors.Join(cause, ErrRecoveryRequired, fmt.Errorf("clear sync key recovery journal: %w", removeErr))
		}
		return cause
	}
	if err := s.writeKeyRecovery(recovery); err != nil {
		return rollback(errors.Join(ErrRecoveryRequired, fmt.Errorf("record advanced remote sync key: %w", err)))
	}
	advanced := current
	advanced.ETag = newETag
	if err := s.writeState(advanced); err != nil {
		return rollback(err)
	}
	if err := commit(); err != nil {
		return rollback(err)
	}
	if err := s.removeKeyRecovery(); err != nil {
		return errors.Join(ErrRecoveryRequired, fmt.Errorf("clear sync key recovery journal: %w", err))
	}
	return nil
}

func (s *Service) resolveKeyRecovery(ctx context.Context, binding remoteBinding, candidate string, commit func() error) (bool, error) {
	journal, exists, err := s.readKeyRecovery()
	if err != nil || !exists {
		return exists, err
	}
	if journal.Target != targetID(binding.config) || journal.ObjectKey != ObjectKeyFor(binding.config) {
		return true, ErrRecoveryRequired
	}
	etag, err := binding.client.Head(ctx, journal.ObjectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return true, ErrRecoveryRequired
		}
		return true, err
	}
	if etag == journal.OldETag {
		current, err := s.readState()
		if err != nil {
			return true, err
		}
		if !stateMatchesTarget(current, binding.config) || current.Base == nil {
			return true, ErrRecoveryRequired
		}
		current.ETag = etag
		if err := s.writeState(current); err != nil {
			return true, errors.Join(ErrRecoveryRequired, err)
		}
		if err := s.removeKeyRecovery(); err != nil {
			return true, errors.Join(ErrRecoveryRequired, err)
		}
		return false, nil
	}
	object, err := binding.client.Get(ctx, journal.ObjectKey)
	if err != nil {
		return true, err
	}
	if object.ETag != etag {
		return true, ErrRemoteMoved
	}
	bodyDigest := Digest(object.Body)
	if bodyDigest == journal.OldCiphertextSHA256 {
		current, err := s.readState()
		if err != nil {
			return true, err
		}
		if !stateMatchesTarget(current, binding.config) || current.Base == nil {
			return true, ErrRecoveryRequired
		}
		current.ETag = etag
		if err := s.writeState(current); err != nil {
			return true, errors.Join(ErrRecoveryRequired, err)
		}
		if err := s.removeKeyRecovery(); err != nil {
			return true, errors.Join(ErrRecoveryRequired, err)
		}
		// The remote conclusively contains the original ciphertext, so the
		// encrypted vault must remain on the original key. ReplaceKey may now
		// retry the requested rotation as a fresh operation.
		return false, nil
	}
	if bodyDigest != journal.NewCiphertextSHA256 {
		return true, ErrRecoveryRequired
	}
	if journal.Phase == keyRecoveryRemoteAdvanced && (journal.NewETag == "" || etag != journal.NewETag) {
		return true, ErrRecoveryRequired
	}
	archive, openedKey, err := envelope.OpenWithin(object.Body, candidate, envelope.AcceptedFromRemote)
	if err != nil {
		return true, err
	}
	openedKey.Destroy()
	if _, _, err := Read(archive); err != nil {
		return true, err
	}
	current, err := s.readState()
	if err != nil {
		return true, err
	}
	if !stateMatchesTarget(current, binding.config) || current.Base == nil {
		return true, ErrRecoveryRequired
	}
	current.ETag = etag
	if err := s.writeState(current); err != nil {
		return true, errors.Join(ErrRecoveryRequired, err)
	}
	if err := commit(); err != nil {
		return true, errors.Join(ErrRecoveryRequired, err)
	}
	if err := s.removeKeyRecovery(); err != nil {
		return true, errors.Join(ErrRecoveryRequired, err)
	}
	return true, nil
}

func (s *Service) keyRecoveryPath() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(KeyRecoveryPath))
}

func (s *Service) readKeyRecovery() (keyRecoveryJournal, bool, error) {
	body, err := s.workspace.FileSystem().ReadFile(s.keyRecoveryPath())
	if errors.Is(err, fs.ErrNotExist) {
		return keyRecoveryJournal{}, false, nil
	}
	if err != nil {
		return keyRecoveryJournal{}, false, err
	}
	var journal keyRecoveryJournal
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil || journal.SchemaVersion != keyRecoverySchemaVersion ||
		journal.Target == "" || journal.ObjectKey == "" || journal.OldETag == "" ||
		len(journal.OldCiphertextSHA256) != 64 || len(journal.NewCiphertextSHA256) != 64 ||
		(journal.Phase != keyRecoveryPrepared && journal.Phase != keyRecoveryRemoteAdvanced) ||
		(journal.Phase == keyRecoveryRemoteAdvanced && journal.NewETag == "") {
		return keyRecoveryJournal{}, true, ErrRecoveryRequired
	}
	return journal, true, nil
}

func (s *Service) writeKeyRecovery(journal keyRecoveryJournal) error {
	body, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	precondition := storage.Precondition{}
	if current, readErr := s.workspace.FileSystem().ReadFile(s.keyRecoveryPath()); readErr == nil {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(current)}
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return readErr
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation: "sync.key-recovery",
		Changes: []storage.Change{{
			Path: s.keyRecoveryPath(), Contents: body, Precondition: precondition, SkipBackup: true,
		}},
	})
	return err
}

func (s *Service) removeKeyRecovery() error {
	body, err := s.workspace.FileSystem().ReadFile(s.keyRecoveryPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation: "sync.key-recovery.clear",
		Removals: []storage.Removal{{
			Path:         s.keyRecoveryPath(),
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(body)},
		}},
	})
	return err
}

// ensureNoKeyRecovery resolves only the outcome which can be proven without
// either key: a prepared rotation whose old ETag is still live. Every uncertain
// remote advance stops before another sync operation can obscure the evidence.
func (s *Service) ensureNoKeyRecovery(ctx context.Context, binding remoteBinding) error {
	journal, exists, err := s.readKeyRecovery()
	if err != nil || !exists {
		return err
	}
	if journal.Target != targetID(binding.config) || journal.ObjectKey != ObjectKeyFor(binding.config) {
		return ErrRecoveryRequired
	}
	etag, err := binding.client.Head(ctx, journal.ObjectKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return ErrRecoveryRequired
		}
		return err
	}
	if etag != journal.OldETag {
		return ErrRecoveryRequired
	}
	// The old ciphertext is still authoritative. A prepared journal therefore
	// proves that the remote CAS never advanced (or was rolled back before any
	// local key commit), so the marker can be safely cleared.
	return s.removeKeyRecovery()
}
