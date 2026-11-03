package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;

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
    public void aProgramThatSaidNothingShowsTheStateAlone() {
        assertEquals("入力待ち", BlockedProgramWatcher.bodyOf("入力待ち", ""));
    }
}
