# Android エンジン埋め込み Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Go の engine を Android アプリの同一プロセス内で起こし、WebView が既存の UI をそのまま表示する。

**Architecture:** `mobile` パッケージが `internal/app.Run` を包み、`gomobile bind` で AAR になる。Kotlin の foreground service がそれを起こし、返ってきた bootstrap URL を WebView が開く。engine の子プロセスは存在しない。

**Tech Stack:** Go 1.26.6 / gomobile bind / Android NDK 28.2.13676358 / Kotlin / Gradle / Android WebView

**Spec:** `docs/superpowers/specs/2026-08-16-android-engine-design.md`

**Worktree:** `.worktrees/android-engine`（ブランチ `android-engine`）

## 実施状況（2026-08-16）

計画から変えたところと、まだ残っているところ。

| Task | 状態 |
|---|---|
| 1 gomobile が bind できるか | **完了。倒れなかった。** Go 1.26.6 で AAR が出る。gomobile は `go.mod` の tool として固定、gobind は `go install` が要る |
| 2 `app.Dependencies` の Android 版 | 完了 |
| 3 `Start` / `Stop` | 完了。ただし失敗の伝え方を変えた（下） |
| 4 実機で残る未知数を測る | **APK まで完了。測定は利用者の実機待ち。** probe はまだ入っている |
| 5 `/system/bin/sh` | 完了 |
| 6 道具が無いときの表明 | 完了。予想どおり実装は不要で、表明だけを足した |
| 7 foreground service とエラーの畳み込み | 完了 |
| 8 CI ゲートと手順書 | 完了 |

計画から変えた判断が 3 つある。

- **Kotlin ではなく Java。** glue が 150 行なので、Kotlin プラグインのバージョン整合を持ち込む価値がない。gomobile が生成する API も Java である。
- **`StartFailureKind(err)` ではなく `LastStartFailureKind()`。** gomobile は Go の error を Java の Exception へ写すとき**メッセージ文字列しか運ばない**ので、戻ってきた値に `errors.Is` は効かない。理由は Go 側で確定させ、Kotlin は番号だけを取りに来る。
- **`platform.Toolchain` は interface だった。** 空の struct ではなく nil が正解で、`keys.CatalogueReader` は既にそれを機能の不在として扱っていた。
- **CI の step は Linux 限定ではなく Unix 側に置いた。** workflow の契約が Unix か Windows かの明示を求めるので、第三の分岐を作らない。

未知数 4（WebView から `ws://127.0.0.1` が CSP `connect-src 'self'` を通るか）は、spec を書いた後に根拠が見つかった。Electron は Chromium であり、埋め込みターミナルはそこで既に動いている。Android WebView も Chromium なので、新しい未知数ではなく M6 のスモーク項目である。

## Global Constraints

- Go 1.26.6（`go.mod` の toolchain で固定）。バージョンを上げない。
- **`CGO_ENABLED=1` 必須。** Android には `/etc/resolv.conf` が無く、pure-Go リゾルバは名前を引けない。
- NDK は `~/Library/Android/sdk/ndk/28.2.13676358`。SDK は `~/Library/Android/sdk`。
- minSdk 26 / targetSdk 36 / ABI は `arm64-v8a` と `x86_64`。
- **`handoff.Owner` に新しい値を足さない。** `OwnerDesktop` を再利用する。
- **`HealthResponse` に `platform` や capability のフィールドを足さない。**
- **`cmd/sshc/wiring_linux.go` のビルドタグを触らない。** `!android` を足すと `GOOS=android go build ./...` が壊れる。
- **Go の error 文字列を Kotlin や UI へそのまま渡さない。** bootstrap fragment を含み得る。
- `selfupdate.Checker` は Android で nil。
- コメントは日本語。既存ファイルの文体（「なぜそうしないか」を書く）に合わせる。
- 各タスクの最後に `go test ./...` が通ること。

## ファイル構成

| ファイル | 責務 |
|---|---|
| `mobile/sshc.go` | 公開 API（`Start` / `Stop` / `Version`）と singleton |
| `mobile/dependencies.go` | `app.Dependencies` の組み立て。Android の前提はここに閉じる |
| `mobile/dependencies_test.go` | 組み立ての表明 |
| `mobile/sshc_test.go` | ライフサイクルの表明（ホスト上で実際に engine を起こす） |
| `internal/platform/shell.go` | `shellFallbacks` の引数化と android 分岐 |
| `android/app/src/main/java/.../MainActivity.kt` | WebView 1 枚と戻るキー |
| `android/app/src/main/java/.../EngineService.kt` | foreground service。engine の寿命を持つ |
| `android/app/src/main/AndroidManifest.xml` | 権限、service 宣言、network security config |
| `android/app/src/main/res/xml/network_security_config.xml` | 127.0.0.1 にだけ cleartext |

---

### Task 1: gomobile が Go 1.26.6 で bind できることを確かめる

**未知数 1 のゲート。** 倒れたら spec の「退けた案」の c-shared + 手書き JNI へ戻る。ここで先へ進んではいけない。

**Files:**
- Create: `mobile/sshc.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Consumes: なし
- Produces: `func Version() string`（Task 3 で `Start` / `Stop` が同じファイルに加わる）

- [ ] **Step 1: gomobile を入れ、モジュールに bind を足す**

```sh
cd .worktrees/android-engine
go install golang.org/x/mobile/cmd/gomobile@latest
go get golang.org/x/mobile/bind
go mod tidy
```

`gomobile bind` は対象モジュールが `golang.org/x/mobile` を持っていることを要求する。`go.mod` に 1 行増えるのは想定どおり。

- [ ] **Step 2: 最小の `mobile/sshc.go` を書く**

```go
// Package mobile は、この engine を Android アプリの中から起こすための境界である。
//
// **ここに置くものは gomobile が bind できる形でなければならない。** 渡せるのは
// 文字列と error だけなので、構造体も interface も公開しない。それは制約では
// なく、この境界に必要なものがそれだけだという事実の反映である。
package mobile

// Version は、この engine のバージョンを返す。
//
// ビルド時に -ldflags で埋める cmd/sshc の version とは別に持つ。AAR は
// cmd/sshc とは別の成果物であり、同じ変数を共有すると、どちらのビルドが
// 値を入れたのか分からなくなる。
var version = "dev"

func Version() string { return version }
```

- [ ] **Step 3: bind して AAR が出ることを確かめる**

```sh
export ANDROID_HOME="$HOME/Library/Android/sdk"
export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/28.2.13676358"
gomobile init
gomobile bind -target=android/arm64 -androidapi 26 -o /tmp/sshc-probe.aar ./mobile
ls -la /tmp/sshc-probe.aar
```

Expected: AAR が生成される。

**倒れた場合:** `gomobile` が Go 1.26.6 を拒否する、または bind が内部エラーで落ちる。そのときは**ここで止まり、設計へ戻る。** `-buildmode=c-shared` + 手書き JNI に切り替える判断は、計画ではなく設計の判断である。

- [ ] **Step 4: 既存のテストが通ることを確かめる**

```sh
go build ./... && go vet ./... && go test ./...
```

Expected: PASS。`mobile` に副作用は無いので、既存の何も動かない。

- [ ] **Step 5: Commit**

```sh
git add mobile/sshc.go go.mod go.sum
git commit -m "feat: open a bind boundary for the Android shell"
```

---

### Task 2: `app.Dependencies` の Android 版を組み立てる

Android の前提——ホームは注入される、ツールチェーンは無い、エージェントも無い、環境は継がない——を 1 つの関数に閉じ込め、ホスト上で表明する。**Android 実機は要らない。**

**Files:**
- Create: `mobile/dependencies.go`
- Create: `mobile/dependencies_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `func newDependencies(home, cache string, logger *slog.Logger, announce func(app.Readiness) error) (app.Dependencies, error)`

- [ ] **Step 1: 失敗するテストを書く**

`mobile/dependencies_test.go`:

```go
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
```

- [ ] **Step 2: テストが失敗することを確かめる**

```sh
go test ./mobile/
```

Expected: FAIL — `undefined: newDependencies`

- [ ] **Step 3: `mobile/dependencies.go` を書く**

```go
package mobile

import (
	"crypto/rand"
	"log/slog"
	"net"
	"os"
	"path/filepath"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/ui"
)

// newDependencies は、Android で成立する形の依存一式を組み立てる。
//
// cmd/sshc/engine.go の runEngineApp と同じ役目だが、落としているものが 4 つ
// ある: 所有権の監視（親が死ねば道連れなので監視する対象が無い）、シグナル
// （Android はプロセスにシグナルを送って落とさない）、終了コードへの写像
// （返すのは error である）、そして自己更新（バイナリを置き換える経路が無い）。
func newDependencies(
	home, cache string,
	logger *slog.Logger,
	announce func(app.Readiness) error,
) (app.Dependencies, error) {
	assets, err := ui.FS()
	if err != nil {
		return app.Dependencies{}, err
	}
	return app.Dependencies{
		Random:   rand.Reader,
		Announce: announce,
		Listen:   net.Listen,
		UI:       assets,
		Logger:   logger,
		Home:     home,
		Owner:    handoff.OwnerDesktop,
		PID:      os.Getpid(),
		// **どちらも nil が答えである。** ssh-keygen も ssh-agent も Android に
		// 居ないので、それを探す道具を持つこと自体が嘘になる。受け側は既に
		// nil を機能の不在として扱う。
		Toolchain: nil,
		KeyAgent:  nil,
		// SHELL を読まない。Android でそれを設定した人は居ないので、
		// 読めば偶然の値が権威になる。
		Lookup:  func(string) (string, bool) { return "", false },
		Environ: androidEnvironment(home, cache),
		// **置き換えられない更新を提示しない。**
		Updates: nil,
	}, nil
}

// androidEnvironment は、埋め込みターミナルが継ぐ環境を固定で組み立てる。
//
// os.Environ を渡さないのは、Android アプリの環境に有用な PATH が無いためで
// ある。そのまま渡せば、/system/bin すら見えないシェルが開く。
func androidEnvironment(home, cache string) func() []string {
	environ := []string{
		"HOME=" + home,
		"PATH=/system/bin:/system/xbin",
		"TERM=xterm-256color",
		"TMPDIR=" + filepath.Clean(cache),
	}
	return func() []string { return append([]string(nil), environ...) }
}
```

- [ ] **Step 4: テストが通ることを確かめる**

```sh
go test ./mobile/ -v
```

Expected: 5 件すべて PASS。

- [ ] **Step 5: Commit**

```sh
git add mobile/dependencies.go mobile/dependencies_test.go
git commit -m "feat: state what the engine can reach on Android"
```

---

### Task 3: `Start` と `Stop`

engine の寿命を 2 つの関数に畳む。**ホスト上で実際に engine を起こしてテストする。** Android 実機は要らない — `app.Run` は darwin でも同じように動く。

**Files:**
- Modify: `mobile/sshc.go`
- Create: `mobile/sshc_test.go`

**Interfaces:**
- Consumes: `newDependencies`（Task 2）
- Produces: `func Start(home, cache string) (string, error)`、`func Stop() error`、`var ErrAlreadyStarted error`、`var ErrNotStarted error`

- [ ] **Step 1: 失敗するテストを書く**

`mobile/sshc_test.go`:

```go
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
```

- [ ] **Step 2: テストが失敗することを確かめる**

```sh
go test ./mobile/
```

Expected: FAIL — `undefined: Start`, `undefined: Stop`, `undefined: ErrAlreadyStarted`, `undefined: ErrNotStarted`

- [ ] **Step 3: `mobile/sshc.go` に実装を足す**

```go
package mobile

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"path/filepath"
	"sync"

	"sshc/internal/app"
	"sshc/internal/enginelock"
)

var version = "dev"

func Version() string { return version }

var (
	// ErrAlreadyStarted は、この プロセスの engine が既に受け付けていることを言う。
	ErrAlreadyStarted = errors.New("an engine is already running in this process")
	ErrNotStarted     = errors.New("no engine is running in this process")
)

// running は、このプロセスの唯一の engine である。
//
// **構造体を bind して Kotlin にインスタンスを持たせない。** 1 プロセスに
// engine は 1 台という制約は Android では設計判断ではなく事実であり、複数
// 持てる形を見せれば、持てないものを持てるように見せることになる。
var running struct {
	sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	release func() error
}

// Start は engine を起こし、入口の URL を返す。
//
// 返るのは listener が bind され Announce が呼ばれた後である。呼び出し側は
// この URL を即座に WebView へ渡すので、早く返せば空のページが出る。
func Start(home, cache string) (string, error) {
	running.Lock()
	defer running.Unlock()
	if running.cancel != nil {
		return "", ErrAlreadyStarted
	}

	// **ロックは engine より先である。** 2 台目が app.Run へ入ってから
	// 落ちると、その一瞬だけ同じ状態ディレクトリを 2 つが握る。
	release, err := enginelock.Acquire(filepath.Join(app.HandoffDir(home), "engine.lock"))
	if err != nil {
		return "", err
	}

	// gomobile bind は標準 log の出力先を logcat へ差し替える。slog をそこへ
	// 流し込めば、cgo を 1 行も書かずに logcat へ出る。
	logger := slog.New(slog.NewTextHandler(log.Writer(), &slog.HandlerOptions{Level: slog.LevelInfo}))

	entrance := make(chan string, 1)
	dependencies, err := newDependencies(home, cache, logger, func(readiness app.Readiness) error {
		entrance <- readiness.DesktopURL
		return nil
	})
	if err != nil {
		return "", errors.Join(err, release())
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	failed := make(chan error, 1)
	go func() {
		defer close(done)
		if runErr := app.Run(ctx, dependencies, version); runErr != nil && !errors.Is(runErr, context.Canceled) {
			failed <- runErr
		}
	}()

	select {
	case url := <-entrance:
		running.cancel, running.done, running.release = cancel, done, release
		return url, nil
	case err := <-failed:
		cancel()
		<-done
		return "", errors.Join(err, release())
	case <-done:
		cancel()
		return "", errors.Join(errors.New("the engine stopped before it announced an entrance"), release())
	}
}

// Stop は engine を止め、ロックを手放す。
//
// **app.Run が戻るまで待つ。** 待たずにロックを手放すと、次の Start が
// まだ握られているポートに bind しに行く。
func Stop() error {
	running.Lock()
	defer running.Unlock()
	if running.cancel == nil {
		return ErrNotStarted
	}
	running.cancel()
	<-running.done
	err := running.release()
	running.cancel, running.done, running.release = nil, nil, nil
	return err
}
```

- [ ] **Step 4: テストが通ることを確かめる**

```sh
go test ./mobile/ -v
go test -race ./mobile/
```

Expected: すべて PASS。race detector も静か。

- [ ] **Step 5: `mobile` にテスト専用パッケージが混ざらないことを表明する**

`internal/acceptance/binary_test.go` の `TestNoTestOnlyPackageReachesTheShippedBinary` と同じことを AAR についても言う。同ファイルへ追記:

```go
// TestNoTestOnlyPackageReachesTheAndroidLibrary は、AAR についても同じことを言う。
// 出荷物が 2 つになったので、規則も 2 つに対して立てる。
func TestNoTestOnlyPackageReachesTheAndroidLibrary(t *testing.T) {
	list := exec.Command("go", "list", "-deps", "./mobile")
	list.Dir = filepath.Join("..", "..")
	output, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("go list = %v\n%s", err, output)
	}
	seen := 0
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			seen++
		}
		switch trimmed {
		case "sshc/internal/acceptance":
			t.Error("the hardening suite is linked into the Android library")
		case "testing", "net/http/httptest":
			t.Errorf("%s is linked into the Android library", trimmed)
		}
	}
	if seen == 0 {
		t.Fatal("go list reported no dependency at all; this check is not looking at the library")
	}
}
```

```sh
go test ./internal/acceptance/ -run TestNoTestOnlyPackageReachesTheAndroidLibrary -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```sh
git add mobile/sshc.go mobile/sshc_test.go internal/acceptance/binary_test.go
git commit -m "feat: hold the engine's life in two functions"
```

---

### Task 4: エミュレータで動かし、残る未知数を測る

**未知数 2 と 3 のゲート。** ここまでは Android を 1 度も動かしていない。この
タスクの成果物は、動く最小の APK と、**測った結果**である。

未知数 4（WebView から `ws://127.0.0.1:PORT` が CSP `connect-src 'self'` の下で
通るか）は、spec を書いた後に根拠が見つかった。Electron は Chromium であり、
埋め込みターミナルはそこで既に動いている。Android WebView も Chromium なので、
新しい未知数ではなくスモークで確認する項目である。

**Files:**
- Create: `android/settings.gradle.kts`, `android/build.gradle.kts`, `android/app/build.gradle.kts`, `android/gradle.properties`
- Create: `android/app/src/main/AndroidManifest.xml`
- Create: `android/app/src/main/java/com/github/aida0710/sshc/MainActivity.kt`
- Create: `mobile/probe_spike.go` — **使い捨て。Step 7 で消す**

**Interfaces:**
- Consumes: `Start` / `Stop`（Task 3）
- Produces: なし（この後のタスクは Kotlin 側を置き換えていく）

- [ ] **Step 1: 使い捨ての probe を書く**

`mobile/probe_spike.go`:

```go
package mobile

// このファイルは使い捨てである。Android で 2 つのことが成立するかを測るためだけに
// 存在し、答えが出たら消す。**リポジトリに残してはならない。**

import (
	"context"
	"net"
	"strings"
	"time"

	"sshc/internal/platform"
)

// ProbeDNS は、この端末で名前が引けるかを答える。
//
// Android には /etc/resolv.conf が無い。cgo リゾルバが netd へ届いていなければ、
// このアプリケーションは Android でどのホストへも繋げない。
func ProbeDNS(host string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return "FAIL: " + err.Error()
	}
	return "OK: " + strings.Join(addresses, ",")
}

// ProbeShell は、開けるログインシェルがこの端末にあるかを答える。
func ProbeShell() string {
	shell, err := platform.LoginShell(func(string) (string, bool) { return "", false })
	if err != nil {
		return "FAIL: " + err.Error()
	}
	return "OK: " + shell
}
```

- [ ] **Step 2: Gradle プロジェクトを作る**

`android/settings.gradle.kts`:

```kotlin
pluginManagement {
    repositories { google(); mavenCentral(); gradlePluginPortal() }
}
dependencyResolutionManagement {
    repositories { google(); mavenCentral() }
}
rootProject.name = "sshc"
include(":app")
```

`android/build.gradle.kts`:

```kotlin
plugins {
    id("com.android.application") version "8.14.0" apply false
    id("org.jetbrains.kotlin.android") version "2.1.20" apply false
}
```

`android/gradle.properties`:

```
android.useAndroidX=true
org.gradle.jvmargs=-Xmx2048m
```

`android/app/build.gradle.kts`:

```kotlin
plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.github.aida0710.sshc"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.github.aida0710.sshc"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "0.1.0"
        ndk { abiFilters += listOf("arm64-v8a", "x86_64") }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
}

dependencies {
    // sshc.aar は `make android-bind` が libs/ へ置く。Go の成果物なので追跡しない。
    implementation(files("libs/sshc.aar"))
}
```

- [ ] **Step 3: AAR を焼いて配置する**

```sh
export ANDROID_HOME="$HOME/Library/Android/sdk"
export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/28.2.13676358"
mkdir -p android/app/libs
gomobile bind -target=android/arm64,android/amd64 -androidapi 26 \
  -o android/app/libs/sshc.aar ./mobile
```

`android/app/libs/` を `.gitignore` へ足す（`/android/app/libs/`）。**AAR は
ビルド成果物であり、追跡しない。**

- [ ] **Step 4: 最小の `MainActivity` を書く**

`android/app/src/main/java/com/github/aida0710/sshc/MainActivity.kt`:

```kotlin
package com.github.aida0710.sshc

import android.annotation.SuppressLint
import android.os.Bundle
import android.util.Log
import android.webkit.WebView
import app.Activity as _unused // 使わない。import 整理で消すこと
import mobile.Mobile

class MainActivity : android.app.Activity() {
    private lateinit var webView: WebView

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // このタスクの成果物は測定値である。probe の答えを logcat へ出す。
        Log.i("sshc-probe", "dns: " + Mobile.probeDNS("github.com"))
        Log.i("sshc-probe", "shell: " + Mobile.probeShell())

        val url = Mobile.start(filesDir.absolutePath, cacheDir.absolutePath)

        webView = WebView(this)
        webView.settings.javaScriptEnabled = true
        webView.settings.domStorageEnabled = true
        webView.loadUrl(url)
        setContentView(webView)
    }

    override fun onDestroy() {
        Mobile.stop()
        super.onDestroy()
    }
}
```

**注意:** `import app.Activity as _unused` の行は書かない（説明のための誤りを避けるための注記であり、`android.app.Activity` を直接継承する）。gomobile が生成する Java のクラス名は `mobile.Mobile`、メソッドは先頭小文字（`start` / `stop` / `probeDNS` / `probeShell`）になる。生成された `libs/sshc.aar` の中の実際の名前を確認してから書くこと。

`android/app/src/main/AndroidManifest.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <uses-permission android:name="android.permission.INTERNET" />
    <application
        android:label="sshc"
        android:usesCleartextTraffic="false"
        android:networkSecurityConfig="@xml/network_security_config">
        <activity android:name=".MainActivity" android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
```

`android/app/src/main/res/xml/network_security_config.xml`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<!--
  127.0.0.1 にだけ cleartext を許す。usesCleartextTraffic="true" は使わない
  ——全ホストが開く。engine が居るのは loopback だけである。
-->
<network-security-config>
    <domain-config cleartextTrafficPermitted="true">
        <domain includeSubdomains="false">127.0.0.1</domain>
    </domain-config>
</network-security-config>
```

- [ ] **Step 5: エミュレータで動かす**

```sh
export ANDROID_HOME="$HOME/Library/Android/sdk"
PATH="$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$ANDROID_HOME/cmdline-tools/latest/bin:$PATH"

avdmanager list avd                       # 無ければ次の行で作る
avdmanager create avd -n sshc -k "system-images;android-36.1;google_apis_playstore;arm64-v8a"
emulator -avd sshc -no-snapshot &
adb wait-for-device

cd android && ./gradlew installDebug && cd ..
adb shell am start -n com.github.aida0710.sshc/.MainActivity
adb logcat -d -s sshc-probe GoLog
```

- [ ] **Step 6: 測った結果を判定する**

| 出力 | 意味 | 判定 |
|---|---|---|
| `dns: OK: …` | cgo リゾルバが netd へ届いている | 未知数 2 解決 |
| `dns: FAIL: …` | **設計へ戻る。** Android で SSH できない | 停止 |
| `shell: OK: /system/bin/sh` | ログインシェルが見つかる（Task 5 の後） | 未知数 3 の前半 |
| WebView に UI が出る | 埋め込み UI と CSP が成立 | 未知数 4 解決 |
| ターミナル画面が繋がる | WebSocket が CSP を通り、PTY が開く | 未知数 3 解決 |

**`dns: FAIL` の場合はここで止まる。** 名前が引けない SSH クライアントには意味が
無く、それは計画ではなく設計の問題である。

- [ ] **Step 7: probe を消し、結果を書き残す**

```sh
git rm mobile/probe_spike.go
```

`MainActivity.kt` から probe の 2 行も消す。測った結果を
`docs/superpowers/specs/2026-08-16-android-engine-design.md` の「計画の最初に潰す
4 つの未知数」の節へ、日付と結果として追記する。

- [ ] **Step 8: Commit**

```sh
git add android .gitignore docs/superpowers/specs/2026-08-16-android-engine-design.md
git rm --cached mobile/probe_spike.go 2>/dev/null || true
git commit -m "feat: put the engine behind a WebView on Android"
```

---

### Task 5: `/system/bin/sh` をログインシェルの候補に加える

`shellFallbacks` は `runtime.GOOS` を関数の中で直接読んでいるのでテストが書けない。
**引数化してから分岐を足す。**

**Files:**
- Modify: `internal/platform/shell.go:16-27`
- Modify: `internal/platform/shell_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `func shellFallbacks(goos string) []string`（パッケージ内部）

- [ ] **Step 1: 失敗するテストを書く**

`internal/platform/shell_test.go` へ追記:

```go
// Android には /bin/bash も /bin/zsh も居ない。**/bin/sh すら居ない** ——
// Android の sh は /system/bin/sh (mksh) である。ここを間違えると、
// ターミナルは「開けるシェルが無い」としか言えなくなる。
func TestShellFallbacksOnAndroidNameTheOnlyShellThatExists(t *testing.T) {
	want := []string{"/system/bin/sh"}
	if got := shellFallbacks("android"); !slices.Equal(got, want) {
		t.Errorf("shellFallbacks(android) = %q, want %q", got, want)
	}
}

// macOS の既定は zsh、それ以外の unix は bash である。**android を足したことで
// この 2 つが変わっていないこと**を、同じ場所で言う。
func TestShellFallbacksKeepTheirExistingOrder(t *testing.T) {
	if got, want := shellFallbacks("darwin"), []string{"/bin/zsh", "/bin/bash", "/bin/sh"}; !slices.Equal(got, want) {
		t.Errorf("shellFallbacks(darwin) = %q, want %q", got, want)
	}
	if got, want := shellFallbacks("linux"), []string{"/bin/bash", "/bin/zsh", "/bin/sh"}; !slices.Equal(got, want) {
		t.Errorf("shellFallbacks(linux) = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: テストが失敗することを確かめる**

```sh
go test ./internal/platform/ -run TestShellFallbacks -v
```

Expected: FAIL — `too many arguments in call to shellFallbacks`

- [ ] **Step 3: `shellFallbacks` を引数化し、android を足す**

`internal/platform/shell.go` の当該関数を差し替える:

```go
// shellFallbacks は、SHELL が無いときに順に確かめる場所である。
//
// /etc/passwd は読まない。ログイン項目として起動した常駐プロセスの中では
// SHELL が設定されていないことがあり、そこが唯一の権威になってしまうが、
// コンテナや合成 passwd の中でその一行は当てにならない。ここに並んでいるのは
// 実際に存在を確かめる絶対パスだけである。
//
// **Android には /bin が無い。** sh は /system/bin/sh (mksh) にあり、
// bash も zsh も居ない。ここに /bin/sh を残しても永久に見つからない。
//
// goos を引数で受けるのは、この一覧がテストできるようにするためである。
// runtime.GOOS をここで読むと、走っているマシンでしか通らない表明になる。
func shellFallbacks(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"/bin/zsh", "/bin/bash", "/bin/sh"}
	case "android":
		return []string{"/system/bin/sh"}
	default:
		return []string{"/bin/bash", "/bin/zsh", "/bin/sh"}
	}
}
```

`LoginShell` の呼び出し側を `for _, candidate := range shellFallbacks(runtime.GOOS) {` に直す。

- [ ] **Step 4: テストが通ることを確かめる**

```sh
go test ./internal/platform/ -v
go test ./...
```

Expected: すべて PASS

- [ ] **Step 5: Commit**

```sh
git add internal/platform/shell.go internal/platform/shell_test.go
git commit -m "fix: name the only shell Android actually has"
```

---

### Task 6: 道具が無いとき、その機能が本当に閉じることを表明する

spec と `app.Dependencies` のコメントは「`Toolchain` が nil ならハードウェア鍵は
出ない」「`KeyAgent` が nil ならエージェント登録は不可と報告する」と約束している。
**約束されているだけで、片方しか表明されていない。** Android は、その約束が初めて
本番で使われる場所である。

これが「Android で CLI 機能を削除する」の実体でもある。削除するコードは無く、
既にある機構が閉じることを固定するだけである。`internal/httpserver` ではなく
`internal/keys` に書く——`httpserver` の stub は `AgentIdentities` を固定値で
返すので、そこでは道具の不在を通せない。

**Files:**
- Modify: `internal/keys/service_test.go`
- Modify: `internal/keys/catalogue_test.go`

**Interfaces:**
- Consumes: なし
- Produces: なし

- [ ] **Step 1: 失敗するテストを書く**

`internal/keys/service_test.go` へ追記:

```go
// **道具が無いことは、機能が無いことである。** Android には ssh-agent が
// 居ないので agent は nil になる。そのとき一覧が「エージェントは居る」と
// 言えば、画面は押しても何も起きないボタンを出す。
//
// newServiceWithAgent に nil を渡すのは、fake を渡さないことに意味があるから
// である——available を false にした fake は「居るが応答しない」であって、
// 「そもそも居ない」ではない。Android は後者である。
func TestAgentIdentitiesSayNoAgentWhenNoneIsWired(t *testing.T) {
	service, _ := newServiceWithAgent(t, newQueryRunner(), nil)
	identities, available := service.AgentIdentities(context.Background())
	if available {
		t.Error("a service with no agent claims one is reachable")
	}
	if identities != nil {
		t.Errorf("identities = %v, want none", identities)
	}
}
```

`internal/keys/catalogue_test.go` へ追記:

```go
// ハードウェア鍵は ssh-keygen を利用者自身が走らせるものである。Android には
// ssh-keygen が居ない。**打てないコマンドを一覧に出さない。**
func TestCatalogueOffersNoHardwareKeyWithoutAToolchain(t *testing.T) {
	catalogue := CatalogueReader{Toolchain: nil}.Read(context.Background())
	for _, variant := range catalogue.Variants {
		if !variant.InProcess {
			t.Errorf("variant %q needs a toolchain this machine does not have", variant.Label)
		}
	}
}
```

- [ ] **Step 2: テストが失敗することを確かめる**

```sh
go test ./internal/keys/ -run "NoAgentWhenNoneIsWired|WithoutAToolchain" -v
```

Expected: 両方 PASS してよい。**通ってしまうのが期待どおりである** ——
`service.go:776` の `service.agent == nil` と `catalogue.go:74` の
`reader.Toolchain == nil` が既に閉じているからだ。このタスクは新しい振る舞いを
足すのではなく、**Android がそこに寄りかかる前に、その振る舞いを固定する。**

どちらかが FAIL した場合は、その分岐が実は閉じていない。Step 3 へ進む。

- [ ] **Step 3: 閉じていなかった分だけ実装する**

FAIL したものがあれば、`internal/keys` の該当箇所に道具の不在を機能の不在として
扱う分岐を足す。**`runtime.GOOS` で分岐しない。** 見るのは依存が居るかどうかだけ
である。両方 PASS したなら、このステップは何もしない。

- [ ] **Step 4: 全体が通ることを確かめる**

```sh
go test ./internal/keys/ -v
go test ./...
```

Expected: PASS

- [ ] **Step 5: Commit**

```sh
git add internal/keys/
git commit -m "test: assert that missing tools mean missing features"
```

---

### Task 7: foreground service とエラーの畳み込み

Task 4 の `MainActivity` は engine を直接持っている。**別アプリへ切り替えた瞬間に
セッションが切れる。** SSH クライアントでそれは、起きるかもしれない話ではなく
最初に起きる話である。engine の寿命を foreground service へ移す。

同時に、`Start` の error を Kotlin へ渡す形を固定する。**Go の error 文字列を
そのまま画面に出さない** — bootstrap fragment を含み得る。

**Files:**
- Create: `android/app/src/main/java/com/github/aida0710/sshc/EngineService.kt`
- Modify: `android/app/src/main/java/com/github/aida0710/sshc/MainActivity.kt`
- Modify: `android/app/src/main/AndroidManifest.xml`
- Modify: `mobile/sshc.go`
- Modify: `mobile/sshc_test.go`

**Interfaces:**
- Consumes: `Start` / `Stop`（Task 3）
- Produces: `func StartFailureKind(err error) int`（`mobile` から公開。1=起動済み, 2=listen 不可, 3=即死, 0=それ以外）

- [ ] **Step 1: 畳み込みの失敗するテストを書く**

`mobile/sshc_test.go` へ追記:

```go
// **Go の error 文字列を Kotlin へ渡さない。** Start が失敗する理由は 3 つに
// 畳める。理由の区別だけを渡し、文面は Android 側が持つ——engine の error は
// 入口の URL を含み得るので、そのまま出せば logcat と画面に fragment が残る。
func TestStartFailureKindCarriesTheReasonAndNotTheText(t *testing.T) {
	if got := StartFailureKind(ErrAlreadyStarted); got != 1 {
		t.Errorf("StartFailureKind(ErrAlreadyStarted) = %d, want 1", got)
	}
	if got := StartFailureKind(errListenFailed); got != 2 {
		t.Errorf("StartFailureKind(errListenFailed) = %d, want 2", got)
	}
	if got := StartFailureKind(errEngineStoppedEarly); got != 3 {
		t.Errorf("StartFailureKind(errEngineStoppedEarly) = %d, want 3", got)
	}
	if got := StartFailureKind(errors.New("http://127.0.0.1:9/#bootstrap=secret")); got != 0 {
		t.Errorf("StartFailureKind(unknown) = %d, want 0", got)
	}
}
```

- [ ] **Step 2: テストが失敗することを確かめる**

```sh
go test ./mobile/ -run StartFailureKind
```

Expected: FAIL — `undefined: StartFailureKind`

- [ ] **Step 3: `mobile/sshc.go` に畳み込みを足す**

Task 3 の `Start` の中で使っている無名の error を、名前のある値へ差し替える。

```go
var (
	errListenFailed       = errors.New("the engine could not take a loopback port")
	errEngineStoppedEarly = errors.New("the engine stopped before it announced an entrance")
)

// StartFailureKind は、Start が失敗した理由を番号ひとつに畳む。
//
// **文字列を渡さない。** engine の error は入口の URL を含み得るので、
// そのまま Kotlin へ渡せば logcat とエラー画面に bootstrap fragment が残る。
// 文面は Android 側が持ち、こちらは区別だけを渡す。
func StartFailureKind(err error) int {
	switch {
	case errors.Is(err, ErrAlreadyStarted):
		return 1
	case errors.Is(err, errListenFailed):
		return 2
	case errors.Is(err, errEngineStoppedEarly):
		return 3
	default:
		return 0
	}
}
```

`Start` の `case <-done:` の枝を `errEngineStoppedEarly` に、`enginelock.Acquire`
の後に続く listen 由来の失敗を `errors.Join(errListenFailed, err)` に直す。

- [ ] **Step 4: テストが通ることを確かめる**

```sh
go test ./mobile/ -v && go test -race ./mobile/
```

Expected: PASS

- [ ] **Step 5: `EngineService` を書く**

`android/app/src/main/java/com/github/aida0710/sshc/EngineService.kt`:

```kotlin
package com.github.aida0710.sshc

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.os.Binder
import android.os.IBinder
import mobile.Mobile

// engine の寿命はここが持つ。
//
// **Activity に持たせない。** 別アプリへ切り替えた瞬間に Activity は止められる
// ので、そこに engine を置けば SSH セッションが切れる。パスワードをコピーしに
// 行って戻ったら落ちている、というのが最初に起きる。
class EngineService : Service() {
    private val binder = LocalBinder()
    var entrance: String? = null
        private set
    var failure: Int = 0
        private set

    inner class LocalBinder : Binder() {
        fun service(): EngineService = this@EngineService
    }

    override fun onCreate() {
        super.onCreate()
        startForeground(NOTIFICATION_ID, notification())
        try {
            entrance = Mobile.start(filesDir.absolutePath, cacheDir.absolutePath)
        } catch (error: Exception) {
            // **error のメッセージを保持しない。** 入口の URL を含み得る。
            failure = Mobile.startFailureKind(error)
        }
    }

    override fun onDestroy() {
        if (entrance != null) Mobile.stop()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder = binder

    private fun notification(): Notification {
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL, "sshc", NotificationManager.IMPORTANCE_LOW)
        )
        return Notification.Builder(this, CHANNEL)
            .setContentTitle(getString(R.string.engine_running))
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .build()
    }

    private companion object {
        const val CHANNEL = "engine"
        const val NOTIFICATION_ID = 1
    }
}
```

**注意:** `Mobile.startFailureKind` は gomobile が生成する名前。実際の生成物で
確認すること。`R.string.engine_running` は
`android/app/src/main/res/values/strings.xml` に足す。

- [ ] **Step 6: `MainActivity` を service に繋ぎ替える**

`MainActivity` から `Mobile.start` / `Mobile.stop` の直接呼び出しを消し、
`bindService` で `EngineService` に繋いで `entrance` を受け取る形にする。
`failure` が 0 でなければ WebView を出さず、番号に対応する文字列リソースを
出す。

`AndroidManifest.xml` へ追記:

```xml
<uses-permission android:name="android.permission.FOREGROUND_SERVICE" />
<uses-permission android:name="android.permission.FOREGROUND_SERVICE_DATA_SYNC" />
<uses-permission android:name="android.permission.POST_NOTIFICATIONS" />
```

`<application>` の中へ:

```xml
<service
    android:name=".EngineService"
    android:exported="false"
    android:foregroundServiceType="dataSync" />
```

- [ ] **Step 7: エミュレータで、切り替えてもセッションが残ることを確かめる**

```sh
cd android && ./gradlew installDebug && cd ..
adb shell am start -n com.github.aida0710.sshc/.MainActivity
# ターミナルを開いて接続したのち、ホームへ抜けて 30 秒待ち、戻る
adb shell input keyevent KEYCODE_HOME
adb shell am start -n com.github.aida0710.sshc/.MainActivity
```

Expected: セッションが生きたまま戻る。

- [ ] **Step 8: Commit**

```sh
git add android mobile/sshc.go mobile/sshc_test.go
git commit -m "fix: let the engine outlive the screen"
```

---

### Task 8: CI ゲートと手順書

Android 向けにコンパイルが通ることを、人の記憶ではなく CI が言う形にする。

**Files:**
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Modify: `README.md`
- Modify: `docs/manual-acceptance.md`

**Interfaces:**
- Consumes: なし
- Produces: `make android-bind`

- [ ] **Step 1: Makefile に target を足す**

```makefile
# Android の AAR。**CGO_ENABLED=1 でなければならない** ——Android には
# /etc/resolv.conf が無く、pure-Go リゾルバは名前を引けない。gomobile が
# NDK を通じて cgo を有効にするので、ここでは NDK の場所だけを要求する。
ANDROID_NDK_HOME ?= $(HOME)/Library/Android/sdk/ndk/28.2.13676358

android-bind:
	ANDROID_NDK_HOME="$(ANDROID_NDK_HOME)" gomobile bind \
		-target=android/arm64,android/amd64 -androidapi 26 \
		-o android/app/libs/sshc.aar ./mobile
```

`.PHONY` の行に `android-bind` を足す。

- [ ] **Step 2: `make test` に Android のビルドを足す**

```makefile
test:
	go test ./...
	go test -race ./...
	@# Android 向けにコンパイルが通ることは、gomobile を持たない環境でも言える。
	@# **CGO_ENABLED=0 で通ることは何も意味しない**が、ここで見ているのは
	@# ビルドタグの食い違いだけなので、これで足りる。
	GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...
	npm test --prefix web
	npm run typecheck --prefix web
	npm test --prefix desktop
```

- [ ] **Step 3: CI のジョブに足す**

`.github/workflows/ci.yml` の Go ジョブへ 1 ステップ追加:

```yaml
      - name: Android build
        run: GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build ./...
```

**gomobile bind と APK ビルドは足さない。** NDK と Gradle を CI に載せるのは
サブプロジェクト 3 の仕事であり、ここで混ぜると、この計画が終わっても CI が
赤いままになる。

- [ ] **Step 4: README を直す**

「Windows と Android は対応対象外です」の行から Android を外し、Android の欄を
足す。書くのは 3 つ: APK が engine を同一プロセスで抱えること、CLI は APK に
入らないこと、ローカルシェルは `/system/bin/sh`（mksh + toybox の箱庭）である
こと。

- [ ] **Step 5: 手順書を書く**

`docs/manual-acceptance.md` に Android の節を足す。SDK と NDK の場所、
`make android-bind`、`./gradlew installDebug`、エミュレータの起こし方、そして
Task 4 の判定表（DNS / シェル / WebView / ターミナル）を受け入れ項目として写す。

- [ ] **Step 6: 全部が通ることを確かめる**

```sh
make test
```

Expected: すべて PASS

- [ ] **Step 7: Commit**

```sh
git add Makefile .github/workflows/ci.yml README.md docs/manual-acceptance.md
git commit -m "build: gate the Android target in CI"
```

---

## 完了後

`android-engine` を main へマージする。レスポンシブ化（`android-responsive-ui`）
とは触るファイルが重ならないので、どちらが先でもよい。

サブプロジェクト 3（署名、Play Store、CI での APK ビルド、artifact 名の検査）は
別の spec と計画になる。`scripts/verify-artifact-name.sh` は現在 goos/goarch しか
知らないので、`.apk` を足すのはそこの仕事である。
