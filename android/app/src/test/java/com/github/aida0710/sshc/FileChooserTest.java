package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;

import java.util.Arrays;
import java.util.Collections;

import org.junit.Test;

/** WebView のファイル選択の要求と結果の扱いを検証する。 */
public final class FileChooserTest {
    @Test
    public void 画像の_accept_は_MIME_type_で絞り込む() {
        assertEquals(Arrays.asList("image/png", "image/jpeg"),
                FileChooser.mimeTypes(new String[] {"image/png,image/jpeg"}));
    }

    @Test
    public void 分かれて渡された_accept_もまとめる() {
        assertEquals(Arrays.asList("image/png", "image/webp"),
                FileChooser.mimeTypes(new String[] {"image/png", " image/webp "}));
    }

    @Test
    public void 拡張子を含む_accept_では絞り込まない() {
        assertTrue(FileChooser.mimeTypes(new String[] {".conf,text/plain"}).isEmpty());
    }

    @Test
    public void accept_が無ければ絞り込まない() {
        assertTrue(FileChooser.mimeTypes(new String[] {""}).isEmpty());
        assertTrue(FileChooser.mimeTypes(null).isEmpty());
    }

    @Test
    public void 一件だけ選ぶと_data_を返す() {
        assertEquals(Collections.singletonList("first"),
                FileChooser.chosen("first", Collections.emptyList()));
    }

    @Test
    public void 複数選ぶと_ClipData_の全件を返す() {
        assertEquals(Arrays.asList("first", "second"),
                FileChooser.chosen("first", Arrays.asList("first", "second")));
    }

    @Test
    public void data_が空でも_ClipData_の全件を返す() {
        assertEquals(Arrays.asList("first", "second"),
                FileChooser.chosen(null, Arrays.asList("first", "second")));
    }

    @Test
    public void 何も無ければ空() {
        assertTrue(FileChooser.chosen(null, Collections.<String>emptyList()).isEmpty());
    }
}
