package com.github.aida0710.sshc;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public final class EngineLeaseTest {
    @Test
    public void stopBeforeStartCompletionMakesTheWorkerStopItsOwnEngine() {
        EngineLease lease = new EngineLease();
        assertFalse(lease.requestStop());
        assertFalse(lease.publishStartedEngine());
        assertTrue(lease.isStopping());
    }

    @Test
    public void stopAfterStartCompletionOwnsExactlyOneStop() {
        EngineLease lease = new EngineLease();
        assertTrue(lease.publishStartedEngine());
        assertTrue(lease.requestStop());
        assertFalse(lease.requestStop());
    }
}
