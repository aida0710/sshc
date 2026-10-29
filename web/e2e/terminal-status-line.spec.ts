import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { drawnRows, terminalKeyboard } from "./support/terminal";

async function run(page: import("@playwright/test").Page, line: string) {
  await terminalKeyboard(page).focus();
  await page.keyboard.type(line);
  await page.keyboard.press("Enter");
}

test("shows what a program writes to the VT320 status line under the screen, one row taken from the program", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf and stty are POSIX here");
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  const statusLine = page.locator("[data-terminal-status-line]");
  await expect(statusLine).toBeHidden();

  await run(page, "stty size | sed 's/^/before:/'");
  await expect(screen).toContainText(/before:\d+ \d+/);
  const before = Number(/before:(\d+)/.exec(await screen.innerText())![1]);

  // The text for the status line comes from arguments, so the typed command does not show it.
  await run(page, "printf '\\033[2$~\\033[1$}\\033[1m%s-%s\\033[0$}' status line");
  await expect(statusLine).toBeVisible();
  await expect(statusLine).toContainText("status-line");
  await expect(drawnRows(page)).not.toContainText("status-line");

  await run(page, "stty size | sed 's/^/after:/'");
  await expect(screen).toContainText(new RegExp(`after:${before - 1} \\d+`));

  await run(page, "printf '\\033[0$~'");
  await expect(statusLine).toBeHidden();
  await run(page, "stty size | sed 's/^/hidden:/'");
  await expect(screen).toContainText(new RegExp(`hidden:${before} \\d+`));
});

test("keeps what a program writes to the status line off the screen when the status line is turned off", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf and stty are POSIX here");
  await installation.write("sshc/metadata.json", JSON.stringify({ schemaVersion: 9, embeddedTerminal: { statusLine: false } }));
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });

  await run(page, "stty size | sed 's/^/before:/'");
  await expect(screen).toContainText(/before:\d+ \d+/);
  const before = Number(/before:(\d+)/.exec(await screen.innerText())![1]);
  await run(page, "printf '\\033[2$~\\033[1$}%s-%s\\033[0$}' status line");
  // The shell runs one command after the other, so the size comes once the status line was written.
  await run(page, "stty size | sed 's/^/after:/'");
  await expect(screen).toContainText(new RegExp(`after:${before} \\d+`));
  await expect(page.locator("[data-terminal-status-line]")).toBeHidden();
  await expect(drawnRows(page)).not.toContainText("status-line");
});
