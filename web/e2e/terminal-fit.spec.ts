import type { Page } from "@playwright/test";
import { expect, openApplication, openSection, test, openLocalShell } from "./support/environment";
import {
  drawnRowCount,
  loseTerminalWebGLContext,
  markWebGLCanvases,
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

test("sends a new session one size, measured without the connecting banner and with the WebGL renderer", async ({ page, installation }) => {
  const observed = watchTerminalStream(page);
  await openApplication(page, installation);
  await openWebGLShell(page);
  await expect.poll(() => observed.sizes.length).toBeGreaterThan(0);
  await expectTerminalPTYSize(page, observed);
  // A size measured under the banner (fewer rows) or before WebGL was in place (fewer columns)
  // would come first and be followed by the right one.
  const sizes = observed.sizes.map((size) => `${size.cols}x${size.rows}`);
  expect([...new Set(sizes)]).toEqual([sizes.at(-1)]);
});

// Coming back to the Terminal section used to build the WebGL renderer again: the screen stopped for a moment and the
// text was drawn with the DOM renderer's cells first, which wrap differently, then moved. The terminal on screen keeps
// its renderer while another section is open.
test("keeps the WebGL renderer of the terminal on screen while another section is open", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openWebGLShell(page);
  const canvases = await terminalCanvasCount(page);
  const kept = await markWebGLCanvases(page, "before-another-section");

  await openSection(page, "Home");
  await openSection(page, "Terminal");

  await expect(kept).toHaveCount(canvases);
  await expectDrawingFits(page);
});
