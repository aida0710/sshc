package com.github.aida0710.sshc;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/**
 * WebView のファイル選択の要求と結果を扱う。Android API に依存しないため JVM テストで検証できる。
 */
final class FileChooser {
    private FileChooser() {}

    /**
     * input の accept から、選択画面で絞り込む MIME type を返す。空なら絞り込まない。
     * 拡張子（.conf など）は端末側の MIME type が当てにならず（.conf は application/octet-stream になる）、
     * 絞り込むと目的のファイルが選べなくなるため、拡張子を含む accept では絞り込まない。
     */
    static List<String> mimeTypes(String[] acceptTypes) {
        List<String> mimeTypes = new ArrayList<>();
        if (acceptTypes == null) return mimeTypes;
        for (String acceptType : acceptTypes) {
            if (acceptType == null) continue;
            for (String entry : acceptType.split(",")) {
                String trimmed = entry.trim();
                if (trimmed.isEmpty()) continue;
                if (!trimmed.contains("/")) return Collections.emptyList();
                mimeTypes.add(trimmed);
            }
        }
        return mimeTypes;
    }

    /**
     * 複数選ぶと選択画面は全件を ClipData だけで返し、Intent の data は空か先頭の 1 件になる。
     * WebChromeClient.FileChooserParams.parseResult は data しか読まないため、ClipData を優先する。
     */
    static <T> List<T> chosen(T data, List<T> clipItems) {
        if (!clipItems.isEmpty()) return clipItems;
        return data == null ? Collections.emptyList() : Collections.singletonList(data);
    }
}
