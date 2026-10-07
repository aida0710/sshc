import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { maxTerminalUnusedHeightPx, terminalDrawingRects, terminalKeyboard, waitForTerminalLayout } from "./support/terminal";
import { expectTerminalPTYSize, watchTerminalStream } from "./support/terminalStream";

// Both renderers must measure the loaded font consistently at fractional DPI.
test.use({ launchOptions: { args: [] }, viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1.1 });
for (const webgl of [true, false]) {
  test(`uses the same rows after delayed font loading as a new shell (${webgl ? "WebGL" : "DOM"})`, async ({ page, installation }) => {
    await installation.write("sshc/metadata.json", JSON.stringify({ schemaVersion: 3, embeddedTerminal: { webgl, appearance: { font: "jetbrains-mono" } } }));
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
