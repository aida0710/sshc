package com.github.aida0710.sshc;

import android.app.Activity;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.ServiceConnection;
import android.os.Bundle;
import android.os.IBinder;
import android.view.KeyEvent;
import android.view.ViewGroup;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.TextView;

/**
 * 画面はこの 1 枚である。engine は EngineService が持っているので、ここがする
 * のは「入口を受け取って WebView へ渡す」ことだけ。
 */
public final class MainActivity extends Activity {
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
        // 何も外へ出さない。この画面が話す相手は loopback の engine だけである。
        webView.setWebViewClient(new WebViewClient());
        webView.loadUrl(entrance);
        setContentView(webView);
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
        setContentView(view);
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
