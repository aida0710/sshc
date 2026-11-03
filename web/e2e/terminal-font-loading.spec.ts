import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { standardPresetMetadata } from "./support/settings";
import { maxTerminalUnusedHeightPx, terminalDrawingRects, terminalKeyboard, waitForTerminalLayout } from "./support/terminal";
import { expectTerminalPTYSize, watchTerminalStream } from "./support/terminalStream";

// Both renderers must measure the loaded font consistently at fractional DPI.
test.use({ launchOptions: { args: [] }, viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1.1 });
for (const webgl of [true, false]) {
  test(`uses the same rows after delayed font loading as a new shell (${webgl ? "WebGL" : "DOM"})`, async ({ page, installation }) => {
    await installation.write("sshc/metadata.json", standardPresetMetadata({ terminal: { webgl, appearance: { font: "jetbrains-mono" } } }));
    let releaseFont!: () => void;
    const fontReady = new Promise<void>((done) => { releaseFont = done; });
    await page.route("**/fonts/JetBrainsMono-*.woff2", async (route) => {
      await fontReady;
      await route.continue();
    });
    const observed = watchTerminalStream(page);
    try {
      await openApplication(page, installation);
      await openLocalShell(page);
      await expect(terminalKeyboard(page)).toBeAttached();
      await expect.poll(() => observed.sizes.length).toBeGreaterThan(0);
      releaseFont();
      await page.evaluate(async () => {
        await document.fonts.load('13px "JetBrains Mono"');
        await document.fonts.ready;
      });
      await waitForTerminalLayout(page);
      // A command response also verifies that the PTY received the refit.
      await expectTerminalPTYSize(page, observed);
      const loaded = observed.sizes.at(-1)!;
      const drawing = await terminalDrawingRects(page);
      expect(drawing.screen.y + drawing.screen.height).toBeLessThanOrEqual(drawing.host.y + drawing.host.height);
      expect(drawing.host.height - drawing.screen.height).toBeLessThan(maxTerminalUnusedHeightPx);
      if (process.env.SSHC_VISUAL_DIR !== undefined) {
        await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/font-${webgl ? "webgl" : "dom"}-fixed.png` });
      }
      await terminalKeyboard(page).focus();
      await page.keyboard.type("exit");
      await page.keyboard.press("Enter");
      await expect(page.getByRole("region", { name: /^Terminal for / })).toContainText("The program exited");
      const priorFits = observed.sizes.length;
      await openLocalShell(page);
      await expect.poll(() => observed.sizes.length).toBeGreaterThan(priorFits);
      expect(observed.sizes.at(-1)).toEqual(loaded);
      await expectTerminalPTYSize(page, observed);
    } finally { releaseFont(); }
  });
}

// Android has no text font with these symbols. The browser fetches the bundled one when a symbol is first drawn, and
// WebGL's texture atlas would keep what it drew until then.
for (const webgl of [true, false]) {
  test(`draws the symbols again once their font arrives (${webgl ? "WebGL" : "DOM"})`, async ({ page, installation }) => {
    await installation.write("sshc/metadata.json", standardPresetMetadata({ terminal: { webgl } }));
    let releaseFont!: () => void;
    const fontReady = new Promise<void>((done) => { releaseFont = done; });
    await page.route("**/fonts/SshcTerminalSymbols.woff2", async (route) => {
      await fontReady;
      await route.continue();
    });
    const nextFrames = () => page.evaluate(() => new Promise((drawn) => { requestAnimationFrame(() => requestAnimationFrame(drawn)); }));
    // Through the protocol: page.screenshot waits for the fonts, and this font is being held back.
    const screenshot = async (clip: { x: number; y: number; width: number; height: number }) => {
      const browserSession = await page.context().newCDPSession(page);
      const { data } = await browserSession.send("Page.captureScreenshot", { clip: { ...clip, scale: 1 } });
      await browserSession.detach();
      return data;
    };
    const observed = watchTerminalStream(page);
    try {
      await openApplication(page, installation);
      await openLocalShell(page);
      await expect(terminalKeyboard(page)).toBeAttached();
      await expect.poll(() => observed.output()).toMatch(/[$#%>]/);
      await terminalKeyboard(page).focus();
      // The stream says when the symbols are on the screen, in a renderer that has no rows to read. Only the output
      // holds "symbols-printed": the line typed holds its %s.
      await page.keyboard.type("clear; printf '\\342\\217\\265\\342\\217\\265 symbols-%s\\n' printed");
      await page.keyboard.press("Enter");
      await expect.poll(() => observed.output()).toContain("symbols-printed");
      await nextFrames();
      const drawing = await terminalDrawingRects(page);
      // The top row only: the cursor below it blinks.
      const symbols = { x: drawing.screen.x, y: drawing.screen.y, width: 60, height: 16 };
      const beforeFont = await screenshot(symbols);

      releaseFont();
      await page.evaluate(async () => { await document.fonts.load('16px "sshc Terminal Symbols"', "\u23F5"); });
      await nextFrames();

      await expect.poll(() => screenshot(symbols)).not.toBe(beforeFont);
    } finally { releaseFont(); }
  });
}
