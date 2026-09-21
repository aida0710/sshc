package remotesync_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/remotesync"
)

func TestLocalDeletionIsNotSilentlyRecreated(t *testing.T) {
	base := manifestOf(file("keys/old", "private key"))
	remote := manifestOf(file("keys/old", "remote changed key"))
	request, conflicts, err := remotesync.PlanEntriesWithIgnore(root, &base, map[string]remotesync.LocalEntry{}, remote,
		map[string][]byte{"keys/old": []byte("remote changed key")}, remotesync.ResolveLocal, nil)
	if err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		t.Fatal(err)
	}
	if len(request.Changes) != 0 || len(conflicts) != 0 {
		t.Fatalf("keep-local recreates a deleted key: changes=%d, conflicts=%d", len(request.Changes), len(conflicts))
	}
}

func TestAutomaticPullDoesNotUndoLocalDeletion(t *testing.T) {
	bucket := &fakeBucket{}
	writer := newInstallation(t, bucket, map[string]string{"config": "Host edge\n", "keys/old": "synthetic-key"})
	if _, err := writer.service.PushUsing(t.Context(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatal(err)
	}
	receiver := newInstallation(t, bucket, map[string]string{})
	preview, err := receiver.service.Pull(t.Context(), syncPassphrase, remotesync.ResolveNone)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyPreview(receiver.service, remotesync.ResolveNone, "", preview); err != nil {
		t.Fatal(err)
	}
	receiver.remove(t, "keys/old")
	writer.write(t, "config", "Host edge\n\tPort 2222\n")
	if _, err := writer.service.PushUsing(t.Context(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatal(err)
	}
	view := autoFor(t, receiver, true).Poll(t.Context())
	if _, err := os.Stat(filepath.Join(receiver.workspace.Root(), "keys/old")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("automatic pull restored a locally deleted key; phase=%s detail=%s", view.Phase, view.Detail)
	}
}

func TestLocalDeletionConflictsWithRemoteEdit(t *testing.T) {
	base := manifestOf(file("keys/old", "private key"))
	remote := manifestOf(file("keys/old", "remote changed key"))
	request, conflicts, err := remotesync.PlanEntriesWithIgnore(root, &base, map[string]remotesync.LocalEntry{}, remote,
		map[string][]byte{"keys/old": []byte("remote changed key")}, remotesync.ResolveNone, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || len(request.Changes) != 0 {
		t.Fatalf("delete/edit silently accepted: changes=%d, conflicts=%d", len(request.Changes), len(conflicts))
	}
}
