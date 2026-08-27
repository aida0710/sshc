package com.github.aida0710.sshc;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.ServiceConnection;
import android.content.pm.ApplicationInfo;
import android.content.res.Configuration;
import android.graphics.Insets;
import android.os.Build;
import android.os.Bundle;
import android.os.IBinder;
import android.util.Log;
import android.webkit.ConsoleMessage;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.view.KeyEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowInsets;
import android.widget.FrameLayout;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.TextView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.Toast;

import java.util.Arrays;

import mobile.Mobile;

/** EngineService から接続 URL を受け取り、WebView に表示する Activity。 */
public final class MainActivity extends Activity {
    private static final String TAG = "sshc";

    private WebView webView;
    private boolean bound;
    private EngineService service;

    private final ServiceConnection connection = new ServiceConnection() {
        @Override
        public void onServiceConnected(ComponentName name, IBinder binder) {
            service = ((EngineService.LocalBinder) binder).service();
            // 通知から engine が停止された場合は、接続不能な WebView を残さず終了する。
            service.listen(new EngineService.Listener() {
                @Override
                public void engineReady() {
                    consumeEntrance();
                }

                @Override
                public void engineStopped() {
                    finishAndRemoveTask();
                }
            });
            consumeEntrance();
        }

        @Override
        public void onServiceDisconnected(ComponentName name) {
            // OS停止後にserviceを常駐再開しない。利用者がActivityを開き直したときだけ
            // 新しいengineと接続URLを作る。
        }
    };

    /** Engine start is asynchronous; null with no failure means keep waiting. */
    private void consumeEntrance() {
        if (service == null) return;
        String entrance = service.entrance();
        if (entrance != null) {
            showEntrance(entrance);
            return;
        }
        long failure = service.failure();
        if (failure == Mobile.KindNone) return;
        showFailure(failure, service.failureCode(), service.failureDetail());
    }

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        // WindowInsets を子 View の listener で処理する。
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            getWindow().setDecorFitsSystemWindows(false);
        }
        Intent intent = new Intent(this, EngineService.class);
        startForegroundService(intent);
        bound = bindService(intent, connection, Context.BIND_AUTO_CREATE);
    }

    @Override
    protected void onDestroy() {
        releaseService();
        // 構成変更による Activity の破棄では engine を停止しない。
        super.onDestroy();
    }

    /** Activity の参照と bind を同時に外す。起動失敗時にも service を再作成可能にする。 */
    private void releaseService() {
        if (service != null) {
            service.listen(null);
            service = null;
        }
        if (bound) {
            unbindService(connection);
            bound = false;
        }
    }

    private void showEntrance(String entrance) {
        webView = new WebView(this);
        webView.setLayoutParams(new ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        webView.getSettings().setJavaScriptEnabled(true);
        webView.getSettings().setDomStorageEnabled(true);

        // WebView のデバッグはページ内容と session cookie を参照できるため、
        // debuggable ビルドだけで有効にする。
        if ((getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0) {
            WebView.setWebContentsDebuggingEnabled(true);
        }

        // targetSdk 35 以降の edge-to-edge 表示に合わせ、system bar と display cutout の
        // 余白は固定値ではなく WindowInsets から取得する。

        // 何も外へ出さない。この画面が通信する相手は loopback の engine だけである。
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                // fragment に資格情報を含む可能性があるため、URL はログへ出さない。
                Log.e(TAG, "web resource failed: mainFrame=" + request.isForMainFrame()
                        + " code=" + error.getErrorCode());
            }
        });

        // WebView の console を logcat へ転送し、画面描画の失敗を診断できるようにする。
        webView.setWebChromeClient(new WebChromeClient() {
            @Override
            public boolean onConsoleMessage(ConsoleMessage message) {
                Log.i(TAG, "web console [" + message.messageLevel() + "] "
                        + message.message() + " (" + message.lineNumber() + ")");
                return true;
            }
        });

        webView.loadUrl(entrance);
        setContentView(frame(webView));
    }

    /** WindowInsets の padding とページ背景色を適用するコンテナを作る。 */
    private FrameLayout frame(View content) {
        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(chromeColour());
        root.addView(content, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        avoidSystemBars(root);
        // listener 登録前に dispatch 済みの場合に備えて再適用を要求する。
        root.requestApplyInsets();
        return root;
    }

    /** 伏せ字化済みの診断情報と、その場で再試行できる操作を表示する。 */
    private void showFailure(long reason, String code, String detail) {
        // gomobile が生成する定数を使い、Go 側の失敗種別と一致させる。
        int message;
        if (reason == Mobile.KindListenFailed) {
            message = R.string.failure_listen;
        } else if (reason == Mobile.KindStoppedEarly) {
            message = R.string.failure_stopped_early;
        } else if (reason == Mobile.KindStorageUnavailable) {
            message = R.string.failure_storage;
        } else if (reason == Mobile.KindEngineStartFailed) {
            message = R.string.failure_engine_start;
        } else {
            message = R.string.failure_unknown;
        }

        String report = FailureReport.render(
                Mobile.version(), code, detail,
                Build.VERSION.RELEASE, Build.VERSION.SDK_INT,
                Build.MANUFACTURER, Build.MODEL,
                String.join(", ", Arrays.asList(Build.SUPPORTED_ABIS)));
        int gap = dp(16);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(dp(24), dp(32), dp(24), dp(32));

        TextView title = new TextView(this);
        title.setText(message);
        title.setTextSize(20);
        content.addView(title, matchWidthWrapHeight());

        TextView explanation = new TextView(this);
        explanation.setText(R.string.failure_explanation);
        explanation.setTextSize(15);
        LinearLayout.LayoutParams explanationLayout = matchWidthWrapHeight();
        explanationLayout.setMargins(0, gap, 0, 0);
        content.addView(explanation, explanationLayout);

        TextView diagnostics = new TextView(this);
        diagnostics.setText(report);
        diagnostics.setTextIsSelectable(true);
        diagnostics.setTextSize(14);
        diagnostics.setTypeface(android.graphics.Typeface.MONOSPACE);
        LinearLayout.LayoutParams diagnosticsLayout = matchWidthWrapHeight();
        diagnosticsLayout.setMargins(0, gap, 0, 0);
        content.addView(diagnostics, diagnosticsLayout);

        Button copy = new Button(this);
        copy.setText(R.string.failure_copy);
        copy.setOnClickListener(view -> {
            ClipboardManager clipboard = getSystemService(ClipboardManager.class);
            clipboard.setPrimaryClip(ClipData.newPlainText("sshc diagnostics", report));
            Toast.makeText(this, R.string.failure_copied, Toast.LENGTH_SHORT).show();
        });
        LinearLayout.LayoutParams copyLayout = matchWidthWrapHeight();
        copyLayout.setMargins(0, gap, 0, 0);
        content.addView(copy, copyLayout);

        Button retry = new Button(this);
        retry.setText(R.string.failure_retry);
        retry.setOnClickListener(view -> {
            if (service == null) return;
            retry.setEnabled(false);
            retry.setText(R.string.failure_retrying);
            // stopSelf済みのbound serviceを再びstarted stateへ戻す。
            startForegroundService(new Intent(this, EngineService.class));
            if (!service.retry()) {
                retry.setEnabled(true);
                retry.setText(R.string.failure_retry);
            }
        });
        LinearLayout.LayoutParams retryLayout = matchWidthWrapHeight();
        retryLayout.setMargins(0, dp(8), 0, 0);
        content.addView(retry, retryLayout);

        ScrollView scroll = new ScrollView(this);
        scroll.addView(content, new ScrollView.LayoutParams(
                ScrollView.LayoutParams.MATCH_PARENT, ScrollView.LayoutParams.WRAP_CONTENT));
        setContentView(frame(scroll));
    }

    private LinearLayout.LayoutParams matchWidthWrapHeight() {
        return new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }

    /** system bar、display cutout、IME の inset を padding に反映する。 */
    private void avoidSystemBars(View view) {
        view.setOnApplyWindowInsetsListener((target, insets) -> {
            int left, top, right, bottom;
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                // 横向きで左右へ移動する display cutout も含める。
                Insets bars = insets.getInsets(
                        WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
                // decorFitsSystemWindows(false) では adjustResize が効かないため、
                // IME の高さも padding に含める。
                Insets keyboard = insets.getInsets(WindowInsets.Type.ime());
                left = bars.left;
                top = bars.top;
                right = bars.right;
                bottom = Math.max(bars.bottom, keyboard.bottom);
            } else {
                left = insets.getSystemWindowInsetLeft();
                top = insets.getSystemWindowInsetTop();
                right = insets.getSystemWindowInsetRight();
                bottom = insets.getSystemWindowInsetBottom();
            }
            target.setPadding(left, top, right, bottom);
            // WebView の入力接続まで IME inset を届けるため、insets は消費しない。
            return insets;
        });
    }

    /**
     * WindowInsets の余白へ適用する色を返す。
     * web/src/index.css の --ui-toolbar と同期し、ページ読込前の色差を防ぐ。
     */
    private int chromeColour() {
        int night = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        return night == Configuration.UI_MODE_NIGHT_YES ? 0xFF15191C : 0xFFE9EDEF;
    }

    /** configChanges で受けた uiMode の変更をコンテナ背景へ反映する。 */
    @Override
    public void onConfigurationChanged(Configuration configuration) {
        super.onConfigurationChanged(configuration);
        View root = findViewById(android.R.id.content);
        if (root instanceof ViewGroup && ((ViewGroup) root).getChildCount() > 0) {
            ((ViewGroup) root).getChildAt(0).setBackgroundColor(chromeColour());
        }
    }

    /** 戻るキーで WebView の履歴を移動する。androidx.activity への依存は追加しない。 */
    @Override
    public boolean onKeyDown(int keyCode, KeyEvent event) {
        if (keyCode == KeyEvent.KEYCODE_BACK && webView != null && webView.canGoBack()) {
            webView.goBack();
            return true;
        }
        return super.onKeyDown(keyCode, event);
    }
}
