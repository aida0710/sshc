package com.github.aida0710.sshc;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.graphics.drawable.Icon;
import android.os.Binder;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.util.Log;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import mobile.Mobile;

/** 利用者が開始したtaskの生存中にengineを維持するforeground service。 */
public final class EngineService extends Service {
    private static final String TAG = "sshc";
    private static final String CHANNEL = "engine";
    private static final int NOTIFICATION_ID = 1;
    // Mobile.Start and Mobile.Stop share a Go process-global lifecycle. A single
    // worker serializes them across successive Service instances, so reopening
    // while the previous stop is still draining never blocks Android's main looper.
    private static final ExecutorService ENGINE = Executors.newSingleThreadExecutor(command -> {
        Thread worker = new Thread(command, "sshc-engine-lifecycle");
        worker.setDaemon(true);
        return worker;
    });

    /** foreground 通知の停止操作を識別する action。 */
    static final String ACTION_STOP = "com.github.aida0710.sshc.STOP";

    /** engine の停止を Activity へ通知する。 */
    interface Listener {
        void engineReady();
        void engineStopped();
    }

    private final LocalBinder binder = new LocalBinder();
    private String entrance;
    private boolean started;
    private volatile boolean stopping;
    private long failure;
    private Listener listener;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final EngineShutdown shutdown = new EngineShutdown(ENGINE, this::stopEngine);

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

        ENGINE.execute(this::startEngine);
    }

    @Override
    public void onDestroy() {
        requestEngineShutdown();
        super.onDestroy();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && ACTION_STOP.equals(intent.getAction())) {
            stopServiceAndEngine(null);
            // 明示的な停止後は service を再作成しない。
            return START_NOT_STICKY;
        }
        // dataSync foreground serviceをOS終了後に常駐再開しない。Activityからの明示的な
        // 起動だけを受理し、taskを閉じた場合はmanifestのstopWithTaskで終了する。
        return START_NOT_STICKY;
    }

    /** Android 15以降のdataSync時間上限に達したら、ANRになる前にengineもserviceも止める。 */
    @Override
    public void onTimeout(int startId, int foregroundServiceType) {
        Log.w(TAG, "the foreground engine reached the dataSync time limit");
        stopServiceAndEngine(startId);
    }

    @Override
    public IBinder onBind(Intent intent) {
        return binder;
    }

    /** foreground と service を main looper 上で即時に外し、Go の待機だけを worker へ送る。 */
    private void stopServiceAndEngine(Integer startId) {
        stopping = true;
        boolean shouldStopEngine = detachEngine();
        stopForeground(STOP_FOREGROUND_REMOVE);
        if (startId == null) {
            stopSelf();
        } else {
            stopSelf(startId);
        }
        if (listener != null) listener.engineStopped();
        if (shouldStopEngine) shutdown.request();
    }

    private void requestEngineShutdown() {
        stopping = true;
        if (detachEngine()) shutdown.request();
    }

    private boolean detachEngine() {
        if (!started) return false;
        started = false;
        entrance = null;
        return true;
    }

    private void stopEngine() {
        try {
            Mobile.stop();
        } catch (Exception error) {
            Log.e(TAG, "the engine did not stop cleanly");
        }
    }

    private void startEngine() {
        if (stopping) return;
        try {
            String startedEntrance = Mobile.start(getFilesDir().getAbsolutePath(), getCacheDir().getAbsolutePath());
            if (stopping) {
                // Stop arrived while Mobile.start was running. Finish the same
                // serialized lifecycle task before a newer Service may start again.
                stopEngine();
                return;
            }
            main.post(() -> {
                if (stopping) {
                    shutdown.request();
                    return;
                }
                entrance = startedEntrance;
                started = true;
                failure = Mobile.KindNone;
                if (listener != null) listener.engineReady();
            });
        } catch (Exception error) {
            long reason = Mobile.lastStartFailureKind();
            main.post(() -> {
                if (stopping) return;
                failure = reason;
                Log.e(TAG, "the engine did not start; reason " + failure);
                if (listener != null) listener.engineReady();
                stopForeground(STOP_FOREGROUND_REMOVE);
                stopSelf();
            });
        }
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
