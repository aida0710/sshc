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
	dependencies, err := newDependencies("android", "/data/user/0/app/files", "/data/user/0/app/cache",
		slog.Default(), func(app.Readiness) error { return nil })
	if err != nil {
		t.Fatalf("newDependencies = %v", err)
	}
	return dependencies
}

// Android では実行できない自己更新を提示しないことを検証する。
func TestAndroidOffersNoSelfUpdate(t *testing.T) {
	if build(t).Updates != nil {
		t.Error("the Android engine offers a self-update it cannot perform")
	}
}

// Android に存在しない ssh-keygen と ssh-agent が配線されないことを検証する。
func TestAndroidHasNeitherToolchainNorAgent(t *testing.T) {
	dependencies := build(t)
	if dependencies.Toolchain != nil {
		t.Error("the Android engine claims an ssh-keygen it cannot find")
	}
	if dependencies.KeyAgent != nil {
		t.Error("the Android engine claims an ssh-agent it cannot reach")
	}
}

// Android 端末に固定の安全な環境変数を渡すことを検証する。
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

// Android アプリの SHELL 環境変数に依存しないことを検証する。
func TestAndroidResolvesTheShellFromFallbacksAlone(t *testing.T) {
	if _, ok := build(t).Lookup("SHELL"); ok {
		t.Error("the Android engine reads SHELL from an environment nobody chose")
	}
}

// Android でも通常の engine owner を使用することを検証する。
func TestAndroidAnnouncesAsTheDesktopOwner(t *testing.T) {
	if got := build(t).Owner; got != handoff.OwnerEngine {
		t.Errorf("Owner = %q, want %q", got, handoff.OwnerEngine)
	}
}

// androidFieldIntent は、app.Dependencies の各項目を Android で配線するか、
// 意図的に空にするか、既定値へ委ねるかを記録する。
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
	"Lookup":   "wired: 常に未検出を返し、SHELL の値には依存しない",
	"Environ":  "wired: 固定の環境。Android アプリの環境に有用な PATH が無い",

	// 意図して空にする。
	"Toolchain": "absent: ssh-keygen が Android に居ない",
	"KeyAgent":  "absent: ssh-agent が Android に居ない",
	"Updates":   "absent: バイナリを置き換える経路が無い",

	"StopEngine": "default: Run が自分で埋める。engine を止められるのは engine 自身だけである",
	"Port":       "default: WebView は通知された URL を使うため、ポート番号を指定させない",

	// 空で既定に落ちるのが正しいもの。
	"ScanHostKeys":    "default: internal/sshclient がプロセス内で SSH 通信する",
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
		// intent と実際の配線状態が一致することも検証する。
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

// シェルを起動できる Android だけに PATH を設定することを検証する。
func TestTheEnvironmentOnlyNamesAPathWhereOneCanBeWalked(t *testing.T) {
	for _, probe := range []struct {
		goos     string
		wantPath bool
	}{
		{goos: "android", wantPath: true},
		{goos: "ios", wantPath: false},
	} {
		environ := mobileEnvironment(probe.goos, "/home", "/cache")()
		found := false
		for _, entry := range environ {
			if strings.HasPrefix(entry, "PATH=") {
				found = true
			}
		}
		if found != probe.wantPath {
			t.Errorf("%s environment PATH present = %v, want %v: %q", probe.goos, found, probe.wantPath, environ)
		}
	}
}

// HOME、TMPDIR、TERM は Android と iOS の両方に設定する。
func TestTheEnvironmentAlwaysNamesTheHomeAndTheScratch(t *testing.T) {
	for _, goos := range []string{"android", "ios"} {
		environ := mobileEnvironment(goos, "/files", "/caches")()
		for _, want := range []string{"HOME=/files", "TMPDIR=/caches", "TERM=xterm-256color"} {
			if !slices.Contains(environ, want) {
				t.Errorf("%s environment = %q, want it to contain %q", goos, environ, want)
			}
		}
	}
}

// 呼び出し側の変更が次回の環境へ混入しないことを検証する。
func TestTheEnvironmentIsCopiedForEveryCaller(t *testing.T) {
	build := mobileEnvironment("android", "/files", "/caches")
	first := build()
	_ = append(first, "INJECTED=1")
	for _, entry := range build() {
		if entry == "INJECTED=1" {
			t.Fatal("a caller's append leaked into the next environment")
		}
	}
}
