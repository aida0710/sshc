package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNull;

import org.junit.Test;

/** 選んだフォルダの中の名前をパスへつなぐ規則を検証する。 */
public final class ChosenFolderTest {
    @Test
    public void 名前を上の階層のパスへつなぐ() {
        assertEquals("photos", ChosenFolder.childPath("", "photos"));
        assertEquals("photos/2026/a.jpg", ChosenFolder.childPath("photos/2026", "a.jpg"));
    }

    @Test
    public void パスとして安全でない名前はつながない() {
        assertNull(ChosenFolder.childPath("photos", ".."));
        assertNull(ChosenFolder.childPath("photos", "."));
        assertNull(ChosenFolder.childPath("photos", "a/b"));
        assertNull(ChosenFolder.childPath("photos", "a\\b"));
        assertNull(ChosenFolder.childPath("photos", "a\nb"));
        assertNull(ChosenFolder.childPath("photos", ""));
        assertNull(ChosenFolder.childPath("photos", null));
    }
}
