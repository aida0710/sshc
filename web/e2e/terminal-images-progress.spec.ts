import { access, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { deflateSync } from "node:zlib";
import type { Page } from "@playwright/test";
import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { drawnCharacterColumns, drawnImagePixels, terminalKeyboard } from "./support/terminal";

function crc32(bytes: Buffer): number {
  let crc = 0xffffffff;
  for (const byte of bytes) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit += 1) crc = (crc >>> 1) ^ (0xedb88320 & -(crc & 1));
  }
  return (crc ^ 0xffffffff) >>> 0;
}

// A 48×24 PNG whose left half is red and right half is blue.
function samplePNG(): Buffer {
  const width = 48;
  const height = 24;
  const rows = Buffer.concat(Array.from({ length: height }, () => Buffer.concat([
    Buffer.from([0]),
    ...Array.from({ length: width }, (_, x) => Buffer.from(x < width / 2 ? [0xd0, 0x30, 0x30] : [0x30, 0x60, 0xd0])),
  ])));
  const chunk = (type: string, data: Buffer) => {
    const length = Buffer.alloc(4);
    length.writeUInt32BE(data.length);
    const body = Buffer.concat([Buffer.from(type, "ascii"), data]);
    const checksum = Buffer.alloc(4);
    checksum.writeUInt32BE(crc32(body));
    return Buffer.concat([length, body, checksum]);
  };
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header.set([8, 2, 0, 0, 0], 8);
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk("IHDR", header), chunk("IDAT", deflateSync(rows)), chunk("IEND", Buffer.alloc(0)),
  ]);
}

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

  // Octal escapes keep "12|" out of the typed command, so waiting for it waits
  // for the output rather than the echo.
  await runInShell(page, "clear; printf '\\360\\237\\230\\200\\174\\n\\061\\062\\174\\n'");
  await expect(screen).toContainText("12|");
  // The DOM renderer draws each row as text; the two bars must share a column.
  const columns = await drawnCharacterColumns(page, "|", 2);
  expect(columns).toHaveLength(2);
  expect(Math.abs((columns[0] ?? 0) - (columns[1] ?? 0))).toBeLessThanOrEqual(1);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-emoji-width.png` });
  }
});
