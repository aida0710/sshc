package com.github.aida0710.sshc;

import androidx.activity.BackEventCompat;
import androidx.activity.OnBackPressedCallback;

import java.util.Locale;

/**
 * Android の戻る（ボタンとジェスチャー）を Web 画面へ渡す。ジェスチャーのあいだは進み具合を
 * sshc-android-back-gesture イベントで送り、Web 画面はそれに合わせてページを縮める
 * （web/src/android/backGestureMotion.ts）。Web 画面が戻るを自分のものではないと知らせたとき
 * （Home の一番上で何も開いていない、web/src/android/backOwnership.ts）は止まる。そのときは Android が
 * 戻るを受け、ホーム画面へ戻る動きを自分で描く。
 */
final class PageBackCallback extends OnBackPressedCallback {
    interface Page {
        /** Web 画面で script を実行する。Web 画面が無ければ何もしない。 */
        void evaluate(String script);
        /** 戻るが押された、またはジェスチャーが離された。 */
        void goBack();
    }

    private final Page page;
    private boolean gestureStarted;
    private int gestureEdge = BackEventCompat.EDGE_LEFT;

    PageBackCallback(Page page) {
        super(true);
        this.page = page;
    }

    @Override
    public void handleOnBackStarted(BackEventCompat backEvent) {
        gestureStarted = true;
        gestureEdge = backEvent.getSwipeEdge();
        page.evaluate(gestureScript("start", backEvent.getProgress(), gestureEdge));
    }

    @Override
    public void handleOnBackProgressed(BackEventCompat backEvent) {
        if (!gestureStarted) return;
        page.evaluate(gestureScript("progress", backEvent.getProgress(), gestureEdge));
    }

    @Override
    public void handleOnBackCancelled() {
        if (!gestureStarted) return;
        gestureStarted = false;
        page.evaluate(gestureScript("cancel", 0f, gestureEdge));
    }

    @Override
    public void handleOnBackPressed() {
        if (gestureStarted) {
            gestureStarted = false;
            page.evaluate(gestureScript("commit", 1f, gestureEdge));
        }
        page.goBack();
    }

    /** ジェスチャーの段階（start、progress、commit、cancel）と進み具合と、指が来た端を Web 画面へ伝える script。 */
    static String gestureScript(String phase, float progress, int edge) {
        return String.format(Locale.ROOT,
                "window.dispatchEvent(new CustomEvent('sshc-android-back-gesture',{detail:{phase:'%s',progress:%.4f,edge:'%s'}}))",
                phase, progress, edge == BackEventCompat.EDGE_RIGHT ? "right" : "left");
    }
}
