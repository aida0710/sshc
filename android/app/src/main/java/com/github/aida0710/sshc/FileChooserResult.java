package com.github.aida0710.sshc;

import java.util.Collections;
import java.util.List;

/**
 * ファイル選択の結果から WebView へ返す項目を選ぶ。Android API に依存しないため JVM テストで検証できる。
 */
final class FileChooserResult {
    private FileChooserResult() {}

    /**
     * 複数選ぶと選択画面は全件を ClipData だけで返し、Intent の data は空か先頭の 1 件になる。
     * WebChromeClient.FileChooserParams.parseResult は data しか読まないため、ClipData を優先する。
     */
    static <T> List<T> chosen(T data, List<T> clipItems) {
        if (!clipItems.isEmpty()) return clipItems;
        return data == null ? Collections.emptyList() : Collections.singletonList(data);
    }
}
