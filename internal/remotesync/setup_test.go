package remotesync_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"sshc/internal/envelope"
	"sshc/internal/remotesync"
)

// secondMachineKey は、2台目が自分で作ってしまうキーの代わり。1台目のキーとは違う。
const secondMachineKey = "a key the second machine would have made for itself"

// setUpTarget は、設定画面と同じく接続先を確かめてから設定を保存する。確かめた状態を返す。
func setUpTarget(t *testing.T, machine installation, direction remotesync.Direction, syncKey string) remotesync.SetupInspection {
	t.Helper()
	config := machine.config
	config.Direction = direction
	inspection, err := remotesync.InspectSetupTarget(context.Background(), machine.client, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := machine.service.CompleteSetup(context.Background(), config, machine.credentials, machine.client,
		inspection, syncKey, func() error { return nil }); err != nil {
		t.Fatalf("CompleteSetup on %s target = %v", inspection.State, err)
	}
	return inspection
}

// requireOnlyTheSetupSnapshot は、同期先に設定で置いたスナップショットしか無いことを確かめる。
// path は同期先のパスで、そのスナップショットは syncKey で開き、ファイルを含まない。送信が
// 1回でも届いていれば、日付付きの履歴の候補とファイルがあるはずである。
func requireOnlyTheSetupSnapshot(t *testing.T, bucket *fakeBucket, path, syncKey string) {
	t.Helper()
	liveKey := remotesync.ObjectName
	if path != "" {
		liveKey = path + "/" + remotesync.ObjectName
	}
	if got := bucket.keys(); len(got) != 1 || got[0] != liveKey {
		t.Fatalf("target holds %v, want only %s", got, liveKey)
	}
	archive, key, err := envelope.OpenRemote(bucket.object(liveKey), syncKey)
	if err != nil {
		t.Fatalf("setup snapshot does not open with the expected key: %v", err)
	}
	key.Destroy()
	manifest, contents, err := remotesync.Read(archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 0 || len(contents) != 0 || manifest.ParentRevision != "" {
		t.Fatalf("setup snapshot = %+v with %d files, want a root without files", manifest, len(contents))
	}
}

// joinAndSend は、同期先に参加したマシンの最初の［今すぐ同期］と同じく、受信してから送信する。
func joinAndSend(t *testing.T, machine installation, syncKey string) {
	t.Helper()
	// 設定で置いた世代にはファイルが無いので、受信するものが無いと言われても適用して世代を記録する。
	preview, err := machine.service.Pull(context.Background(), syncKey, remotesync.ResolveNone)
	if err != nil && !errors.Is(err, remotesync.ErrNothingToApply) {
		t.Fatalf("Pull = %v", err)
	}
	if err := applyPreviewWithKey(machine.service, syncKey, pullChoice{}, preview); err != nil {
		t.Fatalf("Apply = %v", err)
	}
	if _, err := machine.service.PushUsing(context.Background(), keyOf(syncKey), ""); err != nil {
		t.Fatalf("Push = %v", err)
	}
}

// 1台目が［確認して保存］だけを済ませ、まだ同期していないうちに2台目が同じ同期先を設定すると、
// 2台目は空の同期先とは見ず、1台目のキーを求められる（#632）。
func TestASecondMachineSettingUpBeforeTheFirstSyncsIsAskedForTheFirstKey(t *testing.T) {
	for _, direction := range []remotesync.Direction{remotesync.DirectionBoth, remotesync.DirectionPush, remotesync.DirectionPull} {
		t.Run(string(direction), func(t *testing.T) {
			bucket := &fakeBucket{}
			first := newInstallation(t, bucket, map[string]string{"config": "Host first\n"})
			second := newInstallation(t, bucket, map[string]string{"second-only": "from the second machine\n"})
			if inspection := setUpTarget(t, first, direction, syncPassphrase); inspection.State != remotesync.SetupTargetEmpty {
				t.Fatalf("first machine saw %q, want an empty target", inspection.State)
			}

			inspection, err := remotesync.InspectSetupTarget(context.Background(), second.client, second.config)
			if err != nil {
				t.Fatal(err)
			}
			if inspection.State != remotesync.SetupTargetExisting || inspection.ETag == "" || inspection.HistoryPresent {
				t.Fatalf("second machine inspection = %+v, want existing without history", inspection)
			}
			persisted := false
			persist := func() error { persisted = true; return nil }
			if err := second.service.CompleteSetup(context.Background(), second.config, second.credentials, second.client,
				inspection, secondMachineKey, persist); !errors.Is(err, remotesync.ErrWrongPassphrase) {
				t.Fatalf("CompleteSetup with another key = %v, want ErrWrongPassphrase", err)
			}
			if persisted {
				t.Fatal("the second machine saved a key that does not open the first machine's snapshot")
			}
			if err := second.service.CompleteSetup(context.Background(), second.config, second.credentials, second.client,
				inspection, syncPassphrase, persist); err != nil {
				t.Fatalf("CompleteSetup with the first machine's key = %v", err)
			}

			// 2台目が先に送っても、あとで同期する1台目は同じキーで開ける。
			joinAndSend(t, second, syncPassphrase)
			preview, err := first.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone)
			if err != nil {
				t.Fatalf("first machine Pull after the second machine sent = %v", err)
			}
			if !slices.Contains(preview.Written, "second-only") {
				t.Fatalf("first machine preview writes %v, want the second machine's file", preview.Written)
			}
			if direction == remotesync.DirectionPush {
				return // 送信専用のマシンは受信した内容を適用しない。
			}
			if err := applyPreviewWithKey(first.service, syncPassphrase, pullChoice{}, preview); err != nil {
				t.Fatalf("first machine Apply = %v", err)
			}
			if got := first.read(t, "second-only"); got != "from the second machine\n" {
				t.Fatalf("first machine received %q", got)
			}
		})
	}
}

// 設定の保存で置くのは、ファイルを含まないライブのスナップショット1つだけである。
// 除外の設定を確かめる前に ~/.ssh の中身を送らず、画面も「まだ同期していません」のままにする。
func TestSettingUpAnEmptyTargetSendsNoFilesAndIsNotASync(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "Host first\n", "id_ed25519": "private key\n"})
	setUpTarget(t, first, remotesync.DirectionBoth, syncPassphrase)

	requireOnlyTheSetupSnapshot(t, bucket, "", syncPassphrase)
	if view := first.service.SyncState(); view.Synced || view.LastOperation != nil {
		t.Fatalf("SyncState after setup = %+v, want not synced", view)
	}

	// 最初の送信は、設定で置いた世代を親にして置き換える。別のマシンが送ったとは扱わない。
	result, err := first.service.PushUsing(context.Background(), keyOf(syncPassphrase), "")
	if err != nil {
		t.Fatalf("first Push after setup = %v", err)
	}
	if len(result.Added) != 2 {
		t.Fatalf("first Push added %v, want config and id_ed25519", result.Added)
	}
	if view := first.service.SyncState(); !view.Synced || view.Files != 2 {
		t.Fatalf("SyncState after the first push = %+v", view)
	}
}

// 最初の同期の前にキーを置き換えたら、設定で置いたスナップショットも新しいキーで封じ直す。
// 古いキーのまま残すと、2台目は新しいキーで参加できず、1台目も自分の世代を開けない。
func TestReplacingTheKeyBeforeTheFirstSyncResealsTheSetupSnapshot(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "Host first\n"})
	second := newInstallation(t, bucket, map[string]string{})
	setUpTarget(t, first, remotesync.DirectionBoth, syncPassphrase)
	const replacement = "a replacement key chosen before the first sync"
	if err := first.service.ReplaceKeyUsing(context.Background(), replacement, true,
		replacing(syncPassphrase, func() error { return nil })); err != nil {
		t.Fatalf("ReplaceKey = %v", err)
	}

	inspection, err := remotesync.InspectSetupTarget(context.Background(), second.client, second.config)
	if err != nil {
		t.Fatal(err)
	}
	persist := func() error { return nil }
	if err := second.service.CompleteSetup(context.Background(), second.config, second.credentials, second.client,
		inspection, syncPassphrase, persist); !errors.Is(err, remotesync.ErrWrongPassphrase) {
		t.Fatalf("CompleteSetup with the replaced key = %v, want ErrWrongPassphrase", err)
	}
	if err := second.service.CompleteSetup(context.Background(), second.config, second.credentials, second.client,
		inspection, replacement, persist); err != nil {
		t.Fatalf("CompleteSetup with the new key = %v", err)
	}
	if _, err := first.service.PushUsing(context.Background(), keyOf(replacement), ""); err != nil {
		t.Fatalf("first machine Push with the new key = %v", err)
	}
}

// 2台が同じ空の同期先を同時に設定しても、キーは1つにしかならない。遅れた方は、確かめ直すよう
// 断られ、何も保存しない。
func TestTwoMachinesSettingUpOneEmptyTargetAtOnceKeepOneKey(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "Host first\n"})
	second := newInstallation(t, bucket, map[string]string{"config": "Host second\n"})
	secondInspection, err := remotesync.InspectSetupTarget(context.Background(), second.client, second.config)
	if err != nil {
		t.Fatal(err)
	}
	setUpTarget(t, first, remotesync.DirectionBoth, syncPassphrase)

	persisted := false
	err = second.service.CompleteSetup(context.Background(), second.config, second.credentials, second.client,
		secondInspection, secondMachineKey, func() error { persisted = true; return nil })
	if !errors.Is(err, remotesync.ErrSetupTargetChanged) {
		t.Fatalf("late CompleteSetup = %v, want ErrSetupTargetChanged", err)
	}
	if persisted {
		t.Fatal("the late machine saved its own key")
	}
	if _, _, err := envelope.OpenRemote(bucket.object(remotesync.ObjectName), syncPassphrase); err != nil {
		t.Fatalf("the first machine's snapshot no longer opens with its key: %v", err)
	}
}

// 設定の保存に失敗したら、置いたスナップショットを消し、同期の記録も元に戻す。残すと、誰も
// 知らないキーで封じた世代のせいで、次の設定でキーを求められてしまう。
func TestAFailedSaveRemovesTheSetupSnapshotAndKeepsTheOldRecord(t *testing.T) {
	oldBucket := &fakeBucket{}
	machine := newInstallation(t, oldBucket, map[string]string{"config": "Host machine\n"})
	if _, err := machine.service.PushUsing(context.Background(), keyOf(syncPassphrase), "Initial snapshot"); err != nil {
		t.Fatal(err)
	}
	newBucket := &fakeBucket{}
	target := newInstallation(t, newBucket, map[string]string{})
	inspection, err := remotesync.InspectSetupTarget(context.Background(), target.client, target.config)
	if err != nil {
		t.Fatal(err)
	}
	saveFailed := errors.New("the vault refused the settings")
	err = machine.service.CompleteSetup(context.Background(), target.config, target.credentials, target.client,
		inspection, secondMachineKey, func() error { return saveFailed })
	if !errors.Is(err, saveFailed) {
		t.Fatalf("CompleteSetup = %v, want the save failure", err)
	}
	if got := newBucket.keys(); len(got) != 0 {
		t.Fatalf("a failed save left %v in the new target", got)
	}
	// 設定は前の同期先のまま。その同期の記録も残っているので、送るものが無いと分かる。
	if _, err := machine.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); !errors.Is(err, remotesync.ErrNothingToPush) {
		t.Fatalf("Push to the previous target after a failed setup = %v, want ErrNothingToPush", err)
	}
}
