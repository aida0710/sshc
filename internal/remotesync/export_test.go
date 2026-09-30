package remotesync

import (
	"time"

	"sshc/internal/objectstore"
	"sshc/internal/storage"
)

// テストからだけ使う入口。製品は PlanEntriesWithIgnore と push の内部を使う。

// PlanForTest は digest の表から LocalEntry を組み立てて計画する。mode を持たない
// 単体テストのための包みで、製品の manifest は必ず mode を持つ。
func PlanForTest(root string, base *Manifest, local map[string]string, remote Manifest, contents map[string][]byte, resolve Resolution) (storage.Request, []Conflict, error) {
	return PlanWithIgnoreForTest(root, base, local, remote, contents, resolve, nil)
}

// PlanWithIgnoreForTest は PlanForTest に除外規則を足す。
func PlanWithIgnoreForTest(root string, base *Manifest, local map[string]string, remote Manifest, contents map[string][]byte, resolve Resolution, ignored func(string) bool) (storage.Request, []Conflict, error) {
	entries := make(map[string]LocalEntry, len(local))
	for path, digest := range local {
		entries[path] = LocalEntry{SHA256: digest, Mode: "0600"}
	}
	return PlanEntriesWithIgnore(root, base, entries, remote, contents, resolve, ignored)
}

// SnapshotKeyForTest は、createdAt の snapshot が置かれる object key を返す。
func SnapshotKeyForTest(config Config, createdAt string) (string, error) {
	moment, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return "", err
	}
	return joinKey(config.Path, SnapshotPrefix+moment.UTC().Format(datedLayout)+"."+archiveSuffix), nil
}

// ConfigureForTest は、テストの前準備として、設定を保存せずにこの service を
// 接続先へ向ける。製品では CompleteSetup が、接続先を確かめて設定を保存してから
// 同じ切り替えを行う。
func (s *Service) ConfigureForTest(config Config, credentials objectstore.Credentials, client *objectstore.Client) error {
	s.operationMutex.Lock()
	defer s.operationMutex.Unlock()
	config = normalizeConfig(config)
	if err := s.validateRecoveryTarget(config); err != nil {
		return err
	}
	s.configure(config, credentials, client)
	return nil
}

// PushTimerForTest は、Run が送信の期限を待つ timer の形である。
type PushTimerForTest = pushTimer

// SetClockForTest は、backoff と送信の期限を決める時計を clock に替える。
func (a *Auto) SetClockForTest(clock func() time.Time) {
	a.clock = clock
}

// SetPushTimerForTest は、Run が送信の期限を待つ timer を start の作るものに替える。
// テストは SetClockForTest の時計を進めてから自分で発火させ、期限の前後を実時間に
// 頼らずに確かめる。
func (a *Auto) SetPushTimerForTest(start func(delay time.Duration) PushTimerForTest) {
	a.newPushTimer = start
}
