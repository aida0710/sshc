import type { Page } from "@playwright/test";
import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { drawnRows, terminalKeyboard } from "./support/terminal";

// A program on the alternate screen, as vim draws it: it fills the screen past its bottom row, puts the cursor back
// at the top, and on each SIGWINCH clears the screen and writes "line N" at every row of the new size. It runs until
// Ctrl+C and then leaves the alternate screen.
const fullScreenProgram = [
  "draw() { printf '\\033[H\\033[2J'; rows=$(tput lines); for row in $(seq 1 \"$rows\"); do printf '\\033[%d;1Hline %d' \"$row\" \"$row\"; done; }",
  "trap draw WINCH",
  "trap 'printf \"\\033[?1049l\"; exit' INT",
  "printf '\\033[?1049h'",
  "seq 1 60",
  "printf '\\033[H'",
  "while :; do sleep 0.1; done",
].join("; ");

async function shownRows(page: Page): Promise<string[]> {
  return (await drawnRows(page).locator(":scope > div").allTextContents()).map((row) => row.replace(/\u00a0/gu, " ").trimEnd());
}

// On a phone the keyboard coming up makes the terminal shorter between two programs. xterm.js kept the empty
// alternate screen at its taller length, so the next program scrolled lines off its top as if it had a scrollback,
// and once the terminal was taller again its redraw landed in the wrong rows (alternateScreenLength.ts).
test("draws a full-screen program started in a shorter terminal in place once the terminal is taller", async ({ page, installation }) => {
  await page.setViewportSize({ width: 900, height: 800 });
  await openApplication(page, installation);
  await openLocalShell(page);
  await terminalKeyboard(page).focus();
  // A full-screen program comes and goes while the terminal is tall.
  await page.keyboard.type("PS1='$ '; printf '\\033[?1049h'; printf '\\033[?1049l'; clear\n");
  await expect.poll(async () => (await shownRows(page))[0]).toBe("$");
  const tall = (await shownRows(page)).length;

  await page.setViewportSize({ width: 900, height: 450 });
  await expect.poll(async () => (await shownRows(page)).length).toBeLessThan(tall);
  await page.keyboard.type(`bash -c '${fullScreenProgram.replace(/'/gu, "'\\''")}'\n`);
  await expect.poll(async () => (await shownRows(page)).at(-2)).toMatch(/^\d+$/u);

  await page.setViewportSize({ width: 900, height: 800 });
  await expect.poll(async () => (await shownRows(page)).length).toBe(tall);
  await expect.poll(async () => {
    const rows = await shownRows(page);
    return [rows[0], rows.at(-1)];
  }).toEqual(["line 1", `line ${tall}`]);

  await page.keyboard.press("Control+C");
});
