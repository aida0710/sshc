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
		t.Fatalf("index.css に %q という選択子が無い", selector)
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

// androidChromeColours は、chromeColour() が返す 2 つの色を、夜・昼の順で返す。
func androidChromeColours(t *testing.T, java string) (dark, light string) {
	t.Helper()
	found := regexp.MustCompile(
		`UI_MODE_NIGHT_YES\s*\?\s*0x([0-9A-Fa-f]{8})\s*:\s*0x([0-9A-Fa-f]{8})`).FindStringSubmatch(java)
	if found == nil {
		t.Fatal("MainActivity.chromeColour が、夜と昼の 2 つの ARGB を返す形をしていない")
	}
	return found[1], found[2]
}

// ページの帯と、その外側の余白は同じ色である。
//
// padding の外側に見えるのは FrameLayout の背景であり、WebView 自身の背景では
// ない。ページのツールバーと違う色を置くと、画面の上端に別の板が乗っているよう
// に見える。
//
// MainActivity は Web UI の CSS 変数を直接参照できないため、両方の値を検証する。
func TestTheAndroidChromeMatchesThePageToolbar(t *testing.T) {
	css := readRepoFile(t, "web", "src", "index.css")
	java := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")

	dark, light := androidChromeColours(t, java)

	for _, want := range []struct {
		selector string
		java     string
	}{
		{":root", light},
		{`[data-theme="dark"]`, dark},
	} {
		token := cssToken(t, css, want.selector, "--ui-toolbar")
		// Java は ARGB、CSS は RGB。不透明であることも一緒に確かめる。
		if !strings.EqualFold(want.java[:2], "FF") {
			t.Errorf("%s: Java の色 0x%s が不透明ではない", want.selector, want.java)
		}
		if got := "#" + strings.ToLower(want.java[2:]); got != strings.ToLower(token) {
			t.Errorf("%s: index.css は --ui-toolbar: %s、MainActivity は %s\n"+
				"  どちらかを変えたら、もう一方も変えること。", want.selector, token, got)
		}
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
	service := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "EngineService.java")
	activity := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")
	for _, required := range []string{
		"Mobile.lastStartFailureCode()",
		"Mobile.lastStartFailureDetail()",
		"FailureReport.render(",
		"ClipData.newPlainText",
		"service.retry()",
	} {
		if !strings.Contains(service+activity, required) {
			t.Errorf("Androidの診断導線に %q が無い", required)
		}
	}
	if strings.Contains(service, "error.getMessage()") || strings.Contains(service, "Log.e(TAG, error") {
		t.Error("伏せ字化前のJava例外を画面またはlogcatへ出している")
	}
}

// Android 13以降はKEYCODE_BACKでは戻るジェスチャーを受け取れない。
// WebViewの履歴をroute履歴として使い、ホームでだけtaskを背面へ送る形を固定する。
func TestTheAndroidShellRoutesModernBackNavigationThroughTheWebView(t *testing.T) {
	manifest := readRepoFile(t, "android", "app", "src", "main", "AndroidManifest.xml")
	activity := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")

	for _, required := range []string{
		`android:enableOnBackInvokedCallback="true"`,
		"registerOnBackInvokedCallback(",
		"OnBackInvokedDispatcher.PRIORITY_DEFAULT",
		"webView.evaluateJavascript(",
		"sshc-android-back",
		"[role=dialog]",
		`location.pathname==='/'`,
		"history.back()",
		"webView.canGoBack()",
		"webView.goBack()",
		"moveTaskToBack(true)",
	} {
		if !strings.Contains(manifest+activity, required) {
			t.Errorf("Androidの戻る導線に %q が無い", required)
		}
	}
}

// WebViewの既定実装ではfile inputをキャンセルするため、Android shellが
// 選択・保存・renderer異常・通知permissionを明示的に仲介する形を固定する。
func TestTheAndroidShellBridgesDeviceSpecificWebFeatures(t *testing.T) {
	manifest := readRepoFile(t, "android", "app", "src", "main", "AndroidManifest.xml")
	activity := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "MainActivity.java")
	bridge := readRepoFile(t, "android", "app", "src", "main", "java",
		"com", "github", "aida0710", "sshc", "NativeBridge.java")
	web := readRepoFile(t, "web", "src", "android", "native.ts")

	for _, required := range []string{
		"onShowFileChooser(",
		"FileChooserParams.parseResult(",
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
	} {
		if !strings.Contains(manifest+activity+bridge+web, required) {
			t.Errorf("Android固有機能のbridgeに %q が無い", required)
		}
	}
	if strings.Contains(activity, "Log.e(TAG, entrance") {
		t.Error("資格情報を含み得るentrance URLをlogcatへ出している")
	}
}
