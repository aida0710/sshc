import type { Page } from "@playwright/test";
import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { terminalKeyboard } from "./support/terminal";

// bash marks its prompt, the start of each command and how it ended (OSC 133 A, B, C and D), as shell
// integration does.
const shellIntegration = "PS1='\\[\\e]133;A\\a\\]$ \\[\\e]133;B\\a\\]'; PS0='\\e]133;C\\a'; PROMPT_COMMAND='printf \"\\e]133;D;%s\\a\" $?'";

async function run(page: Page, line: string) {
  await terminalKeyboard(page).focus();
  await page.keyboard.type(line);
  await page.keyboard.press("Enter");
}

async function openShell(page: Page, installation: Parameters<typeof openApplication>[1]) {
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await run(page, shellIntegration);
  return screen;
}

test("marks each command by how it ended, with its details on pointing and its line kept above its output", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "the marks come from bash here");
  const screen = await openShell(page, installation);
  await run(page, "seq 1 80");
  await run(page, "ls /no-such-folder-for-sshc");
  await expect(screen).toContainText("No such file");
  const failed = page.locator('.sshc-command-gutter-mark[data-status="failed"]');
  await expect(failed).toHaveCount(1);
  await expect(page.locator('.sshc-command-gutter-mark[data-status="succeeded"]')).not.toHaveCount(0);

  const box = (await failed.boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  const card = page.getByRole("dialog", { name: "Details of ls /no-such-folder-for-sshc" });
  await expect(card).toContainText("Exit code 2");
  await expect(card).toContainText(/Started at/);
  await card.getByRole("button", { name: "Copy output" }).click();
  await expect(screen).toContainText("Copied");

  // Scrolled back into the numbers, their command stays above them; pressing it goes to its line.
  await page.mouse.move(box.x + 200, box.y);
  await page.mouse.wheel(0, -600);
  const header = page.getByRole("button", { name: "Go to the line of seq 1 80" });
  await expect(header).toBeVisible();
  await expect(header).toContainText("Succeeded");
  await header.click();
  await expect(header).toBeHidden();
});

test("shows none of it with the settings off", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "the marks come from bash here");
  await installation.write("sshc/metadata.json", JSON.stringify({ schemaVersion: 9, embeddedTerminal: { commandMarks: false, stickyCommand: false, commandScrollbarMarks: false } }));
  const screen = await openShell(page, installation);
  await run(page, "seq 1 80");
  await run(page, "echo done-$((40+2))");
  await expect(screen).toContainText("done-42");
  await expect(page.locator(".sshc-command-gutter-mark:not([hidden])")).toHaveCount(0);
  await page.mouse.wheel(0, -600);
  await expect(page.getByRole("button", { name: /^Go to the line of / })).toHaveCount(0);
});
