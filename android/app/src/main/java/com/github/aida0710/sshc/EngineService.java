package com.github.aida0710.sshc;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Intent;
import android.os.Binder;
import android.os.IBinder;
import android.util.Log;

import mobile.Mobile;

/**
 * engine の寿命はここが持つ。
 *
 * <p><b>Activity に持たせない。</b> 別アプリへ切り替えた瞬間に Activity は止め
 * られるので、そこに engine を置けば SSH セッションが切れる。パスワードをコピー
 * しに行って戻ったら落ちている、というのが最初に起きる。
 */
public final class EngineService extends Service {
    private static final String TAG = "sshc";
    private static final String CHANNEL = "engine";
    private static final int NOTIFICATION_ID = 1;

    private final LocalBinder binder = new LocalBinder();
    private String entrance;
    private boolean started;
    private int failure;

    public final class LocalBinder extends Binder {
        EngineService service() {
            return EngineService.this;
        }
    }

    /**
     * 入口の URL。engine が起きていなければ null。
     *
     * <p><b>入口は一度しか使えない。</b> URL の fragment は最初の 1 回で使い
     * 切られるので、同じものを読み直すと 2 回目の bootstrap が拒否され、画面は
     * 「開始できませんでした」に落ちる。Activity は設定が変わるたびに作り直され
     * 得る——ダークモードの切り替え、フォントサイズ、分割画面、メモリ逼迫。
     * そのどれでも同じ URL をもう一度渡せば、二度と開かないアプリになる。
     *
     * <p>2 回目からは fragment を落として渡す。ページはクッキーだけで届いた
     * ときに session を更新する道を既に持っており、クッキーは同じプロセスの
     * WebView が持ち続けている。
     */
    String entrance() {
        if (entrance == null) return null;
        String url = entrance;
        int fragment = entrance.indexOf('#');
        if (fragment >= 0) entrance = entrance.substring(0, fragment);
        return url;
    }

    /** 直前の起動が失敗した理由。成功していれば 0。 */
    int failure() {
        return failure;
    }

    @Override
    public void onCreate() {
        super.onCreate();
        // **通知が先である。** startForegroundService から 5 秒以内に
        // startForeground を呼ばないと ANR で落とされる。engine を起こすのは
        // その後でよい。
        startForeground(NOTIFICATION_ID, notification());

        try {
            entrance = Mobile.start(getFilesDir().getAbsolutePath(), getCacheDir().getAbsolutePath());
            started = true;
        } catch (Exception error) {
            // **error のメッセージを保持しない。** 入口の URL を含み得る。
            failure = (int) Mobile.lastStartFailureKind();
            Log.e(TAG, "the engine did not start; reason " + failure);
        }
    }

    @Override
    public void onDestroy() {
        if (started) {
            try {
                Mobile.stop();
            } catch (Exception error) {
                Log.e(TAG, "the engine did not stop cleanly");
            }
            started = false;
            entrance = null;
        }
        super.onDestroy();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        // 死んだら作り直す。ただし intent は配り直さない——起動に引数は無い。
        return START_STICKY;
    }

    @Override
    public IBinder onBind(Intent intent) {
        return binder;
    }

    private Notification notification() {
        NotificationManager manager = getSystemService(NotificationManager.class);
        manager.createNotificationChannel(
                new NotificationChannel(CHANNEL, "sshc", NotificationManager.IMPORTANCE_LOW));
        return new Notification.Builder(this, CHANNEL)
                .setContentTitle(getString(R.string.engine_running))
                .setSmallIcon(android.R.drawable.stat_notify_sync)
                .setOngoing(true)
                .build();
    }
}
