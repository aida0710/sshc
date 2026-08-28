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
        public void setAppearance(String appearance) {
            this.appearance = appearance;
        }
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
        bridge.setAppearance("sepia");
        assertEquals(null, host.appearance);
        bridge.setAppearance("dark");
        assertEquals("dark", host.appearance);
    }
}
