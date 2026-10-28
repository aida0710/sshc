import { access, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { largestPNG, samplePNG } from "./support/images";
import { drawnCharacterColumns, drawnImagePixels, terminalKeyboard } from "./support/terminal";

async function runInShell(page: Page, command: string) {
  await terminalKeyboard(page).focus();
  await page.keyboard.type(command);
  await page.keyboard.press("Enter");
}

test("draws iTerm2, SIXEL and kitty images and keeps them out of the text after reopening", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "cat and printf are POSIX shell syntax here");
  const iterm = `\x1b]1337;File=name=c2FtcGxlLnBuZw==;inline=1;size=${samplePNG().length}:${samplePNG().toString("base64")}\x07`;
  const sixel = "\x1bP0;0;0q\"1;1;40;12#1;2;0;80;0#1!40~-!40~\x1b\\";
  // The transmit-and-display form Claude Code sends for an image it has as data.
  const kitty = `\x1b_Ga=T,t=d,f=100,q=2;${samplePNG().toString("base64")}\x1b\\`;
  await writeFile(join(installation.home, "iterm-image.txt"), `${iterm}\n`);
  await writeFile(join(installation.home, "sixel-image.txt"), `${sixel}\n`);
  await writeFile(join(installation.home, "kitty-image.txt"), `${kitty}\n`);
  await openApplication(page, installation);
  await page.evaluate(() => {
    (window as Window & { cspViolations?: string[] }).cspViolations = [];
    document.addEventListener("securitypolicyviolation", (event) => {
      (window as Window & { cspViolations?: string[] }).cspViolations?.push(event.violatedDirective);
    });
  });
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });

  await runInShell(page, "cd ~ && cat iterm-image.txt");
  await expect.poll(() => drawnImagePixels(page)).toBeGreaterThan(48 * 24 / 2);
  const afterIterm = await drawnImagePixels(page);
  await runInShell(page, "cat sixel-image.txt");
  await expect.poll(() => drawnImagePixels(page)).toBeGreaterThan(afterIterm);
  const afterSixel = await drawnImagePixels(page);
  await runInShell(page, "cat kitty-image.txt");
  await expect.poll(() => drawnImagePixels(page)).toBeGreaterThan(afterSixel);
  expect(await page.evaluate(() => (window as Window & { cspViolations?: string[] }).cspViolations)).toEqual([]);
  await expect(screen).not.toContainText("iVBORw0KGgo");
  await expect(screen).not.toContainText("a=T,t=d");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-images.png` });
  }

  // Leaving and reopening the session replays the scrollback through a new
  // terminal: the images come back and their data never shows as text.
  await openLocalShell(page);
  await page.getByRole("list", { name: "Open sessions" }).getByRole("listitem").first().getByRole("button").first().click();
  await expect(screen).toContainText("cat kitty-image.txt");
  await expect.poll(() => drawnImagePixels(page)).toBeGreaterThan(afterIterm);
  await expect(screen).not.toContainText("iVBORw0KGgo");
});

for (const [name, terminator] of [["BEL", "\x07"], ["ST", "\x1b\\"]] as const) {
  test(`draws an iTerm2 image without size terminated by ${name}, including after reopening`, async ({ page, installation }) => {
    test.skip(process.platform === "win32", "cat is POSIX shell syntax here");
    const image = samplePNG();
    const sequence = `\x1b]1337;File=name=c2FtcGxlLnBuZw==;inline=1:${image.toString("base64")}${terminator}`;
    await writeFile(join(installation.home, "iterm-without-size.txt"), `${sequence}\n`);
    await openApplication(page, installation);
    await openLocalShell(page);
    const screen = page.getByRole("region", { name: /^Terminal for / });
    await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });

    await runInShell(page, "cd ~ && cat iterm-without-size.txt");
    // More than half of the sample must be drawn, allowing for cell clipping.
    const minimumImagePixels = 48 * 24 / 2;
    await expect.poll(() => drawnImagePixels(page)).toBeGreaterThan(minimumImagePixels);
    await expect(screen).not.toContainText("iVBORw0KGgo");

    await openLocalShell(page);
    await page.getByRole("list", { name: "Open sessions" }).getByRole("listitem").first().getByRole("button").first().click();
    await expect(screen).toContainText("cat iterm-without-size.txt");
    await expect.poll(() => drawnImagePixels(page)).toBeGreaterThan(minimumImagePixels);
    await expect(screen).not.toContainText("iVBORw0KGgo");
  });
}

test("answers the kitty graphics query Claude Code sends to detect image support", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "read is POSIX shell syntax");
  // The reply (ESC _ G i=31;OK ESC \\) arrives as terminal input. `read` stops
  // at the backslash of its ST, and the script prints it without the ESCs.
  await writeFile(join(installation.home, "kitty-query.sh"), [
    String.raw`printf '\033_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\033\\'`,
    String.raw`IFS= read -r -s -t 2 -d '\' reply`,
    "reply=${reply//$'\\033'/}",
    "printf 'reply:%s\\n' \"${reply#_G}\"",
    "",
  ].join("\n"));
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await runInShell(page, "cd ~ && bash kitty-query.sh");
  await expect(screen).toContainText("reply:i=31;OK");
});

test("shows OSC 9;4 progress under the pane header and clears it", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });

  await runInShell(page, "printf '\\033]9;4;1;40\\a'");
  const progress = screen.getByRole("progressbar", { name: "Program progress" });
  await expect(progress).toHaveAttribute("aria-valuenow", "40");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-progress.png` });
  }
  await runInShell(page, "printf '\\033]9;4;2\\a'");
  await expect(screen.getByRole("progressbar", { name: "Program progress (error)" })).toHaveAttribute("aria-valuenow", "40");
  await runInShell(page, "printf '\\033]9;4;0\\a'");
  await expect(screen.getByRole("progressbar")).toHaveCount(0);
});

test("shows a background session's program progress in Open sessions while another session is displayed", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "the progress reporter is a POSIX shell loop");
  // Four reports, each waiting for the 2 s session list poll, plus two shells.
  test.setTimeout(60_000);
  const nextReport = join(installation.home, "next-progress-report");
  // The reporter prints a report only when the test writes one, so the output
  // arrives after this session's TerminalView is gone. A plain file (not a
  // FIFO) keeps a write from blocking if the shell has already stopped.
  await writeFile(join(installation.home, "program-progress.sh"), [
    "printf 'progress reporter ready\\n'",
    "while :; do",
    '  if [ -f "$HOME/next-progress-report" ]; then',
    '    report=$(cat "$HOME/next-progress-report"); rm -f "$HOME/next-progress-report"',
    String.raw`    printf '\033]9;4;%s\a' "$report"`,
    "  fi",
    "  sleep 0.1",
    "done",
    "",
  ].join("\n"));
  const report = async (value: string) => {
    await writeFile(nextReport, value);
    await expect.poll(() => access(nextReport).then(() => true, () => false)).toBe(false);
  };
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await runInShell(page, "sh ~/program-progress.sh");
  await expect(screen).toContainText("progress reporter ready");

  await openLocalShell(page);
  await expect(screen).not.toContainText("progress reporter ready");
  const rows = page.getByRole("list", { name: "Open sessions" }).getByRole("listitem");
  await expect(rows).toHaveCount(2);
  await expect(rows.nth(1).getByRole("button").first()).toHaveAttribute("aria-current", "true");
  const background = rows.first();
  await report("1;40");
  await expect(background.getByRole("progressbar", { name: "Program progress" })).toHaveAttribute("aria-valuenow", "40");
  await expect(screen.getByRole("progressbar")).toHaveCount(0);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/session-list-progress.png` });
  }

  await report("2");
  await expect(background.getByRole("progressbar", { name: "Program progress (error)" })).toHaveAttribute("aria-valuenow", "40");
  await report("3");
  await expect(background.getByRole("progressbar", { name: "Program progress" })).not.toHaveAttribute("aria-valuenow");
  await report("0");
  await expect(background.getByRole("progressbar")).toHaveCount(0);
});

test("counts an emoji as two cells so the rest of the line stays aligned", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });

  // Emoji from Unicode 6.1, 13, 14, 15 and 16, each followed by a bar. Octal
  // escapes keep "12|" out of the typed command, so waiting for it waits for
  // the output rather than the echo.
  const emoji = ["\u{1F600}", "\u{1F972}", "\u{1FAE0}", "\u{1FAE8}", "\u{1FAE9}"];
  const octal = (text: string) => [...Buffer.from(text, "utf8")].map((byte) => `\\${byte.toString(8)}`).join("");
  const lines = [...emoji.map((character) => `${octal(character)}${octal("|")}`), octal("12|")];
  await runInShell(page, `clear; printf '${lines.join("\\n")}\\n'`);
  await expect(screen).toContainText("12|");
  // The DOM renderer draws each row as text; every bar must share a column.
  const columns = await drawnCharacterColumns(page, "|", lines.length);
  expect(columns).toHaveLength(lines.length);
  for (const column of columns) expect(Math.abs(column - (columns.at(-1) ?? 0))).toBeLessThanOrEqual(1);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-emoji-width.png` });
  }
});

test("keeps the output moving when a program places a large stored image again and again", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "cat is POSIX shell syntax here");
  // Each placement decodes the stored image again; about 75 ms for this one,
  // so 200 placements held the terminal's output for 15 seconds.
  const image = largestPNG().toString("base64");
  const chunks: string[] = [];
  for (let offset = 0; offset < image.length; offset += 4096) {
    const more = offset + 4096 < image.length ? 1 : 0;
    chunks.push(`\x1b_G${offset === 0 ? "a=t,f=100,i=42,q=2," : ""}m=${more};${image.slice(offset, offset + 4096)}\x1b\\`);
  }
  const placements = "\x1b_Ga=p,i=42,c=4,r=2,q=2\x1b\\".repeat(200);
  await writeFile(join(installation.home, "placements.txt"), `${chunks.join("")}${placements}\n`);
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });

  await runInShell(page, "cd ~ && cat placements.txt; echo placed-$((6*7))");

  await expect(screen).toContainText("placed-42", { timeout: 5_000 });
  expect(await drawnImagePixels(page)).toBeGreaterThan(0);
});
