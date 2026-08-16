package com.github.aida0710.sshc;

import android.app.Activity;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.ServiceConnection;
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
import android.widget.TextView;

/**
 * 画面はこの 1 枚である。engine は EngineService が持っているので、ここがする
 * のは「入口を受け取って WebView へ渡す」ことだけ。
 */
public final class MainActivity extends Activity {
    private static final String TAG = "sshc";

    private WebView webView;
    private boolean bound;

    private final ServiceConnection connection = new ServiceConnection() {
        @Override
        public void onServiceConnected(ComponentName name, IBinder binder) {
            EngineService service = ((EngineService.LocalBinder) binder).service();
            String entrance = service.entrance();
            if (entrance == null) {
                showFailure(service.failure());
                return;
            }
            showEntrance(entrance);
        }

        @Override
        public void onServiceDisconnected(ComponentName name) {
            // service が落ちた。START_STICKY で作り直されるので、次に
            // 繋がったときに onServiceConnected がまた入口を配る。
        }
    };

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        // **これを言わないと、挿入量は子に届かない。** decor が自分で
        // fitsSystemWindows を処理して消費してしまうので、こちらの listener
        // には全部ゼロが渡る——余白が付かないのはそれが理由だった。
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            getWindow().setDecorFitsSystemWindows(false);
        }
        Intent intent = new Intent(this, EngineService.class);
        startForegroundService(intent);
        bound = bindService(intent, connection, Context.BIND_AUTO_CREATE);
    }

    @Override
    protected void onDestroy() {
        if (bound) {
            unbindService(connection);
            bound = false;
        }
        // **engine は止めない。** 画面が回っただけで SSH セッションが切れる
        // ことになる。engine を畳むのは service を止めるときである。
        super.onDestroy();
    }

    private void showEntrance(String entrance) {
        webView = new WebView(this);
        webView.setLayoutParams(new ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));
        webView.getSettings().setJavaScriptEnabled(true);
        webView.getSettings().setDomStorageEnabled(true);

        // **targetSdk 35 以降、edge-to-edge は強制である。** 何もしなければ
        // WebView はステータスバーとナビゲーションバーの下にも描かれ、画面の
        // 上端と下端が読めなくなる。
        //
        // **決め打ちの数値を置かない。** 必要な余白は端末ごとに違う——
        // ステータスバーの高さも、ジェスチャーバーの有無も、ノッチや
        // パンチホールの張り出しも。WindowInsets はそれを実測値で答えるので、
        // 尋ねればよい。

        // 何も外へ出さない。この画面が話す相手は loopback の engine だけである。
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                // **URL を出さない。** 入口の fragment を含み得る。落ちたのが
                // 主文書かどうかと、その理由だけを残す。
                Log.e(TAG, "web resource failed: mainFrame=" + request.isForMainFrame()
                        + " code=" + error.getErrorCode());
            }
        });

        // **画面が白いままのとき、答えはここにしかない。** WebView の console は
        // どこにも出ないので、logcat へ渡す。これが無いと、engine が起きたのに
        // 画面が出ないという状態を、外から見分ける手段が無い。
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

    /**
     * 挿入量を受け止める器。
     *
     * <p>WebView に直接 padding を置かず、1 枚挟む。padding の外側に見えるのは
     * この器の背景であり、WebView 自身の背景ではページの読み込み前に白く光る。
     */
    private FrameLayout frame(View content) {
        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(chromeColour());
        root.addView(content, new FrameLayout.LayoutParams(
                FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT));
        avoidSystemBars(root);
        // 一度きりの配布を自分から要求する。attach のタイミング次第では、
        // listener を付けた後の dispatch が既に済んでいることがある。
        root.requestApplyInsets();
        return root;
    }

    /**
     * **Go の error 文を出さない。** 入口の URL を含み得るので、番号に対応する
     * こちらの文字列だけを出す。
     */
    private void showFailure(int reason) {
        int message;
        switch (reason) {
            case 2:
                message = R.string.failure_already_started;
                break;
            case 3:
                message = R.string.failure_listen;
                break;
            case 4:
                message = R.string.failure_stopped_early;
                break;
            default:
                message = R.string.failure_unknown;
        }
        TextView view = new TextView(this);
        view.setText(message);
        view.setPadding(48, 48, 48, 48);
        setContentView(frame(view));
    }

    /**
     * システムバーの下に潜らないよう、挿入量をそのまま padding にする。
     *
     * <p>setFitsSystemWindows では届かなかった。自分で聞けば、返ってくるのは
     * この端末の実測値である。
     */
    private void avoidSystemBars(View view) {
        view.setOnApplyWindowInsetsListener((target, insets) -> {
            int left, top, right, bottom;
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                // 表示の切り欠きも一緒に見る。横向きにすると、ノッチは
                // ステータスバーではなく左右の縁に来る。
                Insets bars = insets.getInsets(
                        WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
                left = bars.left;
                top = bars.top;
                right = bars.right;
                bottom = bars.bottom;
            } else {
                left = insets.getSystemWindowInsetLeft();
                top = insets.getSystemWindowInsetTop();
                right = insets.getSystemWindowInsetRight();
                bottom = insets.getSystemWindowInsetBottom();
            }
            target.setPadding(left, top, right, bottom);
            // 受け止めたので、この先へは渡さない。
            return Build.VERSION.SDK_INT >= Build.VERSION_CODES.R
                    ? WindowInsets.CONSUMED
                    : insets.consumeSystemWindowInsets();
        });
    }

    /**
     * 余白の帯を塗る色。
     *
     * <p>padding の外側に見えるのは WebView 自身の背景なので、ページと違う色だと
     * 上端に別の板が乗っているように見える。値は web/src/index.css の
     * --ui-toolbar と同じもので、**そちらを変えたらここも変える**。2 か所に
     * 書くのは、ネイティブの外殻がページのトークンを読む手段を持たないためである。
     */
    private int chromeColour() {
        int night = getResources().getConfiguration().uiMode & Configuration.UI_MODE_NIGHT_MASK;
        return night == Configuration.UI_MODE_NIGHT_YES ? 0xFF2A2A2C : 0xFFFBFBFD;
    }

    /**
     * uiMode を configChanges で受けているので、テーマが変わっても Activity は
     * 作り直されない。**帯の色だけが取り残される**ので、ここで塗り直す。
     */
    @Override
    public void onConfigurationChanged(Configuration configuration) {
        super.onConfigurationChanged(configuration);
        View root = findViewById(android.R.id.content);
        if (root instanceof ViewGroup && ((ViewGroup) root).getChildCount() > 0) {
            ((ViewGroup) root).getChildAt(0).setBackgroundColor(chromeColour());
        }
    }

    /**
     * 戻るキーは WebView の履歴に繋ぐ。web/src/routing がセクションを URL として
     * 持っているので、履歴はそのまま Android の戻るキーの意味になる。
     *
     * <p>onBackPressed ではなく onKeyDown なのは、前者を今の形で使うには
     * androidx.activity が要るからである。glue 150 行のために依存を 1 つ増やさない。
     */
    @Override
    public boolean onKeyDown(int keyCode, KeyEvent event) {
        if (keyCode == KeyEvent.KEYCODE_BACK && webView != null && webView.canGoBack()) {
            webView.goBack();
            return true;
        }
        return super.onKeyDown(keyCode, event);
    }
}
