import { mkdir } from "node:fs/promises";
import { expect, openApplication, openSection, test } from "./support/environment";
import { terminalKeyboard } from "./support/terminal";

test("edits shortcuts, keeps them after reload and uses them in a live terminal", async ({ page, context, installation }) => {
  const frames: string[] = [];
  page.on("websocket", (socket) => socket.on("framesent", ({ payload }) => {
    frames.push(typeof payload === "string" ? payload : payload.toString("utf8"));
  }));
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);
  await openSection(page, "Menu");
  await page.getByRole("link", { name: "Open Keyboard shortcuts" }).click();
  await expect(page).toHaveURL(/\/settings\/shortcuts$/);
  async function assign(action: string, chord: string) {
    await page.getByRole("button", { name: `Assign shortcut: ${action}`, exact: true }).click();
    await page.keyboard.press(chord);
  }
  await assign("Command search", "Control+f");
  await expect(page.getByRole("alert")).toContainText("Already assigned to Terminal search");
  await page.keyboard.press("Escape");
  await assign("Command search", "Alt+k");
  await assign("Terminal search", "Alt+f");
  await assign("Paste", "Alt+v");
  await assign("Open Home", "Alt+h");
  await page.reload();
  await expect(page.getByRole("button", { name: "Assign shortcut: Command search", exact: true })).toHaveText("Alt+K");
  if (process.env.SSHC_SHORTCUTS_VISUAL_DIR !== undefined) {
    await mkdir(process.env.SSHC_SHORTCUTS_VISUAL_DIR, { recursive: true });
    await page.screenshot({ path: `${process.env.SSHC_SHORTCUTS_VISUAL_DIR}/shortcuts.png`, fullPage: true });
  }
  const nav = page.getByRole("navigation", { name: "Primary" });
  await nav.getByRole("button", { name: "Local shell", exact: true }).click();
  await expect(page.getByRole("region", { name: /^Console for / })).toContainText(/[$#%>]/);
  await terminalKeyboard(page).focus();
  const beforePalette = frames.length;
  await page.keyboard.press("Alt+k");
  await expect(page.getByRole("dialog", { name: "Search sessions, hosts, files, snippets and settings" })).toBeVisible();
  expect(frames.slice(beforePalette).filter((frame) => frame.includes("\u001bk"))).toEqual([]);
  await page.keyboard.press("Escape");
  await terminalKeyboard(page).focus();
  await page.keyboard.press("Alt+f");
  await expect(page.getByRole("textbox", { name: "Search terminal output" })).toBeVisible();
  await page.getByRole("button", { name: "Close search", exact: true }).click();
  await page.evaluate(() => navigator.clipboard.writeText("echo shortcut-one\necho shortcut-two\n"));
  await terminalKeyboard(page).focus();
  await page.keyboard.press("Alt+v");
  await expect(page.getByRole("textbox", { name: "Edit paste" })).toHaveValue("echo shortcut-one\necho shortcut-two\n");
  // Navigation shortcuts cannot dismiss a confirmation or send its contents.
  await page.keyboard.press("Alt+h");
  await expect(page.getByRole("textbox", { name: "Edit paste" })).toBeVisible();
  await page.getByRole("dialog").getByRole("button", { name: "Cancel", exact: true }).click();
  await terminalKeyboard(page).focus();
  await page.keyboard.press("Alt+h");
  await expect(page).toHaveURL(/\/$/);
  await page.keyboard.press("Alt+PageDown");
  await expect(page).toHaveURL(/\/terminal$/);
  await nav.getByRole("button", { name: "Local shell", exact: true }).click();
  const rows = page.getByRole("list", { name: "Open consoles" }).getByRole("listitem");
  await expect(rows).toHaveCount(2);
  await page.keyboard.press("Alt+PageUp");
  await expect(rows.nth(0).locator('[aria-current="true"]')).toBeVisible();
  await page.keyboard.press("Alt+PageDown");
  await expect(rows.nth(1).locator('[aria-current="true"]')).toBeVisible();
});
