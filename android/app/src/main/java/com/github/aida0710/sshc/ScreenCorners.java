package com.github.aida0710.sshc;

import android.graphics.Point;
import android.os.Build;
import android.view.RoundedCorner;
import android.view.View;
import android.view.WindowInsets;

import java.util.EnumMap;
import java.util.Locale;
import java.util.Map;
import java.util.StringJoiner;

/**
 * 画面の角の丸みが、WebView の四隅にどれだけ入り込んでいるか。Web 画面は、リモートセッションの外枠の四隅を
 * この分だけ丸め、角で枠が切れないようにする（web/src/android/screenCorners.ts）。
 *
 * 角ごとに、WebView の左右の辺と上下の辺から、画面の角の丸みの中心までの長さ（CSS px）を返す。この 2 つを
 * 半径にした楕円は、角の丸み（円）の内側に収まり、WebView の 2 つの辺に接する。システムバーの余白が丸みより
 * 深い角は、WebView に入り込まないので 0 になる。
 */
final class ScreenCorners {
    /** 画面の角の丸み 1 つ。中心は窓の座標（物理 px）。 */
    static final class Corner {
        final int radius;
        final int centerX;
        final int centerY;

        Corner(int radius, int centerX, int centerY) {
            this.radius = radius;
            this.centerX = centerX;
            this.centerY = centerY;
        }
    }

    /** 窓の中の WebView の位置（物理 px）。 */
    static final class Bounds {
        final int left;
        final int top;
        final int right;
        final int bottom;

        Bounds(int left, int top, int right, int bottom) {
            this.left = left;
            this.top = top;
            this.right = right;
            this.bottom = bottom;
        }
    }

    /** 4 つの角。JSON での名前と、WebView の辺から丸みの中心までの測り方を持つ。 */
    enum Position {
        TOP_LEFT("topLeft", RoundedCorner.POSITION_TOP_LEFT) {
            @Override
            int fromSide(Corner corner, Bounds webView) {
                return corner.centerX - webView.left;
            }

            @Override
            int fromEnd(Corner corner, Bounds webView) {
                return corner.centerY - webView.top;
            }
        },
        TOP_RIGHT("topRight", RoundedCorner.POSITION_TOP_RIGHT) {
            @Override
            int fromSide(Corner corner, Bounds webView) {
                return webView.right - corner.centerX;
            }

            @Override
            int fromEnd(Corner corner, Bounds webView) {
                return corner.centerY - webView.top;
            }
        },
        BOTTOM_RIGHT("bottomRight", RoundedCorner.POSITION_BOTTOM_RIGHT) {
            @Override
            int fromSide(Corner corner, Bounds webView) {
                return webView.right - corner.centerX;
            }

            @Override
            int fromEnd(Corner corner, Bounds webView) {
                return webView.bottom - corner.centerY;
            }
        },
        BOTTOM_LEFT("bottomLeft", RoundedCorner.POSITION_BOTTOM_LEFT) {
            @Override
            int fromSide(Corner corner, Bounds webView) {
                return corner.centerX - webView.left;
            }

            @Override
            int fromEnd(Corner corner, Bounds webView) {
                return webView.bottom - corner.centerY;
            }
        };

        final String jsonName;
        /** WindowInsets.getRoundedCorner に渡す位置。 */
        final int roundedCorner;

        Position(String jsonName, int roundedCorner) {
            this.jsonName = jsonName;
            this.roundedCorner = roundedCorner;
        }

        /** WebView の左か右の辺から、丸みの中心までの長さ（物理 px）。 */
        abstract int fromSide(Corner corner, Bounds webView);

        /** WebView の上か下の辺から、丸みの中心までの長さ（物理 px）。 */
        abstract int fromEnd(Corner corner, Bounds webView);
    }

    /** 入り込んでいる角が無いとき。Android 12 より前は、OS が角の丸みを教えない。 */
    static final String NONE = inside(new EnumMap<>(Position.class), new Bounds(0, 0, 0, 0), 1f);

    private ScreenCorners() {
    }

    /** WebView が今置かれている窓の角の丸みから、入り込みを JSON で返す。レイアウトのあとに呼ぶ。 */
    static String of(View webView) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.S) return NONE;
        WindowInsets insets = webView.getRootWindowInsets();
        if (insets == null) return NONE;
        Map<Position, Corner> screen = new EnumMap<>(Position.class);
        for (Position position : Position.values()) {
            RoundedCorner corner = insets.getRoundedCorner(position.roundedCorner);
            if (corner == null) continue;
            Point center = corner.getCenter();
            screen.put(position, new Corner(corner.getRadius(), center.x, center.y));
        }
        int[] location = new int[2];
        webView.getLocationInWindow(location);
        Bounds bounds = new Bounds(location[0], location[1],
                location[0] + webView.getWidth(), location[1] + webView.getHeight());
        return inside(screen, bounds, webView.getResources().getDisplayMetrics().density);
    }

    /**
     * 角ごとの入り込みを、{"topLeft":[横,縦],"topRight":…,"bottomRight":…,"bottomLeft":…} の JSON で返す。
     * screen に無い角は [0,0]。density は物理 px と CSS px の比で、WebView の devicePixelRatio と同じ。
     */
    static String inside(Map<Position, Corner> screen, Bounds webView, float density) {
        StringJoiner json = new StringJoiner(",", "{", "}");
        for (Position position : Position.values()) {
            Corner corner = screen.get(position);
            int fromSide = corner == null ? 0 : position.fromSide(corner, webView);
            int fromEnd = corner == null ? 0 : position.fromEnd(corner, webView);
            // 丸みの中心が WebView の外（長さが 0 以下）なら、その角の丸みは WebView にかからない。
            boolean reaches = corner != null && corner.radius > 0 && fromSide > 0 && fromEnd > 0;
            json.add(String.format(Locale.ROOT, "\"%s\":[%d,%d]", position.jsonName,
                    reaches ? cssPixels(fromSide, density) : 0, reaches ? cssPixels(fromEnd, density) : 0));
        }
        return json.toString();
    }

    // 切り上げる。切り捨てると、枠の角が丸みの外へ 1px はみ出ることがある。
    private static int cssPixels(int physical, float density) {
        return (int) Math.ceil(physical / density);
    }
}
