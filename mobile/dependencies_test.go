package mobile

import (
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"

	"sshc/internal/app"
	"sshc/internal/handoff"
)

func build(t *testing.T) app.Dependencies {
	t.Helper()
	dependencies, err := newDependencies("/data/user/0/app/files", "/data/user/0/app/cache",
		slog.Default(), func(app.Readiness) error { return nil })
	if err != nil {
		t.Fatalf("newDependencies = %v", err)
	}
	return dependencies
}

// **このアプリケーションは、自分を置き換える経路を Android で持たない。**
// Checker を配線したままにすると、置き換えられない更新を提示する画面が出る。
func TestAndroidOffersNoSelfUpdate(t *testing.T) {
	if build(t).Updates != nil {
		t.Error("the Android engine offers a self-update it cannot perform")
	}
}

// ssh-keygen も ssh-agent も Android に居ない。どちらも interface なので、
// **nil がここでの正解であり、配線し忘れではない。** keys.CatalogueReader は
// nil の Toolchain でハードウェア鍵を足さずに返し、keys.Service は nil の
// agent を「到達できるエージェントが無い」として扱う。
func TestAndroidHasNeitherToolchainNorAgent(t *testing.T) {
	dependencies := build(t)
	if dependencies.Toolchain != nil {
		t.Error("the Android engine claims an ssh-keygen it cannot find")
	}
	if dependencies.KeyAgent != nil {
		t.Error("the Android engine claims an ssh-agent it cannot reach")
	}
}

// **端末は、それを起こしたものの事情を継がない。** Android アプリの環境に
// 有用な PATH は無く、os.Environ を渡せば /system/bin すら見えないシェルが開く。
func TestAndroidTerminalInheritsAFixedEnvironment(t *testing.T) {
	got := build(t).Environ()
	want := []string{
		"HOME=/data/user/0/app/files",
		"PATH=/system/bin:/system/xbin",
		"TERM=xterm-256color",
		"TMPDIR=/data/user/0/app/cache",
	}
	if !slices.Equal(got, want) {
		t.Errorf("Environ() = %q, want %q", got, want)
	}
}

// SHELL は読まない。読めば、たまたま設定されていた値がログインシェルの
// 権威になる——Android では誰もそれを選んでいない。fallback の一覧だけが答える。
func TestAndroidResolvesTheShellFromFallbacksAlone(t *testing.T) {
	if _, ok := build(t).Lookup("SHELL"); ok {
		t.Error("the Android engine reads SHELL from an environment nobody chose")
	}
}

// **新しい owner 値を足さない。** 欲しいのは「bootstrap fragment 付きの URL を
// Announce が返す」という desktop の挙動そのものである。
func TestAndroidAnnouncesAsTheDesktopOwner(t *testing.T) {
	if got := build(t).Owner; got != handoff.OwnerEngine {
		t.Errorf("Owner = %q, want %q", got, handoff.OwnerEngine)
	}
}

// androidFieldIntent は、app.Dependencies のすべての項目について、Android の
// engine がそれを配線するのかしないのかと、その理由を並べたものである。
//
// **これは書き写しではなく、決定の一覧である。** Android の依存は struct literal
// で組まれるので、app.Dependencies に項目が増えても Go は何も言わず、新しい項目は
// 黙って零値になる。零値が正しい答えであることも、配線を忘れただけであることも
// あり、型はその二つを区別しない。実際にそうやって落ちた項目があり、コメントは
// 落としたものを 4 つと書いたまま 6 つになっていた。
//
// 下の表に無い項目が現れたら、このテストは失敗する。**そのとき求められているのは
// 表に一行足すことではなく、Android でその項目をどうするかを決めることである。**
var androidFieldIntent = map[string]string{
	// 配線する。
	"Random":   "wired: crypto/rand",
	"Announce": "wired: 入口の URL を Java 側へ渡す",
	"Listen":   "wired: net.Listen",
	"UI":       "wired: 埋め込んだ SPA",
	"Logger":   "wired: 呼び出し元が渡す",
	"Home":     "wired: アプリの filesDir",
	"Owner":    "wired: handoff.OwnerEngine",
	"PID":      "wired: このプロセス",
	"Lookup":   "wired: 常に見つからないと答える。SHELL を偶然の権威にしない",
	"Environ":  "wired: 固定の環境。Android アプリの環境に有用な PATH が無い",

	// 意図して空にする。
	"Toolchain": "absent: ssh-keygen が Android に居ない",
	"KeyAgent":  "absent: ssh-agent が Android に居ない",
	"Updates":   "absent: バイナリを置き換える経路が無い",

	// 空で既定に落ちるのが正しいもの。
	"ScanHostKeys":    "default: internal/sshclient がこのプロセスの中で話す",
	"Probe":           "default: 同上",
	"RemoteRun":       "default: 同上",
	"TerminalStarter": "default: 本物の PTY を確保する。Android では /system/bin/sh",
	"SessionNow":      "default: time.Now",
	"ShutdownTimeout": "default: app が決める既定値",
}

func TestEveryDependencyOfTheAndroidEngineIsADecision(t *testing.T) {
	structure := reflect.TypeOf(app.Dependencies{})
	values := reflect.ValueOf(build(t))

	for index := range structure.NumField() {
		name := structure.Field(index).Name
		intent, known := androidFieldIntent[name]
		if !known {
			t.Errorf("app.Dependencies.%s について Android の engine が何をするか決まっていない。"+
				"配線するのか、意図して空にするのか、既定に落とすのかを決めて androidFieldIntent に書くこと", name)
			continue
		}
		// 表が実態からずれていないことも見る。ずれた表は、無い表よりも悪い。
		if wired := !values.Field(index).IsZero(); wired != strings.HasPrefix(intent, "wired:") {
			t.Errorf("app.Dependencies.%s: 表は %q と言うが、実際は wired=%v", name, intent, wired)
		}
	}

	for name := range androidFieldIntent {
		if _, ok := structure.FieldByName(name); !ok {
			t.Errorf("androidFieldIntent に app.Dependencies から消えた項目 %s が残っている", name)
		}
	}
}
