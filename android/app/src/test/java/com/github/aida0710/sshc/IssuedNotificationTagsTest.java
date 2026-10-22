package com.github.aida0710.sshc;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public final class IssuedNotificationTagsTest {
    @Test
    public void aTagThisAppDidNotIssueIsNotTaken() {
        IssuedNotificationTags tags = new IssuedNotificationTags();
        tags.issue("program-one");

        assertFalse(tags.take("program-two"));
    }

    @Test
    public void anIssuedTagIsTakenOnceUntilItIsIssuedAgain() {
        IssuedNotificationTags tags = new IssuedNotificationTags();
        tags.issue("program-one");

        assertTrue(tags.take("program-one"));
        assertFalse(tags.take("program-one"));
        tags.issue("program-one");
        assertTrue(tags.take("program-one"));
    }
}
