package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public final class BlockedProgramWatcherTest {
    @Test
    public void theHeadingLeadsWithTheConnectionAliasWhichAProgramCannotChange() {
        assertEquals("web-01（Claude Code）", BlockedProgramWatcher.headingOf("web-01", "Claude Code"));
    }

    @Test
    public void theHeadingSaysANameOnce() {
        assertEquals("web-01", BlockedProgramWatcher.headingOf("web-01", "web-01"));
        assertEquals("bash", BlockedProgramWatcher.headingOf("", "bash"));
    }

    @Test
    public void theBodyIsTheStateThenWhatTheProgramSaid() {
        assertEquals("許可待ち · approve Bash", BlockedProgramWatcher.bodyOf("許可待ち", "approve Bash"));
    }

    @Test
    public void oneWaitIsAnnouncedOnceUntilItIsForgotten() {
        String tag = "sshc-terminal-announced-once";
        assertTrue(BlockedProgramWatcher.markAnnounced(tag));
        assertFalse(BlockedProgramWatcher.markAnnounced(tag));
        BlockedProgramWatcher.forgetAnnounced(tag);
        assertTrue(BlockedProgramWatcher.markAnnounced(tag));
        BlockedProgramWatcher.forgetAnnounced(tag);
    }

    @Test
    public void aProgramThatSaidNothingShowsTheStateAlone() {
        assertEquals("入力待ち", BlockedProgramWatcher.bodyOf("入力待ち", ""));
    }
}
