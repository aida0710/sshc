import type { Page } from "@playwright/test";
import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { drawnRows, terminalKeyboard } from "./support/terminal";

// A shell edits a long line for the width it has: at the right edge it moves the cursor to go on. Opened in a
// narrower browser, the session's history must be replayed at the width it was written for and then rewrapped;
// replayed at the narrower width, those cursor moves overwrite the middle of the line (#536).
const marked = Array.from({ length: 40 }, (_, index) => `_${String(index).padStart(2, "0")}_abcdefghij`).join("");

// shownText is the terminal's rows joined without breaks, so a line that wraps reads as it was typed.
async function shownText(page: Page): Promise<string> {
  const rows = await drawnRows(page).locator(":scope > div").allTextContents();
  return rows.join("").replace(/ /gu, " ");
}

test("opens a session typed in a wide browser in a narrower one without losing the middle of the typed line", async ({ page, browser, context, installation }) => {
  await page.setViewportSize({ width: 1600, height: 800 });
  await openApplication(page, installation);
  await openLocalShell(page);
  await terminalKeyboard(page).focus();
  await page.keyboard.type("PS1='$ '; clear\n");
  // The shell must have the wide browser's width before the line is typed: until the terminal has told the
  // engine its size, the shell edits for the size the session was opened with.
  await expect.poll(async () => {
    await page.keyboard.type("stty size\n");
    const sizes = [...(await shownText(page)).matchAll(/\$ stty size(\d+) (\d+)/gu)];
    return Number(sizes.at(-1)?.[2] ?? 0);
  }).toBeGreaterThan(150);
  // Typed ahead of clear, the line would be echoed before the screen is cleared and leave its start behind.
  await page.keyboard.type("clear\n");
  await expect.poll(async () => (await shownText(page)).trim()).toBe("$");
  await page.keyboard.type(`echo ${marked}\n`);
  await expect.poll(() => shownText(page)).toContain(`$ echo ${marked}${marked}`);

  const second = await browser.newContext({ storageState: await context.storageState(), viewport: { width: 900, height: 800 } });
  try {
    const other = await second.newPage();
    await other.goto(page.url());
    await other.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: /^bash/ }).first().click();
    await expect(drawnRows(other)).toContainText("_39_abcdefghij");
    await expect.poll(() => shownText(other)).toContain(`$ echo ${marked}${marked}`);
  } finally {
    await second.close();
  }
});
