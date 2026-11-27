package com.github.aida0710.sshc;

import static org.junit.Assert.assertEquals;

import java.util.EnumMap;
import java.util.Map;

import org.junit.Test;

public final class ScreenCornersTest {
    // 角の丸みの半径は 150px、density は 3.5。
    private static final int RADIUS = 150;
    private static final float DENSITY = 3.5f;

    /** 幅 width・高さ height の画面の、4 つの角の丸み。 */
    private static Map<ScreenCorners.Position, ScreenCorners.Corner> roundedScreen(int width, int height) {
        Map<ScreenCorners.Position, ScreenCorners.Corner> screen = new EnumMap<>(ScreenCorners.Position.class);
        screen.put(ScreenCorners.Position.TOP_LEFT, new ScreenCorners.Corner(RADIUS, RADIUS, RADIUS));
        screen.put(ScreenCorners.Position.TOP_RIGHT, new ScreenCorners.Corner(RADIUS, width - RADIUS, RADIUS));
        screen.put(ScreenCorners.Position.BOTTOM_RIGHT, new ScreenCorners.Corner(RADIUS, width - RADIUS, height - RADIUS));
        screen.put(ScreenCorners.Position.BOTTOM_LEFT, new ScreenCorners.Corner(RADIUS, RADIUS, height - RADIUS));
        return screen;
    }

    @Test
    public void reachesIntoTheWebViewByTheRoundingLeftBelowTheBars() {
        // 縦向きの 1440x3120。上のステータスバーは 105px、下のナビゲーションバーは 63px。左右に余白は無い。
        ScreenCorners.Bounds webView = new ScreenCorners.Bounds(0, 105, 1440, 3120 - 63);

        assertEquals(
                "{\"topLeft\":[43,13],\"topRight\":[43,13],\"bottomRight\":[43,25],\"bottomLeft\":[43,25]}",
                ScreenCorners.inside(roundedScreen(1440, 3120), webView, DENSITY));
    }

    @Test
    public void leavesACornerOutWhenTheBarIsDeeperThanTheRounding() {
        // ステータスバー（160px）が丸み（150px）より深いので、上の角は WebView にかからない。
        ScreenCorners.Bounds webView = new ScreenCorners.Bounds(0, 160, 1440, 3120 - 63);

        assertEquals(
                "{\"topLeft\":[0,0],\"topRight\":[0,0],\"bottomRight\":[43,25],\"bottomLeft\":[43,25]}",
                ScreenCorners.inside(roundedScreen(1440, 3120), webView, DENSITY));
    }

    @Test
    public void leavesTheBottomCornersOutWhileTheKeyboardIsOpen() {
        ScreenCorners.Bounds webView = new ScreenCorners.Bounds(0, 105, 1440, 3120 - 1100);

        assertEquals(
                "{\"topLeft\":[43,13],\"topRight\":[43,13],\"bottomRight\":[0,0],\"bottomLeft\":[0,0]}",
                ScreenCorners.inside(roundedScreen(1440, 3120), webView, DENSITY));
    }

    @Test
    public void leavesOutTheSideWithTheCameraCutoutInLandscape() {
        // 横向きの 3120x1440。左にカメラの切り欠き（170px）、右にナビゲーションバー（63px）がある。
        ScreenCorners.Bounds webView = new ScreenCorners.Bounds(170, 105, 3120 - 63, 1440);

        assertEquals(
                "{\"topLeft\":[0,0],\"topRight\":[25,13],\"bottomRight\":[25,43],\"bottomLeft\":[0,0]}",
                ScreenCorners.inside(roundedScreen(3120, 1440), webView, DENSITY));
    }

    @Test
    public void reachesNowhereOnAScreenWithSquareCorners() {
        ScreenCorners.Bounds webView = new ScreenCorners.Bounds(0, 105, 1440, 3120 - 63);

        assertEquals(
                "{\"topLeft\":[0,0],\"topRight\":[0,0],\"bottomRight\":[0,0],\"bottomLeft\":[0,0]}",
                ScreenCorners.inside(new EnumMap<>(ScreenCorners.Position.class), webView, DENSITY));
        assertEquals(
                "{\"topLeft\":[0,0],\"topRight\":[0,0],\"bottomRight\":[0,0],\"bottomLeft\":[0,0]}",
                ScreenCorners.NONE);
    }
}
