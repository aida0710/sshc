package com.github.aida0710.sshc;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.graphics.drawable.Icon;
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

    /**
     * 通知の「停止」から届く。
     *
     * <p><b>これが無かった間、engine を畳む道は一本も無かった。</b> Activity は
     * 意図的に engine を止めず、通知には押せるものが何も無く、
     * <code>START_STICKY</code> なので OS がプロセスを殺しても作り直された。
     * 利用者に残っていたのは「アプリのデータを消す」だけだった。
     */
    static final String ACTION_STOP = "com.github.aida0710.sshc.STOP";

    /** engine が畳まれたことを画面へ知らせる。 */
    interface Listener {
        void engineStopped();
    }

    private final LocalBinder binder = new LocalBinder();
    private String entrance;
    private boolean started;
    private long failure;
    private Listener listener;

    public final class LocalBinder extends Binder {
        EngineService service() {
            return EngineService.this;
        }
    }

    /**
     * 入口の URL。engine が起きていなければ null。
     *
     * <p>2 回目からは fragment を落として渡す。理由は {@link Entrance} にある。
     */
    String entrance() {
        if (entrance == null) return null;
        String url = entrance;
        entrance = Entrance.withoutFragment(entrance);
        return url;
    }

    /** 直前の起動が失敗した理由。成功していれば {@link Mobile#KindNone}。 */
    long failure() {
        return failure;
    }

    /** 画面が居る間だけ、畳んだことを知らせる先を持つ。 */
    void listen(Listener listener) {
        this.listener = listener;
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
            failure = Mobile.lastStartFailureKind();
            Log.e(TAG, "the engine did not start; reason " + failure);
        }
    }

    @Override
    public void onDestroy() {
        shutdown();
        super.onDestroy();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && ACTION_STOP.equals(intent.getAction())) {
            // **ここで engine を止める。onDestroy を当てにしない。** 画面が
            // bind したままなら stopSelf() では destroy されないので、
            // onDestroy に任せると engine は走り続ける。
            shutdown();
            if (listener != null) listener.engineStopped();
            stopForeground(STOP_FOREGROUND_REMOVE);
            stopSelf();
            // **作り直させない。** START_STICKY は「OS に殺されたら戻ってくる」
            // という意味であり、頼まれて畳んだ後にそれをやられては困る。
            return START_NOT_STICKY;
        }
        // 死んだら作り直す。ただし intent は配り直さない——起動に引数は無い。
        return START_STICKY;
    }

    @Override
    public IBinder onBind(Intent intent) {
        return binder;
    }

    private void shutdown() {
        if (!started) return;
        try {
            Mobile.stop();
        } catch (Exception error) {
            Log.e(TAG, "the engine did not stop cleanly");
        }
        started = false;
        entrance = null;
    }

    /**
     * 常駐であることを言う通知。
     *
     * <p><b>押せるものを 2 つ置く。</b> 本体を押せば画面へ戻り、「停止」を押せば
     * engine が畳まれる。どちらも無かった頃、この通知は消せないだけの帯だった。
     *
     * <p>版を出すのは、APK を手で入れる配り方だからである。**どの engine が
     * 走っているのかは、通知を見なければ分からない。**
     */
    private Notification notification() {
        NotificationManager manager = getSystemService(NotificationManager.class);
        manager.createNotificationChannel(
                new NotificationChannel(CHANNEL, "sshc", NotificationManager.IMPORTANCE_LOW));

        PendingIntent open = PendingIntent.getActivity(this, 0,
                new Intent(this, MainActivity.class),
                PendingIntent.FLAG_IMMUTABLE);
        PendingIntent stop = PendingIntent.getService(this, 1,
                new Intent(this, EngineService.class).setAction(ACTION_STOP),
                PendingIntent.FLAG_IMMUTABLE);

        return new Notification.Builder(this, CHANNEL)
                .setContentTitle(getString(R.string.engine_running))
                .setContentText(Mobile.version())
                .setSmallIcon(android.R.drawable.stat_notify_sync)
                .setOngoing(true)
                .setContentIntent(open)
                .addAction(new Notification.Action.Builder(
                        Icon.createWithResource(this, android.R.drawable.ic_menu_close_clear_cancel),
                        getString(R.string.engine_stop), stop).build())
                .build();
    }
}
