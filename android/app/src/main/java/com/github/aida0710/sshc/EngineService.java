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

/** Activity の状態と独立して engine を維持する foreground service。 */
public final class EngineService extends Service {
    private static final String TAG = "sshc";
    private static final String CHANNEL = "engine";
    private static final int NOTIFICATION_ID = 1;

    /** foreground 通知の停止操作を識別する action。 */
    static final String ACTION_STOP = "com.github.aida0710.sshc.STOP";

    /** engine の停止を Activity へ通知する。 */
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
     * engine の接続 URL を返す。engine が停止中なら null。
     * 2 回目以降は {@link Entrance} の規則に従って fragment を除く。
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

    /** Activity の生存中だけ停止通知先を保持する。 */
    void listen(Listener listener) {
        this.listener = listener;
    }

    @Override
    public void onCreate() {
        super.onCreate();
        // startForegroundService の制限時間内に通知を開始してから engine を起動する。
        startForeground(NOTIFICATION_ID, notification());

        try {
            entrance = Mobile.start(getFilesDir().getAbsolutePath(), getCacheDir().getAbsolutePath());
            started = true;
        } catch (Exception error) {
            // URL を含みうる error message は保持せず、失敗種別だけを記録する。
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
            // Activity が bind 中でも確実に停止するよう、onDestroy より先に shutdown する。
            shutdown();
            if (listener != null) listener.engineStopped();
            stopForeground(STOP_FOREGROUND_REMOVE);
            stopSelf();
            // 明示的な停止後は service を再作成しない。
            return START_NOT_STICKY;
        }
        // OS による終了後は service を再作成する。起動引数がないため intent は不要。
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

    /** Activity を開く操作、engine の停止操作、実行中バージョンを含む通知を作る。 */
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
