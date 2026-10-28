package com.github.aida0710.sshc;

import android.webkit.JavascriptInterface;

import java.io.IOException;
import java.io.OutputStream;
import java.util.Base64;

/** 信頼済みloopback UIとAndroid固有機能をつなぐ最小のbridge。 */
final class NativeBridge {
    interface Host {
        void chooseSaveDestination(String requestId, String suggestedName, String mimeType);
        void notifyTransfer(String status);
        /** barColour は 0xFFrrggbb。 */
        void setAppearance(String appearance, int barColour);
        /** "granted"、"denied"、"default" のいずれか。 */
        String notificationPermission();
        void requestNotificationPermission();
        void showNotification(String tag, String title, String body);
        /** tag の通知を取り下げる。Web 画面がそのセッションを見せたか、閉じたとき。 */
        void cancelNotification(String tag);
        /** 押されたまま Web 画面が受け取っていない通知の tag。無ければ空文字。 */
        String takeOpenedNotification();
        void chooseFolder(String requestId);
        /** 選んだフォルダの一覧（JSON）。無ければ空文字。 */
        String takeChosenFolder(String requestId);
        void releaseChosenFolder();
    }

    // 通知に載せる文字数の上限。プログラムが送る本文を際限なく Android へ渡さない。
    static final int MAX_NOTIFICATION_TITLE = 200;
    static final int MAX_NOTIFICATION_BODY = 2000;

    private final Host host;
    private String saveRequestId;
    private OutputStream saveOutput;

    NativeBridge(Host host) {
        this.host = host;
    }

    /** Androidのdocument pickerを開く。空文字は受付済み、それ以外は安定した失敗code。 */
    @JavascriptInterface
    public String chooseSave(String requestId, String suggestedName, String mimeType) {
        if (!validRequestId(requestId)) return "invalid_request";
        host.chooseSaveDestination(requestId, safeFileName(suggestedName), safeMimeType(mimeType));
        return "";
    }

    /** 選択済みdestinationへ1 chunkを書き込む。 */
    @JavascriptInterface
    public synchronized String writeSaveChunk(String requestId, String encoded) {
        if (!requestId.equals(saveRequestId) || saveOutput == null) return "save_not_ready";
        try {
            saveOutput.write(Base64.getDecoder().decode(encoded));
            return "";
        } catch (IllegalArgumentException error) {
            return "invalid_chunk";
        } catch (IOException error) {
            closeSave();
            return "save_write_failed";
        }
    }

    /** 保存を完了し、document providerへflushする。 */
    @JavascriptInterface
    public synchronized String finishSave(String requestId) {
        if (!requestId.equals(saveRequestId) || saveOutput == null) return "save_not_ready";
        try {
            saveOutput.close();
            saveOutput = null;
            saveRequestId = null;
            return "";
        } catch (IOException error) {
            closeSave();
            return "save_finish_failed";
        }
    }

    @JavascriptInterface
    public synchronized void cancelSave(String requestId) {
        if (requestId.equals(saveRequestId)) closeSave();
    }

    /** Web画面を離れていても分かる転送結果をAndroid通知へ渡す。 */
    @JavascriptInterface
    public void notifyTransfer(String status) {
        if ("completed".equals(status) || "failed".equals(status)) host.notifyTransfer(status);
    }

    /**
     * Web UIとsystem barの明暗と色を一致させる。barColour はWebの上のバーの色（#rrggbb）。
     * テーマごとに違うので、Android側に色の表を持たずWebから受け取る。
     */
    @JavascriptInterface
    public void setAppearance(String appearance, String barColour) {
        if (!"light".equals(appearance) && !"dark".equals(appearance)) return;
        if (barColour == null || !barColour.matches("#[0-9a-fA-F]{6}")) return;
        host.setAppearance(appearance, 0xFF000000 | Integer.parseInt(barColour.substring(1), 16));
    }

    /** Android の通知の許可の状態。ブラウザの Notification.permission と同じ 3 つの値を返す。 */
    @JavascriptInterface
    public String notificationPermission() {
        return host.notificationPermission();
    }

    /**
     * 押された通知の tag を 1 度だけ返す。Web 画面は起動したときと
     * sshc-android-notification-opened イベントを受けたときに呼ぶ。
     * 画面を読み込み直しても、アプリを開き直しても、押された通知を取りこぼさない。
     */
    @JavascriptInterface
    public String takeOpenedNotification() {
        return host.takeOpenedNotification();
    }

    /**
     * Android のフォルダの選択画面を開く。空文字は受付済み、それ以外は安定した失敗 code。
     * 結果は sshc-android-folder イベントで知らせ、一覧は takeChosenFolder で受け取る。
     */
    @JavascriptInterface
    public String chooseFolder(String requestId) {
        if (!validRequestId(requestId)) return "invalid_request";
        host.chooseFolder(requestId);
        return "";
    }

    @JavascriptInterface
    public String takeChosenFolder(String requestId) {
        if (!validRequestId(requestId)) return "";
        return host.takeChosenFolder(requestId);
    }

    /** 選んだフォルダのファイルを読み終えたら、渡すのをやめる。 */
    @JavascriptInterface
    public void releaseChosenFolder() {
        host.releaseChosenFolder();
    }

    /** 通知の許可を求める。結果は sshc-android-notification-permission イベントで Web 画面へ返る。 */
    @JavascriptInterface
    public void requestNotificationPermission() {
        host.requestNotificationPermission();
    }

    /** 空文字は表示した、それ以外は安定した失敗 code。同じ tag の通知は置き換わる。 */
    @JavascriptInterface
    public String showNotification(String tag, String title, String body) {
        if (!validNotificationTag(tag)) return "invalid_notification";
        if (!"granted".equals(host.notificationPermission())) return "notifications_blocked";
        host.showNotification(tag, limitText(title, MAX_NOTIFICATION_TITLE), limitText(body, MAX_NOTIFICATION_BODY));
        return "";
    }

    /** tag の通知を取り下げる。出していなければ何も起きない。 */
    @JavascriptInterface
    public void cancelNotification(String tag) {
        if (!validNotificationTag(tag)) return;
        host.cancelNotification(tag);
    }

    synchronized boolean openSave(String requestId, OutputStream output) {
        if (!validRequestId(requestId) || output == null || saveOutput != null) return false;
        saveRequestId = requestId;
        saveOutput = output;
        return true;
    }

    synchronized void close() {
        closeSave();
    }

    private void closeSave() {
        if (saveOutput != null) {
            try {
                saveOutput.close();
            } catch (IOException ignored) {
            }
        }
        saveOutput = null;
        saveRequestId = null;
    }

    static boolean validRequestId(String value) {
        return value != null && value.matches("[A-Za-z0-9_-]{8,96}");
    }

    static boolean validNotificationTag(String value) {
        return value != null && value.matches("[A-Za-z0-9_-]{1,128}");
    }

    static String limitText(String value, int limit) {
        if (value == null) return "";
        String visible = value.replaceAll("[\\p{Cntrl}&&[^\n]]", " ").trim();
        return visible.length() <= limit ? visible : visible.substring(0, limit);
    }

    static String safeFileName(String value) {
        if (value == null) return "download";
        String safe = value.replaceAll("[\\\\/\\p{Cntrl}]", "_").trim();
        if (safe.isEmpty()) return "download";
        return safe.length() <= 160 ? safe : safe.substring(0, 160);
    }

    static String safeMimeType(String value) {
        if (value != null && value.matches("[A-Za-z0-9!#$&^_.+-]+/[A-Za-z0-9!#$&^_.+-]+")) return value;
        return "application/octet-stream";
    }
}
