package remotesync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/objectstore"
)

// setupSnapshotMessage は、空の同期先へ設定を保存するときに置く、ファイルを含まない
// スナップショットの message。manifest は message を空にできないので、何の世代かが分かる文を入れる。
const setupSnapshotMessage = "Set up the synchronization target"

// setupRollbackTimeout は、設定の保存に失敗したときに、置いたばかりのスナップショットを
// 消す要求に与える時間。呼び出し元が取り消していても消し終えられるよう、呼び出し元の期限とは
// 別に持つ。消せないと、誰も知らないキーで封じた世代が残り、次の設定が進めなくなるため。
const setupRollbackTimeout = 30 * time.Second

type SetupTargetState string

const (
	SetupTargetEmpty      SetupTargetState = "empty"
	SetupTargetExisting   SetupTargetState = "existing"
	SetupTargetIncomplete SetupTargetState = "incomplete"
)

var (
	ErrSetupTargetChanged    = errors.New("the synchronization target changed after connection check")
	ErrSetupTargetIncomplete = errors.New("the synchronization target has history but no live snapshot")
	// ErrSetupTargetEmpty は、書き出した設定の指す同期先にスナップショットが無いことを報告する。
	ErrSetupTargetEmpty = errors.New("the synchronization target named by the exported settings holds no snapshot")
)

type SetupInspection struct {
	State          SetupTargetState
	ETag           string
	HistoryPresent bool
}

// InspectSetupTarget checks the exact live key and its history prefix without
// changing local state or persisting credentials.
func InspectSetupTarget(ctx context.Context, client *objectstore.Client, config Config) (SetupInspection, error) {
	liveETag, err := client.Head(ctx, ObjectKeyFor(config))
	live := err == nil
	if err != nil && !errors.Is(err, objectstore.ErrNotFound) {
		return SetupInspection{}, err
	}
	history, _, err := client.ListNewest(ctx, joinKey(config.Path, SnapshotPrefix), 1)
	if err != nil {
		return SetupInspection{}, err
	}
	inspection := SetupInspection{ETag: liveETag, HistoryPresent: len(history) > 0}
	switch {
	case live:
		inspection.State = SetupTargetExisting
	case inspection.HistoryPresent:
		inspection.State = SetupTargetIncomplete
	default:
		inspection.State = SetupTargetEmpty
	}
	return inspection, nil
}

func sameSetupInspection(left, right SetupInspection) bool {
	return left.State == right.State && left.ETag == right.ETag && left.HistoryPresent == right.HistoryPresent
}

func verifySetupKey(ctx context.Context, client *objectstore.Client, config Config, expectedETag, syncKey string) error {
	object, err := client.Get(ctx, ObjectKeyFor(config))
	if err != nil {
		return err
	}
	if object.ETag != expectedETag {
		return ErrSetupTargetChanged
	}
	archive, derivedKey, err := envelope.OpenRemote(object.Body, syncKey)
	if err != nil {
		return err
	}
	derivedKey.Destroy()
	_, _, err = Read(archive)
	return err
}

// CompleteSetup rechecks the target, validates an existing snapshot key, then
// persists settings and publishes the binding under one operation boundary.
// An empty target receives a snapshot without files before the settings are
// persisted, so that the key shown to the user is already the target's key.
func (s *Service) CompleteSetup(ctx context.Context, config Config, credentials objectstore.Credentials,
	client *objectstore.Client, expected SetupInspection, syncKey string, persist func() error) error {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()
	config = normalizeConfig(config)
	if persist == nil {
		return errors.New("remote sync settings persistence is not configured")
	}
	if err := s.validateRecoveryTarget(config); err != nil {
		return err
	}
	actual, err := InspectSetupTarget(ctx, client, config)
	if err != nil {
		return err
	}
	if !sameSetupInspection(expected, actual) {
		return ErrSetupTargetChanged
	}
	if actual.State == SetupTargetIncomplete {
		return ErrSetupTargetIncomplete
	}
	if err := ValidateKey(syncKey); err != nil {
		return err
	}
	undoSetupSnapshot := func() error { return nil }
	switch actual.State {
	case SetupTargetExisting:
		if err := verifySetupKey(ctx, client, config, actual.ETag, syncKey); err != nil {
			return err
		}
	case SetupTargetEmpty:
		if undoSetupSnapshot, err = s.placeSetupSnapshot(ctx, client, config, syncKey); err != nil {
			return err
		}
	}
	if err := persist(); err != nil {
		return errors.Join(err, undoSetupSnapshot())
	}
	s.configure(config, credentials, client)
	return nil
}

// JoinSetup は、書き出した設定（SettingsExport）の同期先に、それを開いた syncKey で参加する。
//
// 書き出したマシンは設定のときにスナップショットを置いているので、同期先は既存のはずである。
// 空なら、書き出したあとで同期先が消されたか別の場所を指している。ここで新しい同期先として
// 始めると、書き出したマシンと別々の同期先になるので断る。
func (s *Service) JoinSetup(ctx context.Context, config Config, credentials objectstore.Credentials,
	client *objectstore.Client, syncKey string, persist func() error) error {
	inspection, err := InspectSetupTarget(ctx, client, normalizeConfig(config))
	if err != nil {
		return err
	}
	switch inspection.State {
	case SetupTargetEmpty:
		return ErrSetupTargetEmpty
	case SetupTargetIncomplete:
		return ErrSetupTargetIncomplete
	}
	return s.CompleteSetup(ctx, config, credentials, client, inspection, syncKey, persist)
}

// placeSetupSnapshot は、空の同期先へファイルを含まないスナップショットを置き、その世代を
// このマシンの同期の記録にする。返す関数は、置いたスナップショットを消し、記録を元に戻す。
//
// 置くのは、このマシンが最初に同期する前に別のマシンが同じ同期先を設定したとき、そのマシンに
// このキーを入力させるためである。空のままだと、別のマシンは新しい同期先と見て別のキーを作り、
// あとで同期したこのマシンはキーが合わなくなる。ファイルを含めないのは、除外の設定と送る内容を
// 利用者が確かめる前に、~/.ssh の中身を消せない履歴へ送らないためである。同じ理由で受信専用の
// マシンも置く。このマシンの内容は何も出ていかない。
//
// 日付付きの履歴の候補は置かない。候補を置いたあとでライブの書き込みに失敗すると、同期先は
// 「履歴だけがある」状態になり、参加も新規の設定も断られる。中身の無い世代は、戻す先としての
// 意味も無い。
//
// 記録を残すのは、このマシンの次の送信が、このスナップショットを親にして置き換えるためと、
// 最初の同期の前にキーを置き換えたとき、このスナップショットも新しいキーで封じ直すためである。
// 記録は同期した操作を持たないので、画面には「まだ同期していません」と出る。
func (s *Service) placeSetupSnapshot(ctx context.Context, client *objectstore.Client, config Config,
	syncKey string) (func() error, error) {
	previousState, err := s.readStateFile()
	if err != nil {
		return nil, err
	}
	current, err := s.readState()
	if err != nil {
		return nil, err
	}
	origin := current.Origin
	if origin == "" {
		if origin, err = s.newOrigin(); err != nil {
			return nil, err
		}
	}
	manifest := Manifest{CreatedAt: s.now(), Origin: origin, Message: setupSnapshotMessage, Files: []Entry{}}
	if err := FinalizeManifest(&manifest, ""); err != nil {
		return nil, err
	}
	sealed, err := sealSnapshot(manifest, map[string][]byte{}, syncKey)
	if err != nil {
		return nil, err
	}
	objectKey := ObjectKeyFor(config)
	etag, err := client.Put(ctx, objectKey, sealed, "", "*")
	if errors.Is(err, objectstore.ErrPreconditionFailed) {
		// 確かめてから置くまでのあいだに、別のマシンがこの同期先を設定した。
		return nil, ErrSetupTargetChanged
	}
	if err != nil {
		return nil, err
	}
	// キーはまだ誰にも見せていないので、ほかのマシンがこの世代の上に何かを置いていることは無い。
	// 条件を付けずに消してよい。
	removeSnapshot := func() error {
		removeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), setupRollbackTimeout)
		defer cancel()
		if err := client.Delete(removeCtx, objectKey); err != nil {
			return fmt.Errorf("remove the setup snapshot: %w", err)
		}
		return nil
	}
	if err := s.writeState(state{
		ETag: etag, Key: objectKey, Target: targetID(config), Base: &manifest, Origin: origin,
	}); err != nil {
		return nil, errors.Join(err, removeSnapshot())
	}
	return func() error {
		return errors.Join(removeSnapshot(), s.restoreStateFile(previousState))
	}, nil
}
