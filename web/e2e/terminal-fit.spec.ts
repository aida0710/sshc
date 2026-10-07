import type { Page } from "@playwright/test";
import { expect, openApplication, test, openLocalShell } from "./support/environment";
import {
  drawnRowCount,
  loseTerminalWebGLContext,
  terminalCanvasCount,
  terminalDrawingRects,
  terminalKeyboard,
  waitForTerminalLayout,
} from "./support/terminal";

import { expectTerminalPTYSize, watchTerminalStream } from "./support/terminalStream";

// Renderer changes need the real WebGL path, unlike the DOM-oriented suite.
test.use({ launchOptions: { args: [] }, viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1 });

async function expectDrawingFits(page: Page) {
  await expect.poll(async () => {
    const { screen, host } = await terminalDrawingRects(page);
    return Math.max(
      host.x - screen.x,
      host.y - screen.y,
      screen.x + screen.width - host.x - host.width,
      screen.y + screen.height - host.y - host.height,
    );
  }, { message: "Every rendered terminal column and row must fit inside the unchanged host" }).toBeLessThanOrEqual(1);
}

async function openWebGLShell(page: Page) {
  await openLocalShell(page);
  await expect(terminalKeyboard(page)).toBeAttached();
  await expect.poll(() => terminalCanvasCount(page)).toBeGreaterThan(0);
  await waitForTerminalLayout(page);
  await expectDrawingFits(page);
  return terminalDrawingRects(page);
}

test("refits every terminal row and column when DPR changes without resizing the host", async ({ page, installation }) => {
  const observed = watchTerminalStream(page);
  await openApplication(page, installation);
  const before = await openWebGLShell(page);
  await expect.poll(() => observed.sizes.length).toBeGreaterThan(0);
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Emulation.setDeviceMetricsOverride", {
    width: 1440, height: 900, deviceScaleFactor: 1.1, mobile: false,
  });
  await expect.poll(() => page.evaluate(() => window.devicePixelRatio)).toBeCloseTo(1.1, 3);
  // Chromium updates device scale without changing the CSS viewport. Notify
  // xterm as the browser does when a window moves between differently scaled displays.
  await page.evaluate(async () => {
    window.dispatchEvent(new Event("resize"));
    // Let the renderer and ResizeObserver finish their queued layout work
    // before checking that the screen fits; its CSS size can remain unchanged.
    for (let frame = 0; frame < 3; frame++) {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    }
  });
  await expectDrawingFits(page);
  expect((await terminalDrawingRects(page)).host).toEqual(before.host);
  await expectTerminalPTYSize(page, observed);
});

test("refits terminal columns and rows after WebGL context loss switches to DOM rendering", async ({ page, installation }) => {
  const observed = watchTerminalStream(page);
  await openApplication(page, installation);
  const before = await openWebGLShell(page);
  await expect.poll(() => observed.sizes.length).toBeGreaterThan(0);
  expect(await loseTerminalWebGLContext(page)).toBe(true);
  await expect.poll(() => terminalCanvasCount(page)).toBe(0);
  await expectDrawingFits(page);
  expect((await terminalDrawingRects(page)).host).toEqual(before.host);
  await expect.poll(() => drawnRowCount(page)).toBe(observed.sizes.at(-1)!.rows);
  await expectTerminalPTYSize(page, observed);
});
