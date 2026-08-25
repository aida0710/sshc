package com.github.aida0710.sshc;

import java.util.concurrent.Executor;

/** Go engine の待機を Android の lifecycle callback から分離する。 */
final class EngineShutdown {
    interface Stop {
        void run();
    }

    private final Executor executor;
    private final Stop stop;
    private boolean requested;

    EngineShutdown(Executor executor, Stop stop) {
        this.executor = executor;
        this.stop = stop;
    }

    /** 停止を一度だけ非同期に送り、呼び出し元では Go の終了を待たない。 */
    synchronized void request() {
        if (requested) return;
        requested = true;
        executor.execute(stop::run);
    }
}
