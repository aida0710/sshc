package secret

import (
	"testing"

	"sshc/internal/storage"
)

// 起動時に vault の鍵の無いまま片付けてよいのは、巻き戻しが封じた控えを開かない
// 記録だけである。控えを読むのは、適用済みで控えを持つエントリだけである。
func TestARollbackOpensASealedBackupOnlyForAnAppliedEntryThatHasOne(t *testing.T) {
	for _, test := range []struct {
		name    string
		entries []storage.PendingEntry
		want    bool
	}{
		{name: "applied with a backup", entries: []storage.PendingEntry{{Committed: true, HasBackup: true}}, want: true},
		{name: "applied without a backup", entries: []storage.PendingEntry{{Committed: true}}, want: false},
		{name: "not yet applied", entries: []storage.PendingEntry{{HasBackup: true}}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := rollbackOpensSealedBackup(storage.Pending{Entries: test.entries}); got != test.want {
				t.Fatalf("rollbackOpensSealedBackup = %v, want %v", got, test.want)
			}
		})
	}
}
