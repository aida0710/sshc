package com.github.aida0710.sshc;

import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;

/** 通知のチャンネルと、通知から MainActivity を開く操作を、どの通知も同じ形で作る。 */
final class AppNotifications {
    private AppNotifications() {}

    /** チャンネルを作り（あれば何もしない）、通知を出す NotificationManager を返す。 */
    static NotificationManager channel(Context context, String id, CharSequence name, int importance) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        manager.createNotificationChannel(new NotificationChannel(id, name, importance));
        return manager;
    }

    /**
     * 音の無いチャンネルを作る。音を Web 画面が鳴らす通知に使う。WebView は裏でも音を出せるため、
     * チャンネルでも鳴らすと二重になる。
     */
    static NotificationManager silentChannel(Context context, String id, CharSequence name, int importance) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        NotificationChannel channel = new NotificationChannel(id, name, importance);
        channel.setSound(null, null);
        manager.createNotificationChannel(channel);
        return manager;
    }

    /**
     * 通知を押したとき、動いている MainActivity を前へ出す（無ければ開く）操作。requestCode が
     * 違えば、extra の違う操作として別に持つ。
     */
    static PendingIntent openApp(Context context, int requestCode, Intent extras) {
        Intent open = new Intent(context, MainActivity.class)
                .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        if (extras != null) open.putExtras(extras);
        return PendingIntent.getActivity(
                context, requestCode, open, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }
}
