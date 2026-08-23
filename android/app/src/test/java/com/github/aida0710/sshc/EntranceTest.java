package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import org.junit.Test;

/**
 * 入口の URL の扱い。
 *
 * <p><b>この外殻には、これまで検査が一つも無かった。</b> Java は 380 行あって、
 * CI は <code>assembleDebug</code> で「コンパイルは通る」ことだけを見ていた。
 *
 * <p>ここで確かめているのは、**アプリが二度と開かなくなる**経路である。入口の
 * fragment は最初の 1 回で使い切られるので、2 回目に同じものを渡せば bootstrap
 * が拒否される。Activity は画面回転でもダークモード切り替えでも作り直されるので、
 * これは珍しい経路ではなく、**普通に使えば必ず通る**経路である。
 */
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

    /** engine が起きていなければ入口は無い。null を投げ返さない。 */
    @Test
    public void 入口が無ければ_null_のまま() {
        assertNull(Entrance.withoutFragment(null));
    }

    /**
     * <b>最初の # で切る。</b> fragment の中に # がもう一つ現れても、切れ目は
     * 一つ目である——URL の規格がそう決めている。
     */
    @Test
    public void 切れ目は最初の_hash() {
        assertEquals("http://x/", Entrance.withoutFragment("http://x/#a=1#b=2"));
    }

    /** 空の fragment も fragment である。落として空でない URL を残す。 */
    @Test
    public void 空の_fragment_も落とす() {
        assertEquals("http://x/", Entrance.withoutFragment("http://x/#"));
    }
}
