package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;

import androidx.activity.BackEventCompat;

import org.junit.Test;

public final class PageBackCallbackTest {
    @Test
    public void tellsThePageThePhaseHowFarTheFingerWentAndFromWhichEdge() {
        assertEquals(
                "window.dispatchEvent(new CustomEvent('sshc-android-back-gesture',{detail:{phase:'progress',progress:0.4200,edge:'right'}}))",
                PageBackCallback.gestureScript("progress", 0.42f, BackEventCompat.EDGE_RIGHT));
        assertEquals(
                "window.dispatchEvent(new CustomEvent('sshc-android-back-gesture',{detail:{phase:'start',progress:0.0000,edge:'left'}}))",
                PageBackCallback.gestureScript("start", 0f, BackEventCompat.EDGE_LEFT));
    }
}
