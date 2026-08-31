package com.github.aida0710.sshc;

import android.app.Activity;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.annotation.TargetApi;
import android.Manifest;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.ServiceConnection;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.content.res.Configuration;
import android.graphics.Insets;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.IBinder;
import android.util.Log;
import android.webkit.ConsoleMessage;
import android.webkit.RenderProcessGoneDetail;
import android.webkit.ValueCallback;
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
import android.window.OnBackInvokedDispatcher;

import org.json.JSONObject;

import java.io.IOException;
import java.io.OutputStream;
import java.util.Arrays;

import mobile.Mobile;

/** EngineService から接続 URL を受け取り、WebView に表示する Activity。 */
public final class MainActivity extends Activity {
    private static final String TAG = "sshc";
    private static final int FILE_CHOOSER_REQUEST = 20;
    private static final int SAVE_DESTINATION_REQUEST = 21;
    private static final int NOTIFICATION_PERMISSION_REQUEST = 22;
    private static final String TRANSFER_CHANNEL = "transfers";
    private static final String NOTIFICATION_PERMISSION_ASKED = "notification_permission_asked";

    private WebView webView;
    private boolean bound;
    private EngineService service;
    private ValueCallback<Uri[]> fileChooser;
    private String pendingSaveRequest;
    private String lastEntrance;
    private String pendingTransferStatus;
    private String webAppearance;
    private Uri engineOrigin;
    private int transferNotificationSequence = 100;
    private final NativeBridge nativeBridge = new NativeBridge(new NativeBridge.Host() {
        @Override
        public void chooseSaveDestination(String requestId, String suggestedName, String mimeType) {
            runOnUiThread(() -> openSaveDestination(requestId, suggestedName, mimeType));
        }

        @Override
        public void notifyTransfer(String status) {
            runOnUiThread(() -> showTransferNotification(status));
        }

        @Override
        public void setAppearance(String appearance) {
            runOnUiThread(() -> applySystemBarAppearance(appearance));
        }
    });

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
        showPreviousRuntimeNotice();
        // WindowInsets を子 View の listener で処理する。
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            getWindow().setDecorFitsSystemWindows(false);
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            registerBackCallback();
        }
        Intent intent = new Intent(this, EngineService.class);
        startForegroundService(intent);
        bound = bindService(intent, connection, Context.BIND_AUTO_CREATE);
    }

    /** OSによる前回の停止を次回起動時に一度だけ説明する。 */
    private void showPreviousRuntimeNotice() {
        android.content.SharedPreferences preferences = getSharedPreferences(
                EngineService.RUNTIME_PREFERENCES, Context.MODE_PRIVATE);
        String reason = preferences.getString(EngineService.LAST_STOP_REASON, "");
        if (!EngineService.STOP_REASON_DATA_SYNC_TIMEOUT.equals(reason)) return;
        preferences.edit().remove(EngineService.LAST_STOP_REASON).apply();
        Toast.makeText(this, R.string.engine_data_sync_timeout, Toast.LENGTH_LONG).show();
    }

    @Override
    protected void onDestroy() {
        if (fileChooser != null) {
            fileChooser.onReceiveValue(null);
            fileChooser = null;
        }
        nativeBridge.close();
        if (webView != null) {
            webView.removeJavascriptInterface("sshcAndroid");
            webView.destroy();
            webView = null;
        }
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
        if (webView != null && Entrance.isAlreadyShowing(lastEntrance, entrance)) return;
        if (webView != null) {
            webView.removeJavascriptInterface("sshcAndroid");
            webView.destroy();
            nativeBridge.close();
        }
        lastEntrance = Entrance.withoutFragment(entrance);
        engineOrigin = Uri.parse(lastEntrance);
        webView = new WebView(this);
        webView.setLayoutParams(new ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        webView.getSettings().setJavaScriptEnabled(true);
        webView.getSettings().setDomStorageEnabled(true);
        webView.addJavascriptInterface(nativeBridge, "sshcAndroid");
        // WebView のデバッグはページ内容と session cookie を参照できるため、
        // debuggable ビルドだけで有効にする。
        if ((getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0) {
            WebView.setWebContentsDebuggingEnabled(true);
        }

        // targetSdk 35 以降の edge-to-edge 表示に合わせ、system bar と display cutout の
        // 余白は固定値ではなく WindowInsets から取得する。

        // WebView内の遷移は現在のengine originだけに限定し、通常の外部リンクは端末browserへ渡す。
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                Uri target = request.getUrl();
                if (engineOrigin != null
                        && engineOrigin.getScheme().equals(target.getScheme())
                        && engineOrigin.getHost().equals(target.getHost())
                        && engineOrigin.getPort() == target.getPort()) return false;
                if (request.isForMainFrame()
                        && !"127.0.0.1".equals(target.getHost())
                        && ("https".equals(target.getScheme()) || "http".equals(target.getScheme()))) {
                    try {
                        startActivity(new Intent(Intent.ACTION_VIEW, target));
                    } catch (RuntimeException error) {
                        Log.w(TAG, "no activity could open an external link");
                    }
                }
                return true;
            }

            @Override
            public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                // fragment に資格情報を含む可能性があるため、URL はログへ出さない。
                Log.e(TAG, "web resource failed: mainFrame=" + request.isForMainFrame()
                        + " code=" + error.getErrorCode());
            }

            @Override
            public boolean onRenderProcessGone(WebView view, RenderProcessGoneDetail detail) {
                Log.e(TAG, detail.didCrash() ? "WebView renderer crashed" : "WebView renderer was reclaimed");
                view.removeJavascriptInterface("sshcAndroid");
                if (view.getParent() instanceof ViewGroup) ((ViewGroup) view.getParent()).removeView(view);
                view.destroy();
                if (webView == view) webView = null;
                nativeBridge.close();
                showWebViewRecovery();
                return true;
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

            @Override
            public boolean onShowFileChooser(
                    WebView view,
                    ValueCallback<Uri[]> callback,
                    WebChromeClient.FileChooserParams params) {
                if (fileChooser != null) fileChooser.onReceiveValue(null);
                fileChooser = callback;
                try {
                    Intent intent = params.createIntent();
                    intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
                    startActivityForResult(intent, FILE_CHOOSER_REQUEST);
                } catch (RuntimeException error) {
                    fileChooser = null;
                    callback.onReceiveValue(null);
                    Toast.makeText(MainActivity.this, R.string.file_picker_unavailable, Toast.LENGTH_SHORT).show();
                }
                return true;
            }
        });

        webView.loadUrl(entrance);
        setContentView(frame(webView));
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        if (requestCode == FILE_CHOOSER_REQUEST) {
            ValueCallback<Uri[]> callback = fileChooser;
            fileChooser = null;
            if (callback != null) {
                callback.onReceiveValue(WebChromeClient.FileChooserParams.parseResult(resultCode, data));
            }
            return;
        }
        if (requestCode == SAVE_DESTINATION_REQUEST) {
            String requestId = pendingSaveRequest;
            pendingSaveRequest = null;
            if (requestId == null) return;
            if (resultCode != RESULT_OK || data == null || data.getData() == null) {
                reportSaveDestination(requestId, "cancelled");
                return;
            }
            try {
                OutputStream output = getContentResolver().openOutputStream(data.getData(), "w");
                if (!nativeBridge.openSave(requestId, output)) {
                    if (output != null) output.close();
                    reportSaveDestination(requestId, "failed");
                    return;
                }
                reportSaveDestination(requestId, "ready");
            } catch (IOException | RuntimeException error) {
                Log.w(TAG, "could not open the selected save destination");
                reportSaveDestination(requestId, "failed");
            }
            return;
        }
        super.onActivityResult(requestCode, resultCode, data);
    }

    /** Storage Access Frameworkへ保存先だけを選ばせ、内容はbridge経由で順次書き込む。 */
    private void openSaveDestination(String requestId, String suggestedName, String mimeType) {
        if (pendingSaveRequest != null) {
            reportSaveDestination(requestId, "failed");
            return;
        }
        Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType(mimeType);
        intent.putExtra(Intent.EXTRA_TITLE, suggestedName);
        intent.addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION);
        pendingSaveRequest = requestId;
        try {
            startActivityForResult(intent, SAVE_DESTINATION_REQUEST);
        } catch (RuntimeException error) {
            pendingSaveRequest = null;
            reportSaveDestination(requestId, "failed");
            Toast.makeText(this, R.string.file_picker_unavailable, Toast.LENGTH_SHORT).show();
        }
    }

    /** 選択結果をrequest IDだけでWeb側へ返し、URI自体は公開しない。 */
    private void reportSaveDestination(String requestId, String status) {
        if (webView == null) return;
        String script = "window.dispatchEvent(new CustomEvent('sshc-android-save',{detail:{requestId:"
                + JSONObject.quote(requestId) + ",status:" + JSONObject.quote(status) + "}}));";
        webView.evaluateJavascript(script, null);
    }

    private void requestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU
                && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            getPreferences(Context.MODE_PRIVATE).edit().putBoolean(NOTIFICATION_PERMISSION_ASKED, true).apply();
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, NOTIFICATION_PERMISSION_REQUEST);
        }
    }

    /** Web画面を閉じていても転送結果が分かる、内容を伏せた端末通知。 */
    private void showTransferNotification(String status) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU
                && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            if (getPreferences(Context.MODE_PRIVATE).getBoolean(NOTIFICATION_PERMISSION_ASKED, false)) return;
            pendingTransferStatus = status;
            requestNotificationPermission();
            return;
        }
        pendingTransferStatus = null;
        NotificationManager manager = getSystemService(NotificationManager.class);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            manager.createNotificationChannel(new NotificationChannel(
                    TRANSFER_CHANNEL, getString(R.string.transfer_channel), NotificationManager.IMPORTANCE_DEFAULT));
        }
        Intent open = new Intent(this, MainActivity.class)
                .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        android.app.PendingIntent content = android.app.PendingIntent.getActivity(
                this, 2, open, android.app.PendingIntent.FLAG_IMMUTABLE | android.app.PendingIntent.FLAG_UPDATE_CURRENT);
        int message = "completed".equals(status) ? R.string.transfer_completed : R.string.transfer_failed;
        Notification.Builder builder = Build.VERSION.SDK_INT >= Build.VERSION_CODES.O
                ? new Notification.Builder(this, TRANSFER_CHANNEL)
                : new Notification.Builder(this);
        builder.setSmallIcon(android.R.drawable.stat_notify_sync)
                .setContentTitle(getString(R.string.app_name))
                .setContentText(getString(message))
                .setContentIntent(content)
                .setAutoCancel(true)
                .setCategory(Notification.CATEGORY_PROGRESS)
                .setVisibility(Notification.VISIBILITY_PRIVATE);
        manager.notify(transferNotificationSequence++, builder.build());
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode != NOTIFICATION_PERMISSION_REQUEST || pendingTransferStatus == null) return;
        String status = pendingTransferStatus;
        pendingTransferStatus = null;
        if (grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED) {
            showTransferNotification(status);
        }
    }

    /** renderer異常を真っ白な画面にせず、engineを維持したまま再読込できるようにする。 */
    private void showWebViewRecovery() {
        int gap = dp(16);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setGravity(android.view.Gravity.CENTER_VERTICAL);
        content.setPadding(dp(24), dp(32), dp(24), dp(32));

        TextView title = new TextView(this);
        title.setText(R.string.webview_recovery_title);
        title.setTextSize(20);
        content.addView(title, matchWidthWrapHeight());

        TextView explanation = new TextView(this);
        explanation.setText(R.string.webview_recovery_body);
        explanation.setTextSize(15);
        LinearLayout.LayoutParams explanationLayout = matchWidthWrapHeight();
        explanationLayout.setMargins(0, gap, 0, 0);
        content.addView(explanation, explanationLayout);

        Button reload = new Button(this);
        reload.setText(R.string.webview_recovery_reload);
        reload.setEnabled(lastEntrance != null);
        reload.setOnClickListener(view -> {
            if (lastEntrance != null) showEntrance(lastEntrance);
        });
        LinearLayout.LayoutParams reloadLayout = matchWidthWrapHeight();
        reloadLayout.setMargins(0, gap, 0, 0);
        content.addView(reload, reloadLayout);
        setContentView(frame(content));
        applySystemBarAppearance(systemAppearance());
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

    /** Web側で解決したthemeをsystem barの色とiconへ反映する。 */
    private void applySystemBarAppearance(String appearance) {
        webAppearance = appearance;
        boolean dark = "dark".equals(appearance);
        int colour = dark ? 0xFF1B1D1F : 0xFFECECEB;
        getWindow().setStatusBarColor(colour);
        getWindow().setNavigationBarColor(colour);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) getWindow().setNavigationBarDividerColor(colour);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            getWindow().setStatusBarContrastEnforced(false);
            getWindow().setNavigationBarContrastEnforced(false);
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            android.view.WindowInsetsController controller = getWindow().getInsetsController();
            if (controller != null) {
                int darkIcons = android.view.WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS
                        | android.view.WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
                controller.setSystemBarsAppearance(dark ? 0 : darkIcons, darkIcons);
            }
        } else {
            int flags = getWindow().getDecorView().getSystemUiVisibility();
            int darkIcons = View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR;
            getWindow().getDecorView().setSystemUiVisibility(dark ? flags & ~darkIcons : flags | darkIcons);
        }
        View root = findViewById(android.R.id.content);
        if (root instanceof ViewGroup && ((ViewGroup) root).getChildCount() > 0) {
            ((ViewGroup) root).getChildAt(0).setBackgroundColor(colour);
        }
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
        applySystemBarAppearance(systemAppearance());
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
        return night == Configuration.UI_MODE_NIGHT_YES ? 0xFF1B1D1F : 0xFFECECEB;
    }

    private String systemAppearance() {
        int night = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        return night == Configuration.UI_MODE_NIGHT_YES ? "dark" : "light";
    }

    /** configChanges で受けた uiMode の変更をコンテナ背景へ反映する。 */
    @Override
    public void onConfigurationChanged(Configuration configuration) {
        super.onConfigurationChanged(configuration);
        if (webAppearance != null) {
            applySystemBarAppearance(webAppearance);
            return;
        }
        View root = findViewById(android.R.id.content);
        if (root instanceof ViewGroup && ((ViewGroup) root).getChildCount() > 0) {
            ((ViewGroup) root).getChildAt(0).setBackgroundColor(chromeColour());
        }
    }

    /** Android 13 以降のボタン・ジェスチャーによる戻る操作を受け取る。 */
    @TargetApi(Build.VERSION_CODES.TIRAMISU)
    private void registerBackCallback() {
        getOnBackInvokedDispatcher().registerOnBackInvokedCallback(
                OnBackInvokedDispatcher.PRIORITY_DEFAULT, this::navigateBack);
    }

    /** Web画面の一時UIと履歴を優先し、ホームならアプリをバックグラウンドへ戻す。 */
    private void navigateBack() {
        if (webView == null) {
            moveTaskToBack(true);
            return;
        }
        // sshcのrouteはページ読込ではなくhistory.pushStateで積まれるため、
        // WebView.canGoBack()だけでは履歴を検出できない。ページ自身に戻らせる。
        webView.evaluateJavascript(
                "(() => {"
                        + "const modal=document.querySelector('[role=dialog]');"
                        + "if(modal){document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));return true;}"
                        + "const event=new Event('sshc-android-back',{cancelable:true});"
                        + "window.dispatchEvent(event);if(event.defaultPrevented)return true;"
                        + "if(location.pathname==='/'&&!location.search)return false;history.back();return true;"
                        + "})()",
                consumed -> {
                    if ("true".equals(consumed)) return;
                    // 将来、同一originの実ページ遷移を使う場合も通常のWebView履歴を失わない。
                    if (webView.canGoBack()) webView.goBack();
                    else moveTaskToBack(true);
                });
    }

    /** Android 12 以前とハードウェアキーでも同じ戻る動作を行う。 */
    @Override
    public boolean onKeyDown(int keyCode, KeyEvent event) {
        if (keyCode == KeyEvent.KEYCODE_BACK) {
            navigateBack();
            return true;
        }
        return super.onKeyDown(keyCode, event);
    }
}
