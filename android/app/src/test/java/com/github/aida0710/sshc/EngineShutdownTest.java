package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;

import java.util.ArrayList;
import java.util.List;
import org.junit.Test;

/** lifecycle callback が Go の停止完了を待たないことを検証する。 */
public final class EngineShutdownTest {
    @Test
    public void requestは停止を呼び出し元で実行しない() {
        List<Runnable> queued = new ArrayList<>();
        int[] stops = {0};
        EngineShutdown shutdown = new EngineShutdown(queued::add, () -> stops[0]++);

        shutdown.request();

        assertEquals(0, stops[0]);
        assertEquals(1, queued.size());
        queued.get(0).run();
        assertEquals(1, stops[0]);
    }

    @Test
    public void 複数のlifecycle経路でも停止は一度だけ送る() {
        List<Runnable> queued = new ArrayList<>();
        EngineShutdown shutdown = new EngineShutdown(queued::add, () -> {});

        shutdown.request();
        shutdown.request();

        assertEquals(1, queued.size());
    }
}
