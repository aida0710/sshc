package com.github.aida0710.sshc;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;

/**
 * ターミナルのプログラムが求めた通知を Android の通知として出す。WebView にはブラウザの
 * Notification が無いため、Web 画面はブラウザ通知の代わりにこれを使う。
 */
final class TerminalNotifications {
    /** 通知を押して開いた MainActivity が、どの通知から来たかを Web 画面へ伝えるための extra。 */
    static final String EXTRA_TAG = "com.github.aida0710.sshc.NOTIFICATION_TAG";
    private static final String CHANNEL = "terminal";
    // 同じ tag の通知は置き換える。tag ごとに 1 件なので id は固定でよい。
    private static final int NOTIFICATION_ID = 1;

    private final Context context;

    TerminalNotifications(Context context) {
        this.context = context;
    }

    void show(String tag, String title, String body) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        NotificationChannel channel = new NotificationChannel(
                CHANNEL, context.getString(R.string.terminal_channel), NotificationManager.IMPORTANCE_DEFAULT);
        // 音は Web 画面が設定の「通知音」で鳴らす。WebView は裏でも音を出せるため、ここでも鳴らすと二重になる。
        channel.setSound(null, null);
        manager.createNotificationChannel(channel);
        Intent open = new Intent(context, MainActivity.class)
                .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP)
                .putExtra(EXTRA_TAG, tag);
        // request code を tag ごとに変え、別の通知の extra で上書きされないようにする。
        PendingIntent content = PendingIntent.getActivity(
                context, tag.hashCode(), open, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
        Notification notification = new Notification.Builder(context, CHANNEL)
                .setSmallIcon(android.R.drawable.stat_notify_chat)
                .setContentTitle(title)
                .setContentText(body)
                .setStyle(new Notification.BigTextStyle().bigText(body))
                .setContentIntent(content)
                .setAutoCancel(true)
                .setCategory(Notification.CATEGORY_MESSAGE)
                // プログラムが送った本文はロック画面に出さない。
                .setVisibility(Notification.VISIBILITY_PRIVATE)
                .build();
        manager.notify(tag, NOTIFICATION_ID, notification);
    }
}
