package mobile

import (
	"errors"
	"strings"
	"testing"
)

// 起こしたものは必ず止める。t.Cleanup に置くのは、表明が失敗した経路でも
// engine lock が解けるようにするためである。
func started(t *testing.T) string {
	t.Helper()
	url, err := Start(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	t.Cleanup(func() { _ = Stop() })
	return url
}

// Start が返るのは、listener が bind され、入口が発行された後である。
// **早く返してはならない** — 呼び出し側はその URL を即座に WebView へ渡す。
func TestStartReturnsAnEntranceThatIsAlreadyServing(t *testing.T) {
	url := started(t)
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Errorf("URL = %q, want a loopback entrance", url)
	}
	if !strings.Contains(url, "/#bootstrap=") {
		t.Errorf("URL = %q, want a one-time bootstrap fragment", url)
	}
}

// **1 プロセスに engine は 1 台である。** Activity が作り直されるたびに
// もう 1 台起きれば、2 台目が engine lock で落ちるまでの間、同じ状態
// ディレクトリを 2 つのプロセスが握る。
func TestStartRefusesASecondEngine(t *testing.T) {
	started(t)
	if _, err := Start(t.TempDir(), t.TempDir()); !errors.Is(err, ErrAlreadyStarted) {
		t.Errorf("second Start = %v, want ErrAlreadyStarted", err)
	}
}

// 止めた後は、また起こせる。foreground service が落とされて作り直される経路が
// これであり、ここが片道なら 2 度目の起動がアプリの再インストールを要求する。
func TestStopLetsTheNextStartSucceed(t *testing.T) {
	home, cache := t.TempDir(), t.TempDir()
	if _, err := Start(home, cache); err != nil {
		t.Fatalf("first Start = %v", err)
	}
	if err := Stop(); err != nil {
		t.Fatalf("Stop = %v", err)
	}
	url, err := Start(home, cache)
	if err != nil {
		t.Fatalf("second Start = %v", err)
	}
	t.Cleanup(func() { _ = Stop() })
	if url == "" {
		t.Error("the second Start announced no entrance")
	}
}

func TestStopWithoutStartIsAnError(t *testing.T) {
	if err := Stop(); !errors.Is(err, ErrNotStarted) {
		t.Errorf("Stop = %v, want ErrNotStarted", err)
	}
}

// **Go の error を Kotlin へ渡して番号に畳ませない。** gomobile は Go の error
// を Java の Exception へ写すときメッセージ文字列しか運ばないので、戻ってきた
// 値に errors.Is は効かない。理由は Go 側で確定させ、Kotlin は番号だけを取りに
// 来る。engine の error は入口の URL を含み得るので、文面を渡さないことには
// それ自体の意味もある。
func TestTheFailureKindSurvivesWhereTheErrorWouldNot(t *testing.T) {
	started(t)
	if _, err := Start(t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("a second Start succeeded")
	}
	if got := LastStartFailureKind(); got != kindAlreadyStarted {
		t.Errorf("LastStartFailureKind() = %d, want %d", got, kindAlreadyStarted)
	}
}

// 成功したら理由は残らない。前回の失敗が残っていると、Kotlin は起動した後で
// エラー画面を出す。
func TestASuccessfulStartClearsTheLastFailure(t *testing.T) {
	started(t)
	if got := LastStartFailureKind(); got != kindNone {
		t.Errorf("LastStartFailureKind() = %d, want %d", got, kindNone)
	}
}
