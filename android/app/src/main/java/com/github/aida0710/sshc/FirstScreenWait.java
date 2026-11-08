package com.github.aida0710.sshc;

import android.app.Activity;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.view.View;
import android.window.SplashScreenView;

/**
 * 起動のスプラッシュを、Web 画面が最初の画面を中身まで描くまで出しておく。先に消すと、真っ黒な画面、
 * スケルトン、「読み込んでいます…」が続けて見える。そのあいだも WebView は下で描くので、消した瞬間に
 * 描けていない画面は見えない。スプラッシュを残せるのは Android 12 以降で、それより前は今までどおり起動の間の
 * 黒い地からページへ移る。
 */
final class FirstScreenWait {
    /**
     * エンジンの起動やページの読み込みがこれより長くかかったら待つのをやめ、ページ自身の読み込み中の表示を見せる。
     * スプラッシュのまま止まって見える時間を長くしないため。
     */
    static final long LIMIT_MILLIS = 3000;
    /** スプラッシュを消すときのフェードの長さ。システムのスプラッシュが消えるときと同じくらいにする。 */
    private static final long FADE_MILLIS = 200;

    private final Activity activity;
    private final Handler handler = new Handler(Looper.getMainLooper());
    /** 残しているスプラッシュ（Android 12 以降の SplashScreenView）。 */
    private View splash;
    private boolean shown;

    FirstScreenWait(Activity activity) {
        this.activity = activity;
    }

    /** Activity の onCreate で呼ぶ。 */
    void start() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            // この listener を置くと、システムはスプラッシュを自分では消さず、消すのはアプリに任せる。
            activity.getSplashScreen().setOnExitAnimationListener(view -> {
                splash = view;
                if (shown) fadeSplash();
            });
        }
        handler.postDelayed(this::show, LIMIT_MILLIS);
    }

    /** 最初の画面が出せる。スプラッシュを消す。UI スレッドで呼ぶ。2 回目からは何もしない。 */
    void show() {
        if (shown) return;
        shown = true;
        handler.removeCallbacksAndMessages(null);
        if (splash != null) fadeSplash();
    }

    private void fadeSplash() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.S) return;
        SplashScreenView fading = (SplashScreenView) splash;
        splash = null;
        fading.animate().alpha(0f).setDuration(FADE_MILLIS).withEndAction(fading::remove);
    }
}
