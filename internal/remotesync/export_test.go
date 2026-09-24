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
		entries[path] = entryState(digest, "0600")
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
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	config = normalizeConfig(config)
	if err := s.validateRecoveryTarget(config); err != nil {
		return err
	}
	s.configure(config, credentials, client)
	return nil
}
