package com.github.aida0710.sshc;

import android.app.Notification;
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
    // 通知はプロセスに属し、出した MainActivity が閉じたあとに押されることもある。Activity
    // ごとではなくプロセスで 1 つ持つ。
    private static final IssuedNotificationTags issued = new IssuedNotificationTags();

    private final Context context;

    TerminalNotifications(Context context) {
        this.context = context;
    }

    /** tag の通知をこのアプリが出していれば、覚えた tag から外して true を返す。 */
    static boolean takeIssued(String tag) {
        return issued.take(tag);
    }

    void show(String tag, String title, String body) {
        // 音は Web 画面が設定の「通知音」で鳴らす。
        NotificationManager manager = AppNotifications.silentChannel(
                context, CHANNEL, context.getString(R.string.terminal_channel), NotificationManager.IMPORTANCE_DEFAULT);
        // request code を tag ごとに変え、別の通知の extra で上書きされないようにする。
        PendingIntent content = AppNotifications.openApp(context, tag.hashCode(), new Intent().putExtra(EXTRA_TAG, tag));
        Notification notification = new Notification.Builder(context, CHANNEL)
                .setSmallIcon(android.R.drawable.stat_notify_chat)
                // 見出しと本文は接続先のプログラムが決める。sshc 自身の通知と見分けられるよう、
                // ターミナルのプログラムからの通知であることをアプリ名の横に出す。
                .setSubText(context.getString(R.string.terminal_notification_source))
                .setContentTitle(title)
                .setContentText(body)
                .setStyle(new Notification.BigTextStyle().bigText(body))
                .setContentIntent(content)
                .setAutoCancel(true)
                .setCategory(Notification.CATEGORY_MESSAGE)
                // プログラムが送った本文はロック画面に出さない。
                .setVisibility(Notification.VISIBILITY_PRIVATE)
                .build();
        issued.issue(tag);
        manager.notify(tag, NOTIFICATION_ID, notification);
    }

    /** tag の通知を取り下げる。取り下げた通知は押されないので、覚えた tag からも外す。 */
    void cancel(String tag) {
        context.getSystemService(NotificationManager.class).cancel(tag, NOTIFICATION_ID);
        issued.take(tag);
    }
}
