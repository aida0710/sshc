package remotesync_test

import (
	"context"
	"testing"
	"time"

	"sshc/internal/remotesync"
)

// autoFor は、巡回を 1 つ組み立てる。走らせるのはテストが自分で一巡させる形に
// してあるので、時計に依存しない。
func autoFor(t *testing.T, machine installation, enabled bool) *remotesync.Auto {
	t.Helper()
	auto := remotesync.NewAuto(machine.service, time.Minute, func() string { return "2026-08-18T00:00:00Z" })
	auto.Enabled = func() bool { return enabled }
	auto.Key = func() (string, bool) { return syncPassphrase, true }
	return auto
}

// once は一巡させ、その結果を返す。
func once(t *testing.T, auto *remotesync.Auto) remotesync.AutoView {
	t.Helper()
	return auto.Once(context.Background())
}

// UI は engine 起動直後、一巡目のtickerより先にstatusを読む。ゼロ値の空文字は
// APIのphaseではないため、まだ一度も走っていなくてもidleと答える。
func TestAutoStartsIdleBeforeItsFirstCycle(t *testing.T) {
	auto := remotesync.NewAuto(nil, time.Minute, func() string { return "unused" })
	view := auto.View()
	if view.Phase != remotesync.AutoIdle || view.Enabled {
		t.Fatalf("initial view = %+v, want disabled idle", view)
	}
}

// 入っていなければ、巡回は何もしない。バケットへ 1 本も投げない。
func TestAutoDoesNothingWhenItIsNotOn(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	auto := autoFor(t, machine, false)

	view := auto.Once(context.Background())
	if view.Phase == remotesync.AutoRunning {
		t.Fatalf("view = %+v", view)
	}
	if keys := bucket.keys(); len(keys) != 0 {
		t.Fatalf("an installation with auto sync off wrote %v", keys)
	}
	if auto.View().Enabled {
		t.Fatalf("view = %+v", auto.View())
	}
}

// 押すユーザーが居なくても、変わったものは出ていく。
func TestAutoPushesWhatChangedHere(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	auto := autoFor(t, machine, true)

	view := once(t, auto)
	if view.Phase != remotesync.AutoIdle {
		t.Fatalf("view = %+v", view)
	}
	if len(bucket.object(remotesync.ObjectName)) == 0 {
		t.Fatal("the live object was never written")
	}
	// 二巡目は何も送らない。変わっていないものを押し続ける巡回は、他の端末を
	// 毎分起動するだけである。
	before, _ := bucket.uploads()
	_ = once(t, auto)
	after, _ := bucket.uploads()
	if after != before {
		t.Fatalf("a second cycle uploaded again: %d then %d", before, after)
	}
}

func TestSendOnlyAutoStopsBeforeUploadingWhenRemoteMoved(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "remote\n"})
	if _, err := producer.service.Push(context.Background(), syncPassphrase, "Remote setup"); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{"config": "local\n"})
	consumer.direct(remotesync.DirectionPush)
	auto := autoFor(t, consumer, true)
	before, _ := bucket.uploads()

	view := once(t, auto)
	if view.Phase != remotesync.AutoBlocked || view.Detail != "remote_moved" {
		t.Fatalf("view = %+v, want blocked remote_moved", view)
	}
	if after, _ := bucket.uploads(); after != before {
		t.Fatalf("send-only check uploaded before resolving remote: %d objects became %d", before, after)
	}

	view = once(t, auto)
	if view.Phase != remotesync.AutoBlocked || view.Detail != "remote_moved" {
		t.Fatalf("second view = %+v, want blocked remote_moved", view)
	}
	if after, _ := bucket.uploads(); after != before {
		t.Fatalf("blocked retry uploaded: %d objects became %d", before, after)
	}
}

// 向こうが進んでいれば、押されなくても取り込む。
func TestAutoAppliesWhatAnotherMachinePushed(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := producer.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{})
	auto := autoFor(t, consumer, true)

	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("view = %+v", view)
	}
	if got := consumer.read(t, "config"); got != "Host bastion\n" {
		t.Fatalf("config = %q", got)
	}
}

// 消えるものがあるなら、巡回は止まる。置き換えは History から戻せるが、
// 消えたファイルは画面から消える。そこはユーザーが見る分岐である。
func TestAutoStopsInsteadOfRemovingFiles(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{
		"config":               "Host bastion\n",
		"connections/old.conf": "Host old\n",
	})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}
	second := newInstallation(t, bucket, map[string]string{})
	auto := autoFor(t, second, true)
	// 2 台目が両方受け取り、そこが基準になる。
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("first cycle = %+v", view)
	}

	// 1 台目が片方を消して押し出す。
	first.remove(t, "connections/old.conf")
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}

	view := once(t, auto)
	if view.Phase != remotesync.AutoBlocked || view.Detail != "removals" {
		t.Fatalf("view = %+v, want blocked on removals", view)
	}
	if got := second.read(t, "connections/old.conf"); got != "Host old\n" {
		t.Fatalf("the file was removed without anyone saying so: %q", got)
	}
}

// 両側で変わったものは、巡回が選ばない。
func TestAutoStopsOnAConflict(t *testing.T) {
	bucket := &fakeBucket{}
	first := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := first.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}
	second := newInstallation(t, bucket, map[string]string{"config": "Host something else\n"})
	auto := autoFor(t, second, true)

	view := once(t, auto)
	if view.Phase != remotesync.AutoBlocked || view.Detail != "conflicts" {
		t.Fatalf("view = %+v, want blocked on conflicts", view)
	}
	if got := second.read(t, "config"); got != "Host something else\n" {
		t.Fatalf("the local file was overwritten: %q", got)
	}

	// 同じremote世代についてはHEADだけで止まり、snapshotのdownloadとKDFを
	// 1分ごとに繰り返さない。
	downloads := bucket.downloads()
	view = once(t, auto)
	if view.Phase != remotesync.AutoBlocked || view.Detail != "conflicts" {
		t.Fatalf("second blocked view = %+v", view)
	}
	if got := bucket.downloads(); got != downloads {
		t.Fatalf("blocked cycle downloaded again: %d then %d", downloads, got)
	}
}

func TestAutoAcknowledgesAnUnchangedRemoteGeneration(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := producer.service.Push(context.Background(), syncPassphrase, ""); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	auto := autoFor(t, consumer, true)

	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("first cycle = %+v", view)
	}
	downloads := bucket.downloads()
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("second cycle = %+v", view)
	}
	if got := bucket.downloads(); got != downloads {
		t.Fatalf("the acknowledged generation downloaded again: %d then %d", downloads, got)
	}
}

// 巡回は、渡された枠の中で走る。枠がその外へ漏れれば、保管庫は開けっぱなしに
// なるので、包んでいることそのものを見る。
func TestEveryCycleRunsInsideTheUnattendedFrame(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	auto := autoFor(t, machine, true)

	inside := false
	sawKey := false
	auto.Unattended = func(run func()) {
		inside = true
		run()
		inside = false
	}
	auto.Key = func() (string, bool) {
		// 鍵を読むのは巡回の一部である。ここが枠の外に出ていれば、1 分ごとの
		// 読み取りがアイドルの時計を戻し続ける。
		sawKey = inside
		return syncPassphrase, true
	}

	auto.Once(context.Background())
	if !sawKey {
		t.Fatal("the cycle read the key outside the unattended frame")
	}
	if inside {
		t.Fatal("the frame was never closed")
	}
}
