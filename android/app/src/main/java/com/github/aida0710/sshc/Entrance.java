package com.github.aida0710.sshc;

/**
 * 入口の URL の扱い。
 *
 * <p><b>Android を必要としない部分だけをここへ出してある。</b> engine の寿命を
 * 持つ Service は端末が無ければ動かせないが、この判断は文字列だけで決まる——
 * だから <code>src/test</code> の素の JUnit で確かめられる。
 */
final class Entrance {
    private Entrance() {}

    /**
     * 2 回目以降に渡す形。fragment を落とす。
     *
     * <p><b>入口は一度しか使えない。</b> fragment は最初の 1 回で使い切られる
     * ので、同じものを読み直すと 2 回目の bootstrap が拒否され、画面は
     * 「開始できませんでした」に落ちる。Activity は設定が変わるたびに作り直され
     * 得る——ダークモードの切り替え、フォントサイズ、分割画面、メモリ逼迫。
     * そのどれでも同じ URL をもう一度渡せば、二度と開かないアプリになる。
     *
     * <p>ページはクッキーだけで届いたときに session を更新する道を既に持って
     * おり、クッキーは同じプロセスの WebView が持ち続けている。
     */
    static String withoutFragment(String url) {
        if (url == null) return null;
        int fragment = url.indexOf('#');
        return fragment < 0 ? url : url.substring(0, fragment);
    }
}
