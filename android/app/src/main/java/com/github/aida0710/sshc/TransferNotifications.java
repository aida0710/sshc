package com.github.aida0710.sshc;

import android.app.Notification;
import android.app.NotificationManager;
import android.content.Context;

/** Web 画面を閉じていても転送の結果が分かる、内容を伏せた通知。 */
final class TransferNotifications {
    private static final String CHANNEL = "transfers";
    // 開く操作の requestCode。ほかの通知の操作と分ける。
    private static final int OPEN_REQUEST = 2;
    // 通知は置き換えずに並べる。エンジンの通知（1）とターミナルの通知（tag 付き）と重ならない番号から数える。
    private static int sequence = 100;

    private final Context context;

    TransferNotifications(Context context) {
        this.context = context;
    }

    /** status は Web 画面が送る転送の結果（completed か failed）。 */
    void show(String status) {
        NotificationManager manager = AppNotifications.channel(
                context, CHANNEL, context.getString(R.string.transfer_channel), NotificationManager.IMPORTANCE_DEFAULT);
        int message = "completed".equals(status) ? R.string.transfer_completed : R.string.transfer_failed;
        Notification notification = new Notification.Builder(context, CHANNEL)
                .setSmallIcon(android.R.drawable.stat_notify_sync)
                .setContentTitle(context.getString(R.string.app_name))
                .setContentText(context.getString(message))
                .setContentIntent(AppNotifications.openApp(context, OPEN_REQUEST, null))
                .setAutoCancel(true)
                .setCategory(Notification.CATEGORY_PROGRESS)
                .setVisibility(Notification.VISIBILITY_PRIVATE)
                .build();
        manager.notify(sequence++, notification);
    }
}
