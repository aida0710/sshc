package acceptance_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"sshc/internal/storage"
)

// 変更の履歴とバックアップを残す件数と期間は、pages と History 画面にも書いてある。
// storage の定数を変えたら、文言も同じ変更で直す。読む人は画面と pages の方を信じる。
//
// バックアップのファイルの上限は pages だけに書く。画面の「200件まで」は上限なので、
// それより少なく残ることがあっても誤りにはならない。
func TestThePagesAndTheHistoryScreenStateTheRetentionTheStorageApplies(t *testing.T) {
	days := int(storage.HistoryRetentionPeriod / (24 * time.Hour))
	for _, language := range []struct {
		name        string
		page        []string
		records     string
		period      string
		backupFiles string
	}{
		{
			name: "ja", page: []string{"pages", "reference", "security.md"},
			records: fmt.Sprintf("%d件", storage.RetainedHistoryRecords), period: fmt.Sprintf("%d日", days),
			backupFiles: fmt.Sprintf("合わせて%d件", storage.RetainedBackupFiles),
		},
		{
			name: "en", page: []string{"pages", "en", "reference", "security.md"},
			records: fmt.Sprintf("%d changes", storage.RetainedHistoryRecords), period: fmt.Sprintf("%d days", days),
			backupFiles: fmt.Sprintf("%d backup files", storage.RetainedBackupFiles),
		},
	} {
		messages := repositoryFile(t, "web", "src", "i18n", "messages", language.name+".ts")
		for _, location := range []struct {
			name   string
			text   string
			stated []string
		}{
			{
				name: "the security page", text: repositoryFile(t, language.page...),
				stated: []string{language.records, language.period, language.backupFiles},
			},
			{
				name: "the History screen", text: messageFor(t, messages, "history.backupRetention"),
				stated: []string{language.records, language.period},
			},
		} {
			for _, stated := range location.stated {
				if !strings.Contains(location.text, stated) {
					t.Errorf("%s (%s) does not state %q", location.name, language.name, stated)
				}
			}
		}
	}
}
