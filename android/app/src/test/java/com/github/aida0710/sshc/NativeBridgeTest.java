package com.github.aida0710.sshc;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import java.io.ByteArrayOutputStream;
import java.nio.charset.StandardCharsets;

import org.junit.Test;

public final class NativeBridgeTest {
    private static final class Host implements NativeBridge.Host {
        String requestId;
        String name;
        String mime;
        String status;
        String appearance;
        int barColour;

        @Override
        public void chooseSaveDestination(String requestId, String suggestedName, String mimeType) {
            this.requestId = requestId;
            this.name = suggestedName;
            this.mime = mimeType;
        }

        @Override
        public void notifyTransfer(String status) {
            this.status = status;
        }

        @Override
        public void setAppearance(String appearance, int barColour) {
            this.appearance = appearance;
            this.barColour = barColour;
        }

        String permission = "granted";
        String notified;

        @Override
        public String notificationPermission() {
            return permission;
        }

        @Override
        public void requestNotificationPermission() {
        }

        @Override
        public void showNotification(String tag, String title, String body) {
            notified = tag + "|" + title + "|" + body;
        }

        String cancelled;

        @Override
        public void cancelNotification(String tag) {
            cancelled = tag;
        }

        String opened = "";

        @Override
        public String takeOpenedNotification() {
            String tag = opened;
            opened = "";
            return tag;
        }

        String folderRequest;

        @Override
        public void chooseFolder(String requestId) {
            folderRequest = requestId;
        }

        @Override
        public String takeChosenFolder(String requestId) {
            return requestId.equals(folderRequest) ? "{}" : "";
        }

        @Override
        public void releaseChosenFolder() {
        }
    }

    @Test
    public void 押された通知の_tag_を渡す() {
        Host host = new Host();
        host.opened = "sshc-terminal-1";
        NativeBridge bridge = new NativeBridge(host);
        assertEquals("sshc-terminal-1", bridge.takeOpenedNotification());
        assertEquals("", bridge.takeOpenedNotification());
    }

    @Test
    public void フォルダの選択は正しい_request_ID_だけを受け付ける() {
        Host host = new Host();
        NativeBridge bridge = new NativeBridge(host);
        assertEquals("invalid_request", bridge.chooseFolder("../x"));
        assertEquals("", bridge.chooseFolder("folder-request-1"));
        assertEquals("{}", bridge.takeChosenFolder("folder-request-1"));
        assertEquals("", bridge.takeChosenFolder("../x"));
    }

    @Test
    public void 許可されていれば通知を渡す() {
        Host host = new Host();
        assertEquals("", new NativeBridge(host).showNotification("sshc-terminal-1", "build", "done\nok"));
        assertEquals("sshc-terminal-1|build|done\nok", host.notified);
    }

    @Test
    public void 許可されていなければ通知を渡さない() {
        Host host = new Host();
        host.permission = "default";
        assertEquals("notifications_blocked", new NativeBridge(host).showNotification("sshc-terminal-1", "build", "done"));
        assertEquals(null, host.notified);
    }

    @Test
    public void 通知を取り下げる_不正な_tag_は渡さない() {
        Host host = new Host();
        new NativeBridge(host).cancelNotification("../x");
        assertEquals(null, host.cancelled);
        new NativeBridge(host).cancelNotification("sshc-terminal-1");
        assertEquals("sshc-terminal-1", host.cancelled);
    }

    @Test
    public void 不正な_tag_の通知を断る() {
        Host host = new Host();
        assertEquals("invalid_notification", new NativeBridge(host).showNotification("../x", "build", "done"));
        assertEquals("invalid_notification", new NativeBridge(host).showNotification(null, "build", "done"));
        assertEquals(null, host.notified);
    }

    @Test
    public void 通知の文字は制御文字を除き上限で切る() {
        assertEquals("a b\nc", NativeBridge.limitText("a\u0007b\nc", 10));
        assertEquals("abc", NativeBridge.limitText("abcdef", 3));
        assertEquals("", NativeBridge.limitText(null, 3));
    }

    @Test
    public void 選択した保存先へbase64のchunkを順に書き込む() {
        Host host = new Host();
        NativeBridge bridge = new NativeBridge(host);
        String id = "save_request_123";
        assertEquals("", bridge.chooseSave(id, "report.txt", "text/plain"));
        assertEquals(id, host.requestId);

        ByteArrayOutputStream output = new ByteArrayOutputStream();
        assertTrue(bridge.openSave(id, output));
        assertEquals("", bridge.writeSaveChunk(id, "c3NoYw=="));
        assertEquals("", bridge.finishSave(id));
        assertArrayEquals("sshc".getBytes(StandardCharsets.UTF_8), output.toByteArray());
        assertEquals("save_not_ready", bridge.writeSaveChunk(id, "YQ=="));
    }

    @Test
    public void bridgeへ渡す名前とmimeと通知状態を制限する() {
        Host host = new Host();
        NativeBridge bridge = new NativeBridge(host);
        assertEquals("", bridge.chooseSave("safe_request_99", "../bad\nname", "invalid mime"));
        assertFalse(host.name.contains("/"));
        assertFalse(host.name.contains("\n"));
        assertEquals("application/octet-stream", host.mime);

        bridge.notifyTransfer("running");
        assertEquals(null, host.status);
        bridge.notifyTransfer("failed");
        assertEquals("failed", host.status);
        bridge.setAppearance("sepia", "#000000");
        assertEquals(null, host.appearance);
        bridge.setAppearance("dark", "black");
        assertEquals(null, host.appearance);
        bridge.setAppearance("dark", "#0A0a0b");
        assertEquals("dark", host.appearance);
        assertEquals(0xFF0A0A0B, host.barColour);
    }
}
