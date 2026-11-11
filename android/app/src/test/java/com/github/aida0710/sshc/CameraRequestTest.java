package com.github.aida0710.sshc;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public class CameraRequestTest {
    @Test
    public void aRequestForTheCameraAloneIsForVideoOnly() {
        assertTrue(CameraRequest.asksOnlyForVideo(new String[]{"android.webkit.resource.VIDEO_CAPTURE"}));
    }

    @Test
    public void aRequestThatAlsoAsksForTheMicrophoneIsNotForVideoOnly() {
        assertFalse(CameraRequest.asksOnlyForVideo(new String[]{
                "android.webkit.resource.VIDEO_CAPTURE", "android.webkit.resource.AUDIO_CAPTURE"}));
    }

    @Test
    public void aRequestForAnythingElseIsNotForVideoOnly() {
        assertFalse(CameraRequest.asksOnlyForVideo(new String[]{"android.webkit.resource.AUDIO_CAPTURE"}));
        assertFalse(CameraRequest.asksOnlyForVideo(new String[]{}));
        assertFalse(CameraRequest.asksOnlyForVideo(null));
    }
}
