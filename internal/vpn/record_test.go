package vpn

import (
	"context"
	"strings"
	"testing"

	"sshc/internal/connectionlog"
)

// 接続の試みは、接続ログが無くても経路ごとの記録に残る。
func TestAnAttemptIsRecordedEvenWithoutAConnectionLog(t *testing.T) {
	manager := New(t.TempDir(), 1000, nil)
	profile := validProfile()

	_, err := manager.Dial(context.Background(), profile, Secrets{}, "lab.example.jp:22")

	if err == nil {
		t.Fatal("DNS の無いプロファイルでホスト名の接続先へ繋いだ")
	}
	record := manager.state(profile.Name).record.text()
	for _, want := range []string{"[debug2] lab.example.jp:22 へ、VPNプロファイル tohoku", "接続先をVPN経由で使用できません"} {
		if !strings.Contains(record, want) {
			t.Fatalf("記録に %q が無い:\n%s", want, record)
		}
	}
}

// 同じ ctx で重ねて記録を足しても、同じ行を二度残さない。
func TestRecordingTwiceWritesEachLineOnce(t *testing.T) {
	manager := New(t.TempDir(), 1000, nil)
	ctx := manager.recording(manager.recording(context.Background(), "lab"), "lab")

	connectionlog.Say(ctx, connectionlog.Brief, "一度だけ")

	if record := manager.state("lab").record.text(); strings.Count(record, "一度だけ") != 1 {
		t.Fatalf("record = %q", record)
	}
}

// 記録は上限の行数を超えると、古い行から捨てる。
func TestTheRecordKeepsOnlyItsNewestLines(t *testing.T) {
	var record attemptRecord
	for index := 0; index < maxRecordLines+5; index++ {
		record.Write(connectionlog.Detailed, "行")
	}
	record.Write(connectionlog.Brief, "最後")

	lines := strings.Split(record.text(), "\n")
	if len(lines) != maxRecordLines || !strings.HasSuffix(lines[len(lines)-1], "[debug1] 最後") {
		t.Fatalf("lines = %d, last = %q", len(lines), lines[len(lines)-1])
	}
}

// ログは、sshcエンジンの記録とコンテナのログを見出し付きで並べる。
func TestShownLogsCarryTheEngineRecordAndTheContainerLogs(t *testing.T) {
	shown := joinLogSections("12:00:00 [debug1] 起動します", "")

	for _, want := range []string{"== sshcエンジンの記録 ==\n12:00:00 [debug1] 起動します", "== コンテナのログ ==\n（ありません）"} {
		if !strings.Contains(shown, want) {
			t.Fatalf("shown = %q", shown)
		}
	}
}

// 接続ログに出す docker の引数は、長すぎるものを切る。
func TestDescribedArgumentsAreCut(t *testing.T) {
	described := describeArguments([]string{"run", strings.Repeat("a", maxDescribedArgumentBytes)})

	if len(described) > maxDescribedArgumentBytes+len("…") || !strings.HasSuffix(described, "…") {
		t.Fatalf("described = %q", described)
	}
}

// connect が行ったことは debug2 に写し、繋げなかったときは出力も debug2 に写す。
// socat の行は時刻を外して写す。コンテナの時計は UTC で、記録の現地時刻と並べると
// 食い違って見える。
func TestConnectNotesAndFailureOutputReachTheConnectionLog(t *testing.T) {
	var record attemptRecord
	ctx := connectionlog.With(context.Background(), &record)
	watch := newConnectWatch()
	_, _ = watch.Write([]byte("sshc-vpn-note: 接続先 db を名前解決しました：10.0.0.5\n"))
	_, _ = watch.Write([]byte("2026/09/24 10:23:31 socat[7] E connect(): Connection refused\n"))

	watch.describe(ctx)

	text := record.text()
	for _, want := range []string{"[debug2] コンテナの中継：接続先 db を名前解決しました：10.0.0.5", "[debug2]   socat[7] E connect(): Connection refused"} {
		if !strings.Contains(text, want) {
			t.Fatalf("record に %q が無い:\n%s", want, text)
		}
	}
	if strings.Contains(text, "10:23:31 socat") {
		t.Fatalf("socat の時刻を外していない:\n%s", text)
	}
}

// 設定に関係なく出す行は、接続ログと同じく [sshc] の印で記録する。
func TestNoticesAreRecordedWithTheSshcMark(t *testing.T) {
	var record attemptRecord
	ctx := connectionlog.With(context.Background(), &record)

	connectionlog.Say(ctx, connectionlog.Notice, "VPNのコンテナイメージを作成しています。")

	if text := record.text(); !strings.Contains(text, " [sshc] VPNのコンテナイメージを作成しています。") {
		t.Fatalf("record:\n%s", text)
	}
}
