package com.github.aida0710.sshc;

import android.content.Context;
import android.view.View;
import android.widget.LinearLayout;
import android.widget.TextView;

/**
 * ネイティブの画面（起動の失敗、画面の停止からの復旧）の中身を組み立てる。見出し、説明、
 * 操作を、同じ余白と間隔で縦に並べる。
 */
final class NativeScreenContent {
    private static final int TITLE_TEXT_SP = 20;
    private static final int BODY_TEXT_SP = 15;
    private static final int HORIZONTAL_PADDING_DP = 24;
    private static final int VERTICAL_PADDING_DP = 32;
    /** 見出し・説明・操作のあいだの間隔。 */
    static final int GAP_DP = 16;

    private final Context context;
    private final float density;
    private final LinearLayout content;

    NativeScreenContent(Context context) {
        this.context = context;
        density = context.getResources().getDisplayMetrics().density;
        content = new LinearLayout(context);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(dp(HORIZONTAL_PADDING_DP), dp(VERTICAL_PADDING_DP),
                dp(HORIZONTAL_PADDING_DP), dp(VERTICAL_PADDING_DP));
    }

    /** 中身を画面の縦の中央に寄せる。 */
    NativeScreenContent centredVertically() {
        content.setGravity(android.view.Gravity.CENTER_VERTICAL);
        return this;
    }

    /** 最初に置く見出し。 */
    void title(int text) {
        TextView title = new TextView(context);
        title.setText(text);
        title.setTextSize(TITLE_TEXT_SP);
        add(title, 0);
    }

    /** 見出しの下の説明。 */
    void paragraph(int text) {
        TextView paragraph = new TextView(context);
        paragraph.setText(text);
        paragraph.setTextSize(BODY_TEXT_SP);
        add(paragraph, GAP_DP);
    }

    /** view を、上に topMarginDp の間隔を空けて、幅いっぱいに置く。 */
    void add(View view, int topMarginDp) {
        LinearLayout.LayoutParams layout = new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT);
        layout.setMargins(0, dp(topMarginDp), 0, 0);
        content.addView(view, layout);
    }

    LinearLayout view() {
        return content;
    }

    private int dp(int value) {
        return Math.round(value * density);
    }
}
