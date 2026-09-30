package remotesync_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/remotesync"
)

// autoFor は、巡回を 1 つ組み立てる。走らせるのはテストが自分で一巡させる形に
// してあるので、時計に依存しない。
func autoFor(t *testing.T, machine installation, enabled bool) *remotesync.Auto {
	t.Helper()
	auto := remotesync.NewAuto(machine.service, time.Minute, func() string { return "2026-08-18T00:00:00Z" })
	auto.Enabled = func() bool { return enabled }
	auto.Key = func() (string, bool) { return syncPassphrase, true }
	auto.ReportFailure = func(stage string, err error) { t.Logf("auto failure at %s: %v", stage, err) }
	return auto
}

// once は製品と同じ順で 1 巡させる: Poll で受信し、止まらなければ予約された
// 送信をその場で流す。Run では受信で止まった巡回は送信の timer を仕掛けない。
func once(t *testing.T, auto *remotesync.Auto) remotesync.AutoView {
	t.Helper()
	view := auto.Poll(context.Background())
	if view.Phase != remotesync.AutoIdle {
		return view
	}
	return auto.SendScheduled(context.Background())
}

// runStepTimeout は、Run の goroutine が 1 歩進むのを待つ上限。期限の判定には
// 使わない。超えたら Run が止まっているので、テストを止める。
const runStepTimeout = 5 * time.Second

// manualClock は、テストが進めるまで止まっている時計である。Run の goroutine からも
// 読むので、錠で守る。
type manualClock struct {
	mutex sync.Mutex
	now   time.Time
}

func newManualClock() *manualClock {
	return &manualClock{now: time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)}
}

func (c *manualClock) Now() time.Time {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.now
}

func (c *manualClock) Advance(elapsed time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.now = c.now.Add(elapsed)
}

// manualPushTimer は、Run が送信の期限を掛けるたびにその待ちをテストへ渡し、
// テストが発火させるまで発火しない timer である。
type manualPushTimer struct {
	fired chan time.Time
	armed chan time.Duration
}

func newManualPushTimer() *manualPushTimer {
	return &manualPushTimer{fired: make(chan time.Time), armed: make(chan time.Duration, 8)}
}

func (m *manualPushTimer) start(delay time.Duration) remotesync.PushTimerForTest {
	m.armed <- delay
	return m
}

func (m *manualPushTimer) Fired() <-chan time.Time { return m.fired }

func (m *manualPushTimer) Reset(delay time.Duration) { m.armed <- delay }

func (m *manualPushTimer) Stop() {}

// nextArmed は、Run が次に掛けた期限までの待ちを返す。
func (m *manualPushTimer) nextArmed(t *testing.T) time.Duration {
	t.Helper()
	select {
	case delay := <-m.armed:
		return delay
	case <-time.After(runStepTimeout):
		t.Fatal("Run did not arm the push deadline")
		return 0
	}
}

// armedAgain は、テストが受け取っていない期限が残っているかを返す。
func (m *manualPushTimer) armedAgain() bool {
	select {
	case <-m.armed:
		return true
	default:
		return false
	}
}

// fire は、期限が来たことを Run へ届ける。Run が受け取ってから戻る。
func (m *manualPushTimer) fire(t *testing.T, now time.Time) {
	t.Helper()
	select {
	case m.fired <- now:
	case <-time.After(runStepTimeout):
		t.Fatal("Run was not waiting for the push deadline")
	}
}

// waitForLiveObject は、Run の送信が live object を置くまで待つ。Run を止めるのは
// そのあとにする。止めると ctx が切れ、途中の送信も打ち切られる。
func waitForLiveObject(t *testing.T, bucket *fakeBucket) {
	t.Helper()
	deadline := time.Now().Add(runStepTimeout)
	for len(bucket.object(remotesync.ObjectName)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the scheduled push never wrote the live object")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// runAuto は auto.Run を別の goroutine で走らせ、止めて終わりを待つ関数を返す。
func runAuto(auto *remotesync.Auto) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		auto.Run(ctx)
	}()
	return sync.OnceFunc(func() {
		cancel()
		<-done
	})
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

	view := once(t, auto)
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

func TestManualNowRunsWhenAutomaticSyncIsOff(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	auto := autoFor(t, machine, false)

	view := auto.Now(context.Background())
	if view.Phase != remotesync.AutoIdle {
		t.Fatalf("view = %+v, want idle", view)
	}
	if len(bucket.object(remotesync.ObjectName)) == 0 {
		t.Fatal("manual sync did not write the live object while automatic sync was off")
	}
	if auto.View().Enabled {
		t.Fatal("manual sync changed the automatic sync preference")
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

func TestAutomaticPollQueuesLocalChangesWithoutUploadingImmediately(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	auto := autoFor(t, machine, true)
	clock := newManualClock()
	auto.SetClockForTest(clock.Now)

	view := auto.Poll(context.Background())
	if view.Phase != remotesync.AutoIdle {
		t.Fatalf("Poll view = %+v, want idle", view)
	}
	// Poll is the once-per-minute receive path. Even after the debounce deadline,
	// it must not upload by itself; Run owns the separate push scheduler.
	clock.Advance(remotesync.AutoPushDelay)
	if keys := bucket.keys(); len(keys) != 0 {
		t.Fatalf("receive-only poll uploaded %v", keys)
	}
}

func TestAutomaticPushWaitsForFiveSecondsOfQuietAndCoalescesChanges(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	auto := autoFor(t, machine, true)
	clock := newManualClock()
	auto.SetClockForTest(clock.Now)
	timer := newManualPushTimer()
	auto.SetPushTimerForTest(timer.start)
	stop := runAuto(auto)
	t.Cleanup(stop)

	// 変更のたびに、期限はその変更から AutoPushDelay 後へ動く。
	for range 3 {
		auto.NotifyLocalChange()
		if delay := timer.nextArmed(t); delay != remotesync.AutoPushDelay {
			t.Fatalf("a change armed the push after %v, want %v", delay, remotesync.AutoPushDelay)
		}
		clock.Advance(2 * time.Second)
	}
	// 最初の変更から AutoPushDelay を過ぎても、最後の変更からはまだ 1 秒残っている。
	// 早く起きた Run は送らず、残りを待ち直す。
	clock.Advance(2 * time.Second)
	timer.fire(t, clock.Now())
	if delay := timer.nextArmed(t); delay != time.Second {
		t.Fatalf("Run waits %v after waking early, want the 1s left from the last change", delay)
	}
	if keys := bucket.keys(); len(keys) != 0 {
		t.Fatalf("push happened before the latest quiet deadline: %v", keys)
	}

	clock.Advance(time.Second)
	timer.fire(t, clock.Now())
	waitForLiveObject(t, bucket)
	stop()
	if objects, _ := bucket.uploads(); objects != 2 {
		t.Fatalf("one push stored %d objects, want one live and one history object", objects)
	}
	if timer.armedAgain() {
		t.Fatal("coalesced notifications scheduled another push")
	}
}

func TestDebouncedPushReadsSyncSecretsInsideTheUnattendedFrame(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	auto := autoFor(t, machine, true)
	clock := newManualClock()
	auto.SetClockForTest(clock.Now)
	timer := newManualPushTimer()
	auto.SetPushTimerForTest(timer.start)
	var inside atomic.Bool
	var readOutside atomic.Bool
	auto.Unattended = func(run func()) {
		inside.Store(true)
		run()
		inside.Store(false)
	}
	auto.Enabled = func() bool {
		if !inside.Load() {
			readOutside.Store(true)
		}
		return true
	}

	stop := runAuto(auto)
	t.Cleanup(stop)
	auto.NotifyLocalChange()
	timer.nextArmed(t)
	clock.Advance(remotesync.AutoPushDelay)
	timer.fire(t, clock.Now())
	waitForLiveObject(t, bucket)
	stop()
	if readOutside.Load() {
		t.Fatal("debounced push read automatic sync settings outside the unattended frame")
	}
}

func TestAutomaticOperationsPreparePersistedConfigurationBeforeNetworkUse(t *testing.T) {
	bucket := &fakeBucket{}
	machine := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	auto := autoFor(t, machine, true)
	prepared := 0
	auto.Prepare = func() { prepared++ }

	auto.Poll(context.Background())
	if prepared != 1 {
		t.Fatalf("Prepare called %d times, want 1", prepared)
	}
}

func TestSendOnlyAutoStopsBeforeUploadingWhenRemoteMoved(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "remote\n"})
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), "Remote setup"); err != nil {
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
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
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

func TestLiveReplayIsBlockedButExplicitHistoryRestoreRemainsAvailable(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host first\n"})
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), "First"); err != nil {
		t.Fatal(err)
	}
	firstCiphertext := append([]byte(nil), bucket.object(remotesync.ObjectName)...)

	consumer := newInstallation(t, bucket, map[string]string{})
	auto := autoFor(t, consumer, true)
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("initial receive = %+v", view)
	}

	producer.write(t, "config", "Host second\n")
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), "Second"); err != nil {
		t.Fatal(err)
	}
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("descendant receive = %+v", view)
	}
	if got := consumer.read(t, "config"); got != "Host second\n" {
		t.Fatalf("config before replay = %q", got)
	}

	// A bucket writer can replay observed ciphertext without knowing the key.
	bucket.putObject(remotesync.ObjectName, firstCiphertext, `"replayed"`)
	if _, err := consumer.service.Pull(context.Background(), syncPassphrase, remotesync.ResolveNone); !errors.Is(err, remotesync.ErrRemoteMoved) {
		t.Fatalf("Pull replay = %v, want ErrRemoteMoved", err)
	}
	if view := once(t, auto); view.Phase != remotesync.AutoBlocked || view.Detail != "remote_moved" {
		t.Fatalf("auto replay = %+v, want blocked remote_moved", view)
	}
	if got := consumer.read(t, "config"); got != "Host second\n" {
		t.Fatalf("replay changed config to %q", got)
	}

	var firstHistoryKey string
	for _, key := range bucket.keys() {
		if strings.Contains(key, remotesync.SnapshotPrefix) && bytes.Equal(bucket.object(key), firstCiphertext) {
			firstHistoryKey = key
			break
		}
	}
	if firstHistoryKey == "" {
		t.Fatal("first immutable history snapshot was not found")
	}
	preview, err := consumer.service.PullHistory(
		context.Background(), syncPassphrase, firstHistoryKey, remotesync.ResolveRemote,
	)
	if err != nil {
		t.Fatalf("PullHistory = %v", err)
	}
	if err := applyPreview(consumer.service, remotesync.ResolveRemote, firstHistoryKey, preview); err != nil {
		t.Fatalf("Apply explicit history = %v", err)
	}
	auto.ManualApplyCompleted()
	if view := auto.View(); view.Phase != remotesync.AutoIdle || view.Detail != "" {
		t.Fatalf("manual apply left a stale blocked view: %+v", view)
	}
	if got := consumer.read(t, "config"); got != "Host first\n" {
		t.Fatalf("explicit history restored %q", got)
	}
}

func TestAutoAcceptsAProvenMultiGenerationDescendant(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host first\n"})
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), "First"); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{})
	auto := autoFor(t, consumer, true)
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("initial receive = %+v", view)
	}

	for _, value := range []string{"Host second\n", "Host third\n"} {
		producer.write(t, "config", value)
		if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), strings.TrimSpace(value)); err != nil {
			t.Fatal(err)
		}
	}
	// More than the graph window may exist in the bucket. Truncation alone is
	// not a reason to block when every required link is still among the newest
	// 50 authenticated objects.
	for index := range 51 {
		bucket.putObject(
			fmt.Sprintf("%s0000-older-%02d", remotesync.SnapshotPrefix, index),
			[]byte("not an envelope"), fmt.Sprintf(`"older-%02d"`, index),
		)
	}
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("multi-generation receive = %+v", view)
	}
	if got := consumer.read(t, "config"); got != "Host third\n" {
		t.Fatalf("config = %q", got)
	}
}

func TestAutoLineageProofDecodesRepeatedCiphertextOnlyOnce(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host first\n"})
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), "First"); err != nil {
		t.Fatal(err)
	}
	firstCiphertext := append([]byte(nil), bucket.object(remotesync.ObjectName)...)
	consumer := newInstallation(t, bucket, map[string]string{})
	auto := autoFor(t, consumer, true)
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("initial receive = %+v", view)
	}
	for _, value := range []string{"Host second\n", "Host third\n"} {
		producer.write(t, "config", value)
		if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), strings.TrimSpace(value)); err != nil {
			t.Fatal(err)
		}
	}
	for index := range 12 {
		bucket.putObject(fmt.Sprintf("%szz-duplicate-%02d", remotesync.SnapshotPrefix, index), firstCiphertext, fmt.Sprintf(`"duplicate-%02d"`, index))
	}
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("receive behind repeated ciphertext = %+v", view)
	}
	if got := consumer.read(t, "config"); got != "Host third\n" {
		t.Fatalf("config = %q", got)
	}
}

func TestAutoUsesAuthenticatedAncestorsBeyondLegacyCiphertextBudget(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host first\n"})
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), "First"); err != nil {
		t.Fatal(err)
	}
	firstCiphertext := append([]byte(nil), bucket.object(remotesync.ObjectName)...)
	consumer := newInstallation(t, bucket, map[string]string{})
	auto := autoFor(t, consumer, true)
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("initial receive = %+v", view)
	}
	for _, value := range []string{"Host second\n", "Host third\n"} {
		producer.write(t, "config", value)
		if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), strings.TrimSpace(value)); err != nil {
			t.Fatal(err)
		}
	}
	archive, key, err := envelope.OpenRemote(firstCiphertext, syncPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Destroy()
	for index := range 9 {
		resealed, sealErr := key.Seal(archive)
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		bucket.putObject(fmt.Sprintf("%szz-distinct-%02d", remotesync.SnapshotPrefix, index), resealed, fmt.Sprintf(`"distinct-%02d"`, index))
	}
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("receive beyond legacy lineage budget = %+v", view)
	}
	if got := consumer.read(t, "config"); got != "Host third\n" {
		t.Fatalf("authenticated lineage did not advance config: %q", got)
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
	if _, err := first.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
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
	if _, err := first.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
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
	if _, err := first.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
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
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
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

func TestAutoBlocksWhenAnAcknowledgedLiveObjectWasDeleted(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{})
	auto := autoFor(t, consumer, true)
	if view := once(t, auto); view.Phase != remotesync.AutoIdle {
		t.Fatalf("initial receive = %+v", view)
	}
	bucket.removeObject(remotesync.ObjectName)
	view := once(t, auto)
	if view.Phase != remotesync.AutoBlocked || view.Detail != "remote_deleted" {
		t.Fatalf("deleted live = %+v, want blocked remote_deleted", view)
	}
}

func TestAutoCachesAnUnreadableRemoteGeneration(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{})
	auto := autoFor(t, consumer, true)
	key := "a wrong but sufficiently long synchronization key"
	auto.Key = func() (string, bool) { return key, true }
	first := once(t, auto)
	if first.Phase != remotesync.AutoFailed || first.Detail != "wrong_passphrase" {
		t.Fatalf("first unreadable generation = %+v", first)
	}
	downloads := bucket.downloads()
	second := once(t, auto)
	if second.Phase != remotesync.AutoFailed || second.Detail != "wrong_passphrase" {
		t.Fatalf("cached unreadable generation = %+v", second)
	}
	if got := bucket.downloads(); got != downloads {
		t.Fatalf("same unreadable generation downloaded again: %d then %d", downloads, got)
	}
	key = syncPassphrase
	third := once(t, auto)
	if third.Phase != remotesync.AutoIdle {
		t.Fatalf("corrected key did not retry the same ETag: %+v", third)
	}
	if got := bucket.downloads(); got != downloads+1 {
		t.Fatalf("corrected key downloads = %d, want %d", got, downloads+1)
	}
}

func TestAutoReportsTheInternalFailureStageWithoutChangingTheSafeView(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{})
	auto := autoFor(t, consumer, true)
	auto.Key = func() (string, bool) {
		return "a wrong but sufficiently long synchronization key", true
	}
	stage := ""
	auto.ReportFailure = func(got string, err error) {
		stage = got
		if !errors.Is(err, remotesync.ErrWrongPassphrase) {
			t.Errorf("reported error = %v", err)
		}
	}

	view := once(t, auto)
	if stage != "pull" {
		t.Fatalf("reported stage = %q, want pull", stage)
	}
	if view.Phase != remotesync.AutoFailed || view.Detail != "wrong_passphrase" {
		t.Fatalf("safe view = %+v", view)
	}
}

func TestAutoRetriesATransientFailureAfterBoundedBackoff(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{})
	auto := remotesync.NewAuto(consumer.service, 10*time.Millisecond, func() string { return "2026-08-18T00:00:00Z" })
	current := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	auto.SetClockForTest(func() time.Time { return current })
	auto.Enabled = func() bool { return true }
	auto.Key = func() (string, bool) { return syncPassphrase, true }
	bucket.refuseObjectGets(3)
	if first := once(t, auto); first.Phase != remotesync.AutoFailed || first.Detail != "bucket_unavailable" {
		t.Fatalf("first transient failure = %+v", first)
	}
	downloads := bucket.downloads()
	if second := once(t, auto); second.Phase != remotesync.AutoFailed || second.Detail != "bucket_unavailable" {
		t.Fatalf("backoff view = %+v", second)
	}
	if got := bucket.downloads(); got != downloads {
		t.Fatalf("transient failure retried before backoff: %d then %d", downloads, got)
	}
	current = current.Add(20 * time.Millisecond)
	if third := once(t, auto); third.Phase != remotesync.AutoIdle {
		t.Fatalf("transient failure did not expire: %+v", third)
	}
	if got := bucket.downloads(); got != downloads+1 {
		t.Fatalf("retry downloads = %d, want %d", got, downloads+1)
	}
}

func TestAutoConfigurationResetClearsFailureEvidence(t *testing.T) {
	bucket := &fakeBucket{}
	producer := newInstallation(t, bucket, map[string]string{"config": "Host bastion\n"})
	if _, err := producer.service.PushUsing(context.Background(), keyOf(syncPassphrase), ""); err != nil {
		t.Fatal(err)
	}
	consumer := newInstallation(t, bucket, map[string]string{})
	auto := autoFor(t, consumer, true)
	auto.Key = func() (string, bool) { return "a wrong but sufficiently long synchronization key", true }
	if view := once(t, auto); view.Phase != remotesync.AutoFailed {
		t.Fatalf("first view = %+v", view)
	}
	downloads := bucket.downloads()
	auto.ResetRemoteCache()
	if view := once(t, auto); view.Phase != remotesync.AutoFailed {
		t.Fatalf("view after reset = %+v", view)
	}
	if got := bucket.downloads(); got != downloads+1 {
		t.Fatalf("configuration reset kept stale failure cache: %d then %d", downloads, got)
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

	once(t, auto)
	if !sawKey {
		t.Fatal("the cycle read the key outside the unattended frame")
	}
	if inside {
		t.Fatal("the frame was never closed")
	}
}
