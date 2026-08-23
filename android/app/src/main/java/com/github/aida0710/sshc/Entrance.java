package com.github.aida0710.sshc;

/**
 * 入口 URL の文字列処理。Android API に依存しないため JVM テストで検証できる。
 */
final class Entrance {
    private Entrance() {}

    /**
     * 2 回目以降に渡す URL から、一度だけ使える bootstrap fragment を除去する。
     * WebView のセッションは同一プロセス内の cookie で維持される。
     */
    static String withoutFragment(String url) {
        if (url == null) return null;
        int fragment = url.indexOf('#');
        return fragment < 0 ? url : url.substring(0, fragment);
    }
}
