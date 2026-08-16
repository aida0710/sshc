package mobile

import (
	"log/slog"
	"slices"
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
	if got := build(t).Owner; got != handoff.OwnerDesktop {
		t.Errorf("Owner = %q, want %q", got, handoff.OwnerDesktop)
	}
}
