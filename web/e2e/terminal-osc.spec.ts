import { expect, openApplication, openSection, test, openLocalShell } from "./support/environment";
import { terminalKeyboard } from "./support/terminal";

// Programs rename panes with OSC 0/1/2 and raise notifications with
// OSC 9/99/777. The engine records both, so the pane header, the console
// list and the unread marks follow without any agent-specific plugin.
test("applies terminal titles to the pane and marks notifications unread", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "printf escape sequences are POSIX shell syntax");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.setViewportSize({ width: 1440, height: 900 });
  }
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const nav = page.getByRole("navigation", { name: "Primary" });
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Console for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();

  // `read` keeps the program in the foreground so a prompt that sets its own
  // title cannot immediately replace ours.
  await page.keyboard.type("printf '\\033]0;deploy checklist\\a'; read -r _");
  await page.keyboard.press("Enter");
  const consoles = nav.getByRole("list", { name: "Open consoles" });
  await expect(page.getByRole("region", { name: "Console for deploy checklist" })).toBeVisible({ timeout: 15_000 });
  await expect(consoles.getByRole("listitem").first()).toContainText("deploy checklist");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-osc-title.png`, fullPage: true });
  }
  await page.keyboard.press("Enter");

  // Ask for a notification after a delay, then leave the pane. The unread
  // mark must appear on the pane we are no longer looking at.
  await page.keyboard.type("sleep 3; printf '\\033]777;notify;Build finished;main.go compiled\\a'");
  await page.keyboard.press("Enter");
  await openLocalShell(page);
  await expect(consoles.getByRole("listitem")).toHaveCount(2);
  const first = consoles.getByRole("listitem").first();
  await expect(first.getByLabel("Unread notification")).toBeVisible({ timeout: 20_000 });
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/terminal-osc-unread.png`, fullPage: true });
  }

  // Showing the pane again reads the notification.
  await first.getByRole("button").first().click();
  await expect(consoles.getByLabel("Unread notification")).toHaveCount(0);
});
