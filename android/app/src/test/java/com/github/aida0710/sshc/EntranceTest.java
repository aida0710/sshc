package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import org.junit.Test;

/** 一度だけ使える bootstrap fragment の除去を検証する。 */
public final class EntranceTest {
    @Test
    public void 二度目は_fragment_を落とす() {
        assertEquals(
                "http://127.0.0.1:31337/",
                Entrance.withoutFragment("http://127.0.0.1:31337/#bootstrap=abc123"));
    }

    @Test
    public void fragment_が無ければそのまま() {
        assertEquals(
                "http://127.0.0.1:31337/",
                Entrance.withoutFragment("http://127.0.0.1:31337/"));
    }

    /** 入口がない場合は null を維持する。 */
    @Test
    public void 入口が無ければ_null_のまま() {
        assertNull(Entrance.withoutFragment(null));
    }

    /** 最初の # 以降を fragment として除去する。 */
    @Test
    public void 切れ目は最初の_hash() {
        assertEquals("http://x/", Entrance.withoutFragment("http://x/#a=1#b=2"));
    }

    /** 空の fragment も除去する。 */
    @Test
    public void 空の_fragment_も落とす() {
        assertEquals("http://x/", Entrance.withoutFragment("http://x/#"));
    }
}
