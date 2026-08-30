import { join } from "node:path";
import { expect, openApplication, openSection, test } from "./support/environment";
import { terminalKeyboard, terminalScrollbarSlider } from "./support/terminal";

const visualDirectory = process.env.SSHC_VISUAL_DIR;

test("uses a thin rounded scrollbar for terminal scrollback", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await page.getByRole("navigation", { name: "Primary" }).getByRole("button", { name: "Local shell" }).click();

  const terminal = page.getByRole("region", { name: /^Console for / });
  await expect(terminal).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  await page.keyboard.type("i=1; while [ \"$i\" -le 120 ]; do printf 'scrollback-line-%03d\\n' \"$i\"; i=$((i+1)); done");
  await page.keyboard.press("Enter");
  await expect(terminal).toContainText("scrollback-line-120", { timeout: 20_000 });

  await terminal.hover();
  await page.mouse.wheel(0, -800);

  const slider = terminalScrollbarSlider(page);
  await expect(slider).toBeVisible();
  const scrollbar = await slider.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      width: style.width,
      radius: style.borderRadius,
    };
  });
  expect(scrollbar).toEqual({
    width: "6px",
    radius: "999px",
  });

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-modern-scrollbar-desktop.png"), fullPage: true });
  }
});

test("keeps terminal actions compact and exposes the new terminal settings", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const navigation = page.getByRole("navigation", { name: "Primary" });
  await navigation.getByRole("button", { name: "Local shell" }).click();

  const terminal = page.getByRole("region", { name: /^Console for / });
  await expect(terminal).toBeVisible();
  await expect(terminal.getByRole("button", { name: "Find" })).toBeVisible();
  await terminal.getByRole("button", { name: "More terminal actions" }).click();
  const menu = terminal.getByRole("menu", { name: "More terminal actions" });
  await expect(menu.getByRole("menuitem", { name: "Quick Commands" })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "Copy recent terminal context" })).toBeVisible();
  await expect(menu.getByRole("menuitemcheckbox", { name: "OSC 52" })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "Port forwarding" })).toHaveCount(0);

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-actions-desktop.png"), fullPage: true });
  }

  await page.keyboard.press("Escape");
  await openSection(page, "Settings");
  const settings = page.getByRole("region", { name: "Terminal" });
  await expect(settings.getByLabel("Browser scrollback (lines)")).toBeVisible();
  await expect(settings.getByLabel("Default local shell")).toBeVisible();
  await expect(settings.getByText("Allow OSC 52 clipboard writes by default")).toBeVisible();
  await expect(settings.getByText("Send the JIS ¥ key as backslash")).toBeVisible();

  if (visualDirectory !== undefined) {
    await settings.getByLabel("Browser scrollback (lines)").scrollIntoViewIfNeeded();
    await page.screenshot({ path: join(visualDirectory, "terminal-settings-desktop.png"), fullPage: true });
    await settings.getByText("Send the JIS ¥ key as backslash").scrollIntoViewIfNeeded();
    await page.screenshot({ path: join(visualDirectory, "terminal-input-settings-desktop.png"), fullPage: true });
  }

  await page.setViewportSize({ width: 360, height: 800 });
  await page.reload();
  await openSection(page, "Terminal");
  await expect(terminal).toBeVisible();
  await terminal.getByRole("button", { name: "More terminal actions" }).click();
  await expect(menu).toBeVisible();

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "terminal-actions-mobile.png"), fullPage: true });
  }
});

test("opens forwarding management only for an SSH terminal", async ({ page, installation }) => {
  const session = {
    id: "forwarding-preview",
    kind: "ssh",
    alias: "bastion",
    title: "bastion",
    startedAt: "2026-08-30T00:00:00Z",
    state: "connected",
    problem: "",
    forwards: [{
      id: "pf-1",
      kind: "dynamic",
      listen: "127.0.0.1:1080",
      to: "",
      problem: "",
      temporary: true,
    }],
  };
  await page.route("**/api/v1/terminal/sessions", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [session], maxSessions: 50 }) });
      return;
    }
    await route.continue();
  });
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const terminal = page.getByRole("region", { name: "Console for bastion" });
  await expect(terminal).toBeVisible();
  await terminal.getByRole("button", { name: "More terminal actions" }).click();
  await terminal.getByRole("menuitem", { name: "Port forwarding" }).click();
  const dialog = page.getByRole("dialog", { name: "Port forwarding" });
  await expect(dialog.getByText("socks5://127.0.0.1:1080")).toBeVisible();
  await expect(dialog.getByText(/Listeners are bound to this device only/)).toBeVisible();

  if (visualDirectory !== undefined) {
    await page.screenshot({ path: join(visualDirectory, "port-forwarding-live-desktop.png"), fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: join(visualDirectory, "port-forwarding-live-mobile.png"), fullPage: true });
  }
});
