package acceptance_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Android のネイティブ層と Web UI の間で共有する設定値を検証する。
//
// ネイティブ層は Java で、ページは CSS で、engine は Go である。gomobile は Go の定数を
// Java へ運んでくれるので、失敗理由の番号は Go に 1 つだけ在ればよくなった
// （mobile.KindListenFailed を Java がそのまま読む）。色にはその道が無い。
// WebView の中の CSS 変数を、外側の FrameLayout が読む手段は無い。
//
// だから色だけは 2 か所に在り、揃っていることを誰かが見なければならない。
// 揃っていないと、ページの上端に別の板が乗っているように見える。落ちないし、
// 例外も出ないので、見たユーザーが違和感を言うまで誰も気づかない。

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// cssToken は、宣言ブロックひとつの中からその変数の値を取る。
func cssToken(t *testing.T, css, selector, name string) string {
	t.Helper()
	start := strings.Index(css, selector+" {")
	if start < 0 {
		t.Fatalf("テーマの色のファイルに %q という選択子が無い", selector)
	}
	end := strings.Index(css[start:], "\n}")
	if end < 0 {
		t.Fatalf("%q の宣言ブロックが閉じていない", selector)
	}
	block := css[start : start+end]

	found := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `:\s*([^;]+);`).FindStringSubmatch(block)
	if found == nil {
		t.Fatalf("%q の中に %s が無い", selector, name)
	}
	return strings.TrimSpace(found[1])
}

// androidChromeColours は、chromeColour() が返す 2 つの色（CHROME_NIGHT・CHROME_DAY）を、
// 夜・昼の順で返す。
func androidChromeColours(t *testing.T, java string) (dark, light string) {
	t.Helper()
	if !regexp.MustCompile(`UI_MODE_NIGHT_YES\s*\?\s*CHROME_NIGHT\s*:\s*CHROME_DAY`).MatchString(java) {
		t.Fatal("MainActivity.chromeColour が、夜と昼の色（CHROME_NIGHT・CHROME_DAY）を返す形をしていない")
	}
	constant := func(name string) string {
		found := regexp.MustCompile(`static final int ` + name + `\s*=\s*0x([0-9A-Fa-f]{8});`).FindStringSubmatch(java)
		if found == nil {
			t.Fatalf("MainActivity に %s の ARGB が無い", name)
		}
		return found[1]
	}
	return constant("CHROME_NIGHT"), constant("CHROME_DAY")
}

// ページの帯と、その外側の余白は同じ色である。
//
// padding の外側に見えるのは FrameLayout の背景であり、WebView 自身の背景では
// ない。ページのツールバーと違う色を置くと、画面の上端に別の板が乗っているよう
// に見える。
//
// ネイティブの画面（起動の失敗など）では、MainActivity がシステムの明暗でライトか
// ダークの色を自分で塗る。MainActivity は Web UI の CSS 変数を直接参照できないため、
// 両方の値を検証する。Web を読み込んだあとは、Web が表示中のテーマの --ui-toolbar を
// 渡すので、ほかのテーマはここに要らない。Web を読み込む前の色は、下の
// TestTheAndroidLaunchIsTheDarkCanvas が確かめる。
func TestTheAndroidChromeMatchesThePageToolbar(t *testing.T) {
	t.Parallel()
	java := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")

	dark, light := androidChromeColours(t, java)

	for _, want := range []struct {
		theme    string
		selector string
		java     string
	}{
		{"light", ":where(:root),\n[data-theme=\"light\"]", light},
		{"dark", `[data-theme="dark"]`, dark},
	} {
		css := readRepoFile(t, "web", "src", "theme", "themes", want.theme+".css")
		token := cssToken(t, css, want.selector, "--ui-toolbar")
		// Java は ARGB、CSS は RGB。不透明であることも一緒に確かめる。
		if !strings.EqualFold(want.java[:2], "FF") {
			t.Errorf("%s: Java の色 0x%s が不透明ではない", want.selector, want.java)
		}
		if got := "#" + strings.ToLower(want.java[2:]); got != strings.ToLower(token) {
			t.Errorf("%s: %s.css は --ui-toolbar: %s、MainActivity は %s\n"+
				"  どちらかを変えたら、もう一方も変えること。", want.selector, want.theme, token, got)
		}
	}
}

// 起動の間は、ダークのページの地と同じ黒である。
//
// 起動画面（スプラッシュ）と、ページが描かれるまでの窓と WebView は、システムや
// アプリのテーマに関わらず launch_background で塗る。ライトの窓のまま起動すると、
// 暗いページの前に白い画面が一瞬光る。ダークのページへ切り替わる所で色が変わら
// ないよう、dark.css の --ui-canvas とそろえる。
func TestTheAndroidLaunchIsTheDarkCanvas(t *testing.T) {
	t.Parallel()
	res := func(parts ...string) string {
		return readRepoFile(t, append([]string{"android", "app", "src", "main", "res"}, parts...)...)
	}

	found := regexp.MustCompile(`<color name="launch_background">#([0-9A-Fa-f]{6})</color>`).FindStringSubmatch(res("values", "colors.xml"))
	if found == nil {
		t.Fatal("colors.xml に launch_background の RGB が無い")
	}
	canvas := cssToken(t, readRepoFile(t, "web", "src", "theme", "themes", "dark.css"), `[data-theme="dark"]`, "--ui-canvas")
	if got := "#" + strings.ToLower(found[1]); got != strings.ToLower(canvas) {
		t.Errorf("colors.xml の launch_background は %s、dark.css の --ui-canvas は %s\n"+
			"  どちらかを変えたら、もう一方も変えること。", got, canvas)
	}

	for _, want := range []struct {
		folder string
		item   string
	}{
		{"values", "android:windowBackground"},
		{"values-v31", "android:windowSplashScreenBackground"},
	} {
		item := `<item name="` + want.item + `">@color/launch_background</item>`
		if !strings.Contains(res(want.folder, "themes.xml"), item) {
			t.Errorf("%s/themes.xml の起動のテーマに %s が無い", want.folder, item)
		}
	}
	java := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")
	if !strings.Contains(java, "webView.setBackgroundColor(getColor(R.color.launch_background));") {
		t.Error("MainActivity が、ページが描かれる前の WebView を launch_background で塗っていない")
	}
}

// 番号は Java に書かれていない。
//
// gomobile は export された定数を Java 側にも生やすので、失敗理由は
// mobile/sshc.go の iota だけが持てばよい。以前ここは `case 2:` と直に書かれて
// おり、iota の途中に一つ挿すだけで Android が暗黙に別の文言を出す状態だった。
//
// Java 側に独自の失敗種別が再導入されないことを検証する。
func TestTheAndroidShellReadsTheFailureKindsFromGo(t *testing.T) {
	t.Parallel()
	java := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")

	failure := java[strings.Index(java, "private void showFailure("):]
	if end := strings.Index(failure, "\n    }"); end > 0 {
		failure = failure[:end]
	}
	if strings.Contains(failure, "private void showFailure(int ") {
		t.Error("showFailure が int を取っている。Mobile.lastStartFailureKind は long を返す")
	}
	if strings.Contains(java, "KindAlreadyStarted") {
		t.Error("Android単一process内の重複起動をfatal errorとして再導入している")
	}

	for _, kind := range []string{"KindListenFailed", "KindStoppedEarly", "KindStorageUnavailable", "KindEngineStartFailed"} {
		if !strings.Contains(failure, "Mobile."+kind) {
			t.Errorf("showFailure が Mobile.%s を読んでいない", kind)
		}
	}
	// 生の数と比べていたら、Go 側を動かしても気づけない。
	if regexp.MustCompile(`(?m)^\s*case\s+\d+\s*:`).MatchString(failure) {
		t.Error("showFailure が番号を直に書いている。Mobile の定数を読むこと")
	}
}

func TestTheAndroidFailureScreenUsesOnlySanitizedDiagnostics(t *testing.T) {
	t.Parallel()
	service := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "EngineService.java")
	activity := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")
	for _, required := range []string{
		"Mobile.lastStartFailureCode()",
		"Mobile.lastStartFailureDetail()",
		"FailureReport.render(",
		"ClipData.newPlainText",
	} {
		if !strings.Contains(service+activity, required) {
			t.Errorf("Androidの診断導線に %q が無い", required)
		}
	}
	if strings.Contains(service, "error.getMessage()") || strings.Contains(service, "Log.e(TAG, error") {
		t.Error("伏せ字化前のJava例外を画面またはlogcatへ出している")
	}
}

// Android 13以降はKEYCODE_BACKでは戻るジェスチャーを受け取れない。AndroidXの
// OnBackPressedDispatcherへ一本化し、WebViewの履歴をroute履歴として使い、
// ホームでだけtaskを背面へ送る形を固定する。
func TestTheAndroidShellRoutesModernBackNavigationThroughTheWebView(t *testing.T) {
	t.Parallel()
	manifest := readRepoFile(t, "android", "app", "src", "main", "AndroidManifest.xml")
	activity := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")

	for _, required := range []string{
		`android:enableOnBackInvokedCallback="true"`,
		"getOnBackPressedDispatcher().addCallback(",
		"new OnBackPressedCallback(true)",
		"webView.evaluateJavascript(",
		"sshc-android-back",
		"[role=dialog]",
		`location.pathname==='/'`,
		"history.back()",
		"webView.canGoBack()",
		"webView.goBack()",
		"moveTaskToBack(true)",
		// ページがHomeで離れると答えたら、WebViewの履歴（pushStateも積まれる）を戻さずに背面へ送る。
		`if ("false".equals(consumed)) {`,
	} {
		if !strings.Contains(manifest+activity, required) {
			t.Errorf("Androidの戻る導線に %q が無い", required)
		}
	}
	// 直接KEYCODE_BACKを見ると、predictive backを持つ端末で二重に反応する。
	for _, forbidden := range []string{"KeyEvent.KEYCODE_BACK", "onKeyDown("} {
		if strings.Contains(activity, forbidden) {
			t.Errorf("Androidの戻る導線が %q を直に扱っている", forbidden)
		}
	}
}

// WebViewの既定実装ではfile inputをキャンセルするため、Android shellが
// 選択・保存・renderer異常・通知permissionを明示的に仲介する形を固定する。
func TestTheAndroidShellBridgesDeviceSpecificWebFeatures(t *testing.T) {
	t.Parallel()
	manifest := readRepoFile(t, "android", "app", "src", "main", "AndroidManifest.xml")
	activity := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")
	bridge := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "NativeBridge.java")
	web := readRepoFile(t, "web", "src", "android", "native.ts")

	for _, required := range []string{
		"onShowFileChooser(",
		"getClipData()",
		"FileChooser.chosen(",
		"FileChooser.mimeTypes(",
		"Intent.ACTION_OPEN_DOCUMENT_TREE",
		"chosenFolder.open(",
		"setServiceWorkerClient(",
		"Intent.ACTION_CREATE_DOCUMENT",
		`addJavascriptInterface(nativeBridge, "sshcAndroid")`,
		"onRenderProcessGone(",
		"POST_NOTIFICATIONS",
		"requestPermissions(",
		"Intent.ACTION_VIEW",
		"engineOrigin.getPort() == target.getPort()",
		"setSystemBarsAppearance(",
		"saveWithAndroid(",
		"notifyAndroidTransfer(",
		"showAndroidNotification(",
		"terminalNotifications.show(",
		"sshc-android-notification-opened",
		"takeOpenedNotification(",
	} {
		if !strings.Contains(manifest+activity+bridge+web, required) {
			t.Errorf("Android固有機能のbridgeに %q が無い", required)
		}
	}
	if strings.Contains(activity, "Log.e(TAG, entrance") {
		t.Error("資格情報を含み得るentrance URLをlogcatへ出している")
	}
}

// ランチャーや通知から戻っただけでMainActivityを積み増したり、同じengineの
// WebViewを読み直したりしない。入力途中のフォームを通常のアプリ切り替えで失わないための境界。
func TestTheAndroidShellKeepsTheVisibleWebViewAcrossAppSwitches(t *testing.T) {
	t.Parallel()
	manifest := readRepoFile(t, "android", "app", "src", "main", "AndroidManifest.xml")
	activity := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")

	if !strings.Contains(manifest, `android:launchMode="singleTask"`) {
		t.Error("MainActivityがランチャー復帰で重複しないsingleTaskになっていない")
	}
	if !strings.Contains(activity, "Entrance.isAlreadyShowing(lastEntrance, entrance)") {
		t.Error("同じengine入口の再通知でWebViewを読み直す可能性がある")
	}
}
