package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;

import org.junit.Test;

public final class FailureReportTest {
    @Test
    public void 共有用の版とcodeと詳細を整形する() {
        assertEquals(
                "Version: 0.13.3\nCode: port_unavailable\nDetail: listen: permission denied"
                        + "\nAndroid: 16 (SDK 36)\nDevice: Google Pixel 9\nABI: arm64-v8a",
                FailureReport.render("0.13.3", "port_unavailable", "listen: permission denied",
                        "16", 36, "Google", "Pixel 9", "arm64-v8a"));
    }

    @Test
    public void 欠けた値を明示する() {
        assertEquals(
                "Version: Unavailable\nCode: Unavailable\nDetail: Unavailable"
                        + "\nAndroid: Unavailable (SDK 0)\nDevice: Unavailable\nABI: Unavailable",
                FailureReport.render(null, "", "  ", null, 0, "", "", null));
    }
}
