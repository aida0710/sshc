package com.github.aida0710.sshc;

import android.app.NotificationManager;
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
import android.webkit.ServiceWorkerClient;
import android.webkit.ServiceWorkerController;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.view.View;
import android.view.ViewGroup;
import android.view.WindowInsets;
import android.widget.FrameLayout;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.TextView;
import android.widget.ScrollView;
import android.widget.Toast;

import androidx.activity.ComponentActivity;
import androidx.activity.OnBackPressedCallback;

import org.json.JSONObject;

import java.io.IOException;
import java.io.OutputStream;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

import mobile.Mobile;

/** EngineService から接続 URL を受け取り、WebView に表示する Activity。 */
public final class MainActivity extends ComponentActivity {
    private static final String TAG = "sshc";
    private static final int FILE_CHOOSER_REQUEST = 20;
    private static final int SAVE_DESTINATION_REQUEST = 21;
    private static final int NOTIFICATION_PERMISSION_REQUEST = 22;
    private static final int FOLDER_CHOOSER_REQUEST = 23;
    private static final String NOTIFICATION_PERMISSION_ASKED = "notification_permission_asked";
    /** ページの帯の夜と昼の色。web/src/index.css の --ui-toolbar と同期し、ページ読込前の色差を防ぐ。 */
    private static final int CHROME_NIGHT = 0xFF161616;
    private static final int CHROME_DAY = 0xFFFFFFFF;
    /** 失敗の画面で、［コピー］と［やり直す］のあいだを詰める間隔。 */
    private static final int RETRY_GAP_DP = 8;

    private WebView webView;
    private boolean bound;
    private EngineService service;
    private ValueCallback<Uri[]> fileChooser;
    private String pendingSaveRequest;
    private String lastEntrance;
    private String pendingTransferStatus;
    private String webAppearance;
    private int webBarColour;
    private Uri engineOrigin;
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
        public void setAppearance(String appearance, int barColour) {
            runOnUiThread(() -> applySystemBarAppearance(appearance, barColour));
        }

        @Override
        public String notificationPermission() {
            return currentNotificationPermission();
        }

        @Override
        public void requestNotificationPermission() {
            runOnUiThread(() -> requestNotificationPermissionForWeb());
        }

        @Override
        public void showNotification(String tag, String title, String body) {
            terminalNotifications.show(tag, title, body);
        }

        @Override
        public void cancelNotification(String tag) {
            terminalNotifications.cancel(tag);
        }

        @Override
        public String takeOpenedNotification() {
            return takeOpenedNotificationTag();
        }

        @Override
        public void chooseFolder(String requestId) {
            runOnUiThread(() -> openFolderChooser(requestId));
        }

        @Override
        public String takeChosenFolder(String requestId) {
            synchronized (folderLock) {
                if (!requestId.equals(folderRequest) || folderListing == null) return "";
                String listing = folderListing;
                folderListing = null;
                return listing;
            }
        }

        @Override
        public void releaseChosenFolder() {
            chosenFolder.release();
        }
    });
    // 選んだフォルダの一覧と、それを受け取る request ID。bridge は UI スレッドの外から呼ぶので folderLock の中で読み書きする。
    private final Object folderLock = new Object();
    private String folderRequest;
    private String folderListing;
    private ChosenFolder chosenFolder;
    private final TerminalNotifications terminalNotifications = new TerminalNotifications(this);
    private boolean notificationPermissionRequestedByWeb;
    // 押された通知の tag。Web 画面が takeOpenedNotification で受け取るまで持つ。bridge の呼び出しは
    // UI スレッドの外から来るので、読み書きは openedNotificationLock の中で行う。
    private final Object openedNotificationLock = new Object();
    private String openedNotificationTag = "";

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
        // 閉じていたアプリを通知から開いた場合。Web 画面は起動したときに受け取りに来る。
        rememberOpenedNotification(getIntent());
        chosenFolder = new ChosenFolder(getContentResolver());
        // service worker が扱う画面では、その通信は WebViewClient を通らない。選んだフォルダのファイルの
        // URL にはどちらの経路からでも答える。
        ServiceWorkerController.getInstance().setServiceWorkerClient(new ServiceWorkerClient() {
            @Override
            public WebResourceResponse shouldInterceptRequest(WebResourceRequest request) {
                return chosenFolderFile(request.getUrl());
            }
        });
        // WindowInsets を子 View の listener で処理する。
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            getWindow().setDecorFitsSystemWindows(false);
        }
        registerBackCallback();
        Intent intent = new Intent(this, EngineService.class);
        startForegroundService(intent);
        bound = bindService(intent, connection, Context.BIND_AUTO_CREATE);
    }

    /** ターミナルの通知から開いたとき、どの通知かを覚え、Web 画面へ受け取りに来るよう知らせる。 */
    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        if (!rememberOpenedNotification(intent) || webView == null) return;
        webView.evaluateJavascript("window.dispatchEvent(new CustomEvent('sshc-android-notification-opened'));", null);
    }

    /**
     * 通知から開いた Intent なら tag を覚えて true を返す。この Activity は exported なので、
     * ほかのアプリが付けた tag は受け取らず、このアプリが出した通知の tag だけを受け取る。
     */
    private boolean rememberOpenedNotification(Intent intent) {
        String tag = intent.getStringExtra(TerminalNotifications.EXTRA_TAG);
        if (tag == null || !NativeBridge.validNotificationTag(tag) || !TerminalNotifications.takeIssued(tag)) return false;
        synchronized (openedNotificationLock) {
            openedNotificationTag = tag;
        }
        return true;
    }

    private String takeOpenedNotificationTag() {
        synchronized (openedNotificationLock) {
            String tag = openedNotificationTag;
            openedNotificationTag = "";
            return tag;
        }
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
            public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
                return chosenFolderFile(request.getUrl());
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
                    startActivityForResult(fileChooserIntent(params), FILE_CHOOSER_REQUEST);
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
                callback.onReceiveValue(chosenFiles(resultCode, data));
            }
            return;
        }
        if (requestCode == FOLDER_CHOOSER_REQUEST) {
            Uri tree = resultCode == RESULT_OK && data != null ? data.getData() : null;
            if (tree == null) {
                reportChosenFolder("cancelled");
            } else {
                listChosenFolder(tree);
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

    /**
     * FileChooserParams.createIntent は accept=".conf,text/plain" で text/plain だけに絞り込み、
     * application/octet-stream として登録される .conf を選べなくするため、選択画面への要求は自前で組み立てる。
     */
    private static Intent fileChooserIntent(WebChromeClient.FileChooserParams params) {
        Intent intent = new Intent(Intent.ACTION_GET_CONTENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType("*/*");
        List<String> mimeTypes = FileChooser.mimeTypes(params.getAcceptTypes());
        if (!mimeTypes.isEmpty()) intent.putExtra(Intent.EXTRA_MIME_TYPES, mimeTypes.toArray(new String[0]));
        if (params.getMode() == WebChromeClient.FileChooserParams.MODE_OPEN_MULTIPLE) {
            intent.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true);
        }
        intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        return intent;
    }

    /** 選択画面の結果を WebView へ返す形にする。取り消しと空の結果は null で知らせる。 */
    private static Uri[] chosenFiles(int resultCode, Intent data) {
        if (resultCode != RESULT_OK || data == null) return null;
        List<Uri> clipItems = new ArrayList<>();
        ClipData clipData = data.getClipData();
        if (clipData != null) {
            for (int index = 0; index < clipData.getItemCount(); index++) {
                Uri uri = clipData.getItemAt(index).getUri();
                if (uri != null) clipItems.add(uri);
            }
        }
        List<Uri> chosen = FileChooser.chosen(data.getData(), clipItems);
        return chosen.isEmpty() ? null : chosen.toArray(new Uri[0]);
    }

    /** フォルダの選択画面を開く。選び終えたら listChosenFolder が中身を一覧にする。 */
    private void openFolderChooser(String requestId) {
        synchronized (folderLock) {
            folderRequest = requestId;
            folderListing = null;
        }
        try {
            startActivityForResult(new Intent(Intent.ACTION_OPEN_DOCUMENT_TREE), FOLDER_CHOOSER_REQUEST);
        } catch (RuntimeException error) {
            reportChosenFolder("failed");
            Toast.makeText(this, R.string.file_picker_unavailable, Toast.LENGTH_SHORT).show();
        }
    }

    /** 大きなフォルダでも画面を止めないよう、一覧は別のスレッドで作る。 */
    private void listChosenFolder(Uri tree) {
        new Thread(() -> {
            String status = "ready";
            try {
                String listing = chosenFolder.list(tree);
                synchronized (folderLock) {
                    folderListing = listing;
                }
            } catch (ChosenFolder.TooManyFiles error) {
                status = "too_many";
            } catch (org.json.JSONException | RuntimeException error) {
                Log.w(TAG, "could not list the chosen folder");
                status = "failed";
            }
            String reported = status;
            runOnUiThread(() -> reportChosenFolder(reported));
        }, "sshc-chosen-folder").start();
    }

    private void reportChosenFolder(String status) {
        String requestId;
        synchronized (folderLock) {
            requestId = folderRequest;
        }
        if (webView == null || requestId == null) return;
        String script = "window.dispatchEvent(new CustomEvent('sshc-android-folder',{detail:{requestId:"
                + JSONObject.quote(requestId) + ",status:" + JSONObject.quote(status) + "}}));";
        webView.evaluateJavascript(script, null);
    }

    /** engine の origin の、選んだフォルダのファイルの URL にだけ答える。 */
    private WebResourceResponse chosenFolderFile(Uri url) {
        Uri origin = engineOrigin;
        if (origin == null || chosenFolder == null || !origin.getScheme().equals(url.getScheme())
                || !origin.getHost().equals(url.getHost()) || origin.getPort() != url.getPort()) return null;
        return chosenFolder.open(url);
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

    /**
     * ブラウザの Notification.permission と同じ 3 値にする。Android は「まだ聞いていない」と
     * 「2 回断られてもう聞けない」を区別しないため、聞いたかどうかの記録と説明の要否で見分ける。
     */
    private String currentNotificationPermission() {
        boolean enabled = getSystemService(NotificationManager.class).areNotificationsEnabled();
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU
                || checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED) {
            return enabled ? "granted" : "denied";
        }
        boolean asked = getPreferences(Context.MODE_PRIVATE).getBoolean(NOTIFICATION_PERMISSION_ASKED, false);
        return asked && !shouldShowRequestPermissionRationale(Manifest.permission.POST_NOTIFICATIONS) ? "denied" : "default";
    }

    /** Web 画面の［通知を有効にする］から。聞く必要が無ければその場で今の状態を返す。 */
    private void requestNotificationPermissionForWeb() {
        if (!"default".equals(currentNotificationPermission())) {
            reportNotificationPermission();
            return;
        }
        notificationPermissionRequestedByWeb = true;
        requestNotificationPermission();
    }

    private void reportNotificationPermission() {
        if (webView == null) return;
        String script = "window.dispatchEvent(new CustomEvent('sshc-android-notification-permission',{detail:{status:"
                + JSONObject.quote(currentNotificationPermission()) + "}}));";
        webView.evaluateJavascript(script, null);
    }

    private void requestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU
                && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            getPreferences(Context.MODE_PRIVATE).edit().putBoolean(NOTIFICATION_PERMISSION_ASKED, true).apply();
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, NOTIFICATION_PERMISSION_REQUEST);
        }
    }

    /** 転送の結果を通知する。通知の権限をまだ尋ねていなければ、先に尋ねる。 */
    private void showTransferNotification(String status) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU
                && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            if (getPreferences(Context.MODE_PRIVATE).getBoolean(NOTIFICATION_PERMISSION_ASKED, false)) return;
            pendingTransferStatus = status;
            requestNotificationPermission();
            return;
        }
        pendingTransferStatus = null;
        new TransferNotifications(this).show(status);
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode != NOTIFICATION_PERMISSION_REQUEST) return;
        if (notificationPermissionRequestedByWeb) {
            notificationPermissionRequestedByWeb = false;
            reportNotificationPermission();
        }
        if (pendingTransferStatus == null) return;
        String status = pendingTransferStatus;
        pendingTransferStatus = null;
        if (grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED) {
            showTransferNotification(status);
        }
    }

    /** renderer異常を真っ白な画面にせず、engineを維持したまま再読込できるようにする。 */
    private void showWebViewRecovery() {
        NativeScreenContent content = new NativeScreenContent(this).centredVertically();
        content.title(R.string.webview_recovery_title);
        content.paragraph(R.string.webview_recovery_body);

        Button reload = new Button(this);
        reload.setText(R.string.webview_recovery_reload);
        reload.setEnabled(lastEntrance != null);
        reload.setOnClickListener(view -> {
            if (lastEntrance != null) showEntrance(lastEntrance);
        });
        content.add(reload, NativeScreenContent.GAP_DP);
        setContentView(frame(content.view()));
        showNativeScreenColours();
    }

    /** WindowInsets の padding とページ背景色を適用するコンテナを作る。 */
    private FrameLayout frame(View content) {
        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(chromeColour());
        paintText(content, inkColour());
        root.addView(content, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        avoidSystemBars(root);
        // listener 登録前に dispatch 済みの場合に備えて再適用を要求する。
        root.requestApplyInsets();
        return root;
    }

    /**
     * ネイティブの画面（起動の失敗、画面の停止からの復旧）は、システムのライト・ダークに従う。
     * Web 側のテーマは覚えないので、表示中に切り替えると onConfigurationChanged が塗り直す。
     */
    private void showNativeScreenColours() {
        webAppearance = null;
        paintSystemBars(systemAppearance(), chromeColour());
    }

    /** Web側で解決したthemeをsystem barの色とiconへ反映する。 */
    private void applySystemBarAppearance(String appearance, int barColour) {
        webAppearance = appearance;
        webBarColour = barColour;
        paintSystemBars(appearance, barColour);
    }

    private void paintSystemBars(String appearance, int colour) {
        boolean dark = "dark".equals(appearance);
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
        NativeScreenContent content = new NativeScreenContent(this);
        content.title(message);
        content.paragraph(R.string.failure_explanation);

        TextView diagnostics = new TextView(this);
        diagnostics.setText(report);
        diagnostics.setTextIsSelectable(true);
        diagnostics.setTextSize(14);
        diagnostics.setTypeface(android.graphics.Typeface.MONOSPACE);
        content.add(diagnostics, NativeScreenContent.GAP_DP);

        Button copy = new Button(this);
        copy.setText(R.string.failure_copy);
        copy.setOnClickListener(view -> {
            ClipboardManager clipboard = getSystemService(ClipboardManager.class);
            clipboard.setPrimaryClip(ClipData.newPlainText("sshc diagnostics", report));
            Toast.makeText(this, R.string.failure_copied, Toast.LENGTH_SHORT).show();
        });
        content.add(copy, NativeScreenContent.GAP_DP);

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
        content.add(retry, RETRY_GAP_DP);

        ScrollView scroll = new ScrollView(this);
        scroll.addView(content.view(), new ScrollView.LayoutParams(
                ScrollView.LayoutParams.MATCH_PARENT, ScrollView.LayoutParams.WRAP_CONTENT));
        setContentView(frame(scroll));
        showNativeScreenColours();
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
     * web/src/theme/themes/ のライト・ダークの --ui-toolbar と同期し、ページ読込前の色差を防ぐ。
     */
    private int chromeColour() {
        int night = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        return night == Configuration.UI_MODE_NIGHT_YES ? CHROME_NIGHT : CHROME_DAY;
    }

    /**
     * ネイティブの画面の文字の色を返す。web/src/theme/themes/ のライト・ダークの --ui-ink と同期する。
     * Activity のテーマは起動時の uiMode で決まり、configChanges で受けた切り替えでは
     * 変わらないので、背景（chromeColour）と一緒に文字の色も塗り直す。
     */
    private int inkColour() {
        int night = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        return night == Configuration.UI_MODE_NIGHT_YES ? 0xFFE8E9EB : 0xFF1F2326;
    }

    /** view とその中の文字（ボタンを除く）を colour で塗る。 */
    private static void paintText(View view, int colour) {
        if (view instanceof Button) return;
        if (view instanceof TextView) {
            ((TextView) view).setTextColor(colour);
            return;
        }
        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) view;
            for (int index = 0; index < group.getChildCount(); index++) paintText(group.getChildAt(index), colour);
        }
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
            applySystemBarAppearance(webAppearance, webBarColour);
            return;
        }
        paintSystemBars(systemAppearance(), chromeColour());
        View root = findViewById(android.R.id.content);
        if (root instanceof ViewGroup && ((ViewGroup) root).getChildCount() > 0) {
            View screen = ((ViewGroup) root).getChildAt(0);
            screen.setBackgroundColor(chromeColour());
            paintText(screen, inkColour());
        }
    }

    /** AndroidX経由で旧OSのボタンとAndroid 13以降のpredictive backを同じ処理へ渡す。 */
    private void registerBackCallback() {
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                navigateBack();
            }
        });
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
}
