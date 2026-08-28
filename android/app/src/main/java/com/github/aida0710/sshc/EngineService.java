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
    static final String RUNTIME_PREFERENCES = "runtime_notices";
    static final String LAST_STOP_REASON = "last_stop_reason";
    static final String STOP_REASON_DATA_SYNC_TIMEOUT = "data_sync_timeout";
    // Mobile.StartとMobile.StopはGo process-global lifecycleを共有する。一つのworkerで
    // Service世代をまたいで直列化し、前世代の停止中に開き直してもmain looperを塞がない。
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
    private long failure;
    private String failureCode = "none";
    private String failureDetail = "";
    private Listener listener;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final EngineShutdown shutdown = new EngineShutdown(ENGINE, this::stopEngine);
    private final EngineLease lease = new EngineLease();

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

    /** 利用者がissueへ貼れる安定した失敗code。 */
    String failureCode() {
        return failureCode;
    }

    /** Go側でcredentialを伏せ字化し、長さを制限した診断文。 */
    String failureDetail() {
        return failureDetail;
    }

    /** 失敗した同じService世代でengineをもう一度起動する。 */
    boolean retry() {
        if (failure == Mobile.KindNone || lease.isStopping()) return false;
        failure = Mobile.KindNone;
        failureCode = "none";
        failureDetail = "";
        // 失敗時に外したforegroundを、次のGo呼び出しより先に復帰させる。
        startForeground(NOTIFICATION_ID, notification());
        ENGINE.execute(this::startEngine);
        return true;
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
        getSharedPreferences(RUNTIME_PREFERENCES, MODE_PRIVATE).edit()
                .putString(LAST_STOP_REASON, STOP_REASON_DATA_SYNC_TIMEOUT)
                .apply();
        stopServiceAndEngine(startId);
    }

    @Override
    public IBinder onBind(Intent intent) {
        return binder;
    }

    /** foreground と service を main looper 上で即時に外し、Go の待機だけを worker へ送る。 */
    private void stopServiceAndEngine(Integer startId) {
        boolean shouldStopEngine = lease.requestStop();
        entrance = null;
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
        entrance = null;
        if (lease.requestStop()) shutdown.request();
    }

    private void stopEngine() {
        try {
            Mobile.stop();
        } catch (Exception error) {
            Log.e(TAG, "the engine did not stop cleanly");
        }
    }

    private void startEngine() {
        if (lease.isStopping()) return;
        try {
            String startedEntrance = Mobile.start(getFilesDir().getAbsolutePath(), getCacheDir().getAbsolutePath());
            if (!lease.publishStartedEngine()) {
                // Mobile.start中に停止要求が来た。次のService世代を起動する前に、
                // 同じlifecycle worker上で停止まで完了する。
                stopEngine();
                return;
            }
            main.post(() -> {
                if (lease.isStopping()) return;
                entrance = startedEntrance;
                failure = Mobile.KindNone;
                failureCode = "none";
                failureDetail = "";
                if (listener != null) listener.engineReady();
            });
        } catch (Exception error) {
            long reason = Mobile.lastStartFailureKind();
            String code = Mobile.lastStartFailureCode();
            String detail = Mobile.lastStartFailureDetail();
            main.post(() -> {
                if (lease.isStopping()) return;
                failure = reason;
                failureCode = code;
                failureDetail = detail;
                Log.e(TAG, "engine start failed; code=" + failureCode
                        + "; detail=" + failureDetail);
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
