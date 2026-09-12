import type { Page } from "@playwright/test";
import { expect, openApplication, test, windowsShell } from "./support/environment";
import {
  drawnRowCount,
  loseTerminalWebGLContext,
  terminalCanvasCount,
  terminalDrawingRects,
  terminalKeyboard,
} from "./support/terminal";

// Renderer changes need the real WebGL path, unlike the DOM-oriented suite.
test.use({ launchOptions: { args: [] }, viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1 });

type TerminalSize = { cols: number; rows: number };

function watchTerminal(page: Page) {
  const sizes: TerminalSize[] = [];
  let output = "";
  page.on("websocket", (socket) => {
    socket.on("framesent", ({ payload }) => {
      try {
        const frame = JSON.parse(typeof payload === "string" ? payload : payload.toString("utf8")) as { resize?: TerminalSize };
        if (frame.resize !== undefined) sizes.push(frame.resize);
      } catch { /* Input frames are not resize messages. */ }
    });
    socket.on("framereceived", ({ payload }) => {
      output += typeof payload === "string" ? payload : payload.toString("utf8");
    });
  });
  return { sizes, output: () => output };
}

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
  await page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "Local shell" }).click();
  await expect(terminalKeyboard(page)).toBeAttached();
  await expect.poll(() => terminalCanvasCount(page)).toBeGreaterThan(0);
  await page.evaluate(async () => {
    await document.fonts.ready;
    await new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
  });
  await expectDrawingFits(page);
  return terminalDrawingRects(page);
}

async function expectPTYSize(page: Page, observed: ReturnType<typeof watchTerminal>) {
  const size = observed.sizes.at(-1)!;
  expect(size.cols).toBeGreaterThan(1);
  expect(size.rows).toBeGreaterThan(1);
  const outputStart = observed.output().length;
  await terminalKeyboard(page).focus();
  await page.keyboard.type(windowsShell
    ? '"FIT_SIZE=$($Host.UI.RawUI.WindowSize.Height)-$($Host.UI.RawUI.WindowSize.Width)"'
    : 'printf "FIT_SIZE="; stty size | tr " " "-"');
  await page.keyboard.press("Enter");
  await expect.poll(() => observed.output().slice(outputStart)).toContain(`FIT_SIZE=${size.rows}-${size.cols}`);
}

test("refits every terminal row and column when DPR changes without resizing the host", async ({ page, installation }) => {
  const observed = watchTerminal(page);
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
  await expectPTYSize(page, observed);
});

test("refits terminal columns and rows after WebGL context loss switches to DOM rendering", async ({ page, installation }) => {
  const observed = watchTerminal(page);
  await openApplication(page, installation);
  const before = await openWebGLShell(page);
  await expect.poll(() => observed.sizes.length).toBeGreaterThan(0);
  expect(await loseTerminalWebGLContext(page)).toBe(true);
  await expect.poll(() => terminalCanvasCount(page)).toBe(0);
  await expectDrawingFits(page);
  expect((await terminalDrawingRects(page)).host).toEqual(before.host);
  await expect.poll(() => drawnRowCount(page)).toBe(observed.sizes.at(-1)!.rows);
  await expectPTYSize(page, observed);
});
