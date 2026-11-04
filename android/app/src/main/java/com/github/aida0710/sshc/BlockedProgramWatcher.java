package com.github.aida0710.sshc;

import android.content.Context;
import android.util.Log;

import org.json.JSONException;
import org.json.JSONObject;

import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

import mobile.Mobile;

/**
 * プログラムが利用者を待っている（OSC 7501 の blocked）ことをエンジンから受け取り、通知にする。
 * アプリが裏にいる間は Web 画面の JavaScript が止まることがあるので、Web 画面を通さない。
 * 通知の tag は Web 画面の通知と同じなので、押すとそのセッションが開き、見れば取り下げられる。
 */
final class BlockedProgramWatcher {
    private static final String TAG = "sshc";

    private static boolean started;

    /**
     * いまの待ちを知らせたセッションの tag。裏にいる間はこのスレッドが、裏に回る瞬間は Web 画面が知らせる
     * （見えている間に始まった待ちは、Web 画面しか「見たか」を知らない）。同じ待ちで二度鳴らさない。
     */
    private static final Set<String> announcedTags = ConcurrentHashMap.newKeySet();

    private final Context context;
    private final TerminalNotifications notifications;

    private BlockedProgramWatcher(Context context) {
        this.context = context;
        this.notifications = new TerminalNotifications(context);
    }

    /** プロセスで 1 本だけ動かす。エンジンをまたいで同じスレッドが待つので、サービスが作り直されても増やさない。 */
    static synchronized void ensureStarted(Context context) {
        if (started) return;
        started = true;
        // サービスより長く生きるので、アプリの Context で持つ。
        BlockedProgramWatcher watcher = new BlockedProgramWatcher(context.getApplicationContext());
        Thread thread = new Thread(watcher::watch, "sshc-blocked-programs");
        thread.setDaemon(true);
        thread.start();
    }

    /** この待ちをまだ知らせていなければ、知らせたことにして true を返す。 */
    static boolean markAnnounced(String tag) {
        return announcedTags.add(tag);
    }

    static void forgetAnnounced(String tag) {
        announcedTags.remove(tag);
    }

    private void watch() {
        while (true) {
            try {
                apply(new JSONObject(Mobile.nextBlockedProgramChange()));
            } catch (JSONException error) {
                Log.e(TAG, "the engine sent a blocked program change that could not be read");
            }
        }
    }

    private void apply(JSONObject change) throws JSONException {
        String tag = TerminalNotifications.sessionTag(change.getString("sessionId"));
        if (!change.getBoolean("blocked")) {
            forgetAnnounced(tag);
            notifications.cancel(tag);
            return;
        }
        // 見えている画面にはサイドバーの印がある。Web 画面の通知と同じく、見ていない人にだけ知らせる。
        if (AppVisibility.isVisible() || !markAnnounced(tag)) return;
        notifications.showBlockedProgram(
                tag,
                NativeBridge.limitText(
                        headingOf(change.getString("sessionAlias"), change.getString("sessionTitle")),
                        NativeBridge.MAX_NOTIFICATION_TITLE),
                NativeBridge.limitText(
                        bodyOf(context.getString(labelOf(change.getString("kind"))), change.getString("said")),
                        NativeBridge.MAX_NOTIFICATION_BODY));
    }

    /**
     * Web 画面の通知（web の nextTerminalNotification）と同じ形にする。接続名を先頭に置く。セッションの
     * 名前は接続先のプログラムが変えられるので、接続名の無い見出しは sshc 自身の通知を装える。
     */
    static String headingOf(String alias, String sessionTitle) {
        return alias.isEmpty() || alias.equals(sessionTitle) ? sessionTitle : alias + "（" + sessionTitle + "）";
    }

    /** サイドバーのセッションの行（web の programStatusText）と同じ形にする。 */
    static String bodyOf(String label, String said) {
        return said.isEmpty() ? label : label + " · " + said;
    }

    private static int labelOf(String kind) {
        switch (kind) {
            case "permission":
                return R.string.blocked_program_permission;
            case "question":
                return R.string.blocked_program_question;
            case "auth":
                return R.string.blocked_program_auth;
            default:
                return R.string.blocked_program_waiting;
        }
    }
}
