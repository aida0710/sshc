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
    private int failure;

    public final class LocalBinder extends Binder {
        EngineService service() {
            return EngineService.this;
        }
    }

    /** 入口の URL。engine が起きていなければ null。 */
    String entrance() {
        return entrance;
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

        // 測定: この端末で名前が引けるか、開けるシェルがあるか。
        // どちらも Android 固有の前提であり、logcat に答えを残す。
        Log.i(TAG, "probe dns: " + Mobile.probeDNS("github.com"));
        Log.i(TAG, "probe shell: " + Mobile.probeShell());

        try {
            entrance = Mobile.start(getFilesDir().getAbsolutePath(), getCacheDir().getAbsolutePath());
        } catch (Exception error) {
            // **error のメッセージを保持しない。** 入口の URL を含み得る。
            failure = (int) Mobile.lastStartFailureKind();
            Log.e(TAG, "the engine did not start; reason " + failure);
        }
    }

    @Override
    public void onDestroy() {
        if (entrance != null) {
            try {
                Mobile.stop();
            } catch (Exception error) {
                Log.e(TAG, "the engine did not stop cleanly");
            }
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
