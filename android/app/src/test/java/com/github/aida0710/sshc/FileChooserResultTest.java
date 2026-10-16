package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import java.util.Arrays;
import java.util.Collections;

import org.junit.Test;

/** ファイル選択の結果から WebView へ返す項目の選び方を検証する。 */
public final class FileChooserResultTest {
    @Test
    public void 一件だけ選ぶと_data_を返す() {
        assertEquals(Collections.singletonList("first"),
                FileChooserResult.chosen("first", Collections.emptyList()));
    }

    @Test
    public void 複数選ぶと_ClipData_の全件を返す() {
        assertEquals(Arrays.asList("first", "second"),
                FileChooserResult.chosen("first", Arrays.asList("first", "second")));
    }

    @Test
    public void data_が空でも_ClipData_の全件を返す() {
        assertEquals(Arrays.asList("first", "second"),
                FileChooserResult.chosen(null, Arrays.asList("first", "second")));
    }

    @Test
    public void 何も無ければ空() {
        assertTrue(FileChooserResult.chosen(null, Collections.<String>emptyList()).isEmpty());
    }
}
