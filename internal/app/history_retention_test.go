package app

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/storage"
)

// changeRecordIdentifierLayout は、storage が変更の記録の ID の先頭に付ける、変更を
// 始めた時刻（UTC）の書式。
const changeRecordIdentifierLayout = "20060102T150405.000"

// placeFinishedChange は、started に始めて完了した変更の記録（history/<id>.json）と
// 控え（backups/<id>/）を置き、その 2 つのパスを返す。
func placeFinishedChange(t *testing.T, home string, started time.Time) []string {
	t.Helper()
	identifier := started.UTC().Format(changeRecordIdentifierLayout) + "-0badc0de"
	state := filepath.Join(home, ".ssh", "sshc")
	record := filepath.Join(state, "history", identifier+".json")
	backup := filepath.Join(state, storage.BackupDirectoryName, identifier)
	acltest.WritePrivateFile(t, record, []byte("{}\n"))
	acltest.WritePrivateFile(t, filepath.Join(backup, "config"), []byte("Host before\n"))
	return []string{record, backup}
}

// sshcエンジンを組むときに、保持の期間を過ぎた変更の記録と控えを消す。長く変更の無かった
// ワークスペースでも、次の変更を待たずに消す。
func TestBuildingTheEngineRemovesChangeRecordsPastTheRetentionPeriod(t *testing.T) {
	home := t.TempDir()
	expired := placeFinishedChange(t, home, time.Now().Add(-storage.HistoryRetentionPeriod-24*time.Hour))
	recent := placeFinishedChange(t, home, time.Now().Add(-time.Hour))

	if _, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader}); err != nil {
		t.Fatal(err)
	}

	for _, path := range expired {
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s past the retention period is left: %v", path, err)
		}
	}
	for _, path := range recent {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s within the retention period was removed: %v", path, err)
		}
	}
}
