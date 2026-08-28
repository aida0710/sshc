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
        void setAppearance(String appearance);
    }

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

    /** Web UIとsystem barの明暗を一致させる。 */
    @JavascriptInterface
    public void setAppearance(String appearance) {
        if ("light".equals(appearance) || "dark".equals(appearance)) host.setAppearance(appearance);
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
