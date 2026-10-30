import { expect, openApplication, openLocalShell, openSettingsPage, test } from "./support/environment";
import { drawnRows, selectionMarks, terminalKeyboard } from "./support/terminal";

// A touch screen as large as a PC's that the browser reports as touch first,
// (hover: none) and (pointer: coarse), as a 2-in-1 Windows PC can.
test.use({ hasTouch: true, isMobile: true, viewport: { width: 1280, height: 800 } });

const url = "https://example.test/touch-mode";

test("fixes a touch-first PC to the mouse, so drag selection and link clicks work", async ({ page, context, installation }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await openApplication(page, installation);
  expect(await page.evaluate(() => matchMedia("(hover: none) and (pointer: coarse)").matches)).toBe(true);

  await openLocalShell(page);
  const terminal = page.getByRole("region", { name: /^Terminal for / });
  await expect(terminal).toContainText(/[$#%>]/, { timeout: 20_000 });
  await terminalKeyboard(page).focus();
  await page.keyboard.type(`printf '%s\\n' ${url}`);
  await page.keyboard.press("Enter");
  const urlRow = drawnRows(page).locator(":scope > div").filter({ hasText: /^https:\/\/example\.test\/touch-mode\s*$/ });
  await expect(urlRow).toBeVisible({ timeout: 20_000 });
  // Detection is on at first and follows the browser: the finger's layer covers the terminal.
  const overlay = page.locator(".sshc-select-overlay");
  await expect(overlay).toHaveCount(1);

  await terminal.getByRole("button", { name: "More terminal actions" }).click();
  const allTerminals = terminal.getByRole("group", { name: "All terminals" });
  await allTerminals.getByRole("menuitemcheckbox", { name: "Detect touch automatically" }).click();
  await allTerminals.getByRole("menuitemradio", { name: "Use by mouse" }).click();
  // The same terminal, without reconnecting: the output stays and only the layer goes.
  await expect(overlay).toHaveCount(0);
  await expect(urlRow).toBeVisible();

  const rows = await drawnRows(page).boundingBox();
  if (rows === null) throw new Error("the terminal rows are not visible");
  await page.mouse.move(rows.x + 2, rows.y + 2);
  await page.mouse.down();
  await page.mouse.move(rows.x + rows.width - 2, rows.y + rows.height - 2, { steps: 12 });
  await page.mouse.up();
  await expect(selectionMarks(page).first()).toBeAttached();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toContain("example.test/touch-mode");

  const row = await urlRow.boundingBox();
  if (row === null) throw new Error("the URL row is not visible");
  await page.mouse.click(row.x + 25, row.y + row.height / 2);
  const actions = page.getByRole("dialog", { name: "Terminal link actions" });
  await expect(actions.locator("p")).toHaveAttribute("title", url);
  await page.keyboard.press("Escape");

  // Settings > Terminal shows the same choice and turns detection back on at once, without Save.
  await openSettingsPage(page, "Terminal");
  const settings = page.getByRole("region", { name: "Terminal" });
  await settings.getByRole("tab", { name: "Behaviour", exact: true }).click();
  const detect = settings.getByRole("checkbox", { name: "Detect automatically" });
  await expect(detect).not.toBeChecked();
  await expect(settings.getByRole("combobox", { name: "Fixed input" })).toHaveValue("mouse");
  await detect.click();
  await expect(settings.getByRole("combobox", { name: "Fixed input" })).toBeDisabled();
  // The choice is kept in the preset this machine uses, and detection, the default, is not written.
  await expect.poll(async () => (JSON.parse(await installation.read("sshc/metadata.json")) as {
    presets: { interface?: { touchMode?: string } }[];
  }).presets[0]?.interface?.touchMode).toBeUndefined();
});
