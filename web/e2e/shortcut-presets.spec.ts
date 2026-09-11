import { mkdir } from "node:fs/promises";
import { expect, openApplication, openSection, test } from "./support/environment";

test("migrates shortcuts and shares presets while keeping browser choices independent", async ({ page, browser, context, installation }) => {
  await openApplication(page, installation);
  await page.evaluate(() => {
    localStorage.setItem("sshc.shortcuts.v1", JSON.stringify({ palette: ["Alt+K"] }));
    localStorage.removeItem("sshc.shortcuts.selected.v1");
  });
  await page.reload();
  await openSection(page, "Menu");
  await page.getByRole("link", { name: "Open Keyboard shortcuts" }).click();
  const selector = page.getByRole("combobox", { name: "Preset for this browser" });
  await expect(selector.locator("option:checked")).toHaveText("Imported shortcuts");
  await expect(page.getByRole("button", { name: "Assign shortcut: Command search", exact: true })).toHaveText("Alt+K");
  await page.getByRole("textbox", { name: "Preset name", exact: true }).fill("Work laptop");
  await page.getByRole("button", { name: "Rename", exact: true }).click();
  await expect(selector.locator("option:checked")).toHaveText("Work laptop");
  const id = await selector.inputValue();
  const auth = await context.storageState();
  for (const origin of auth.origins) origin.localStorage = origin.localStorage.filter((item) => !item.name.startsWith("sshc.shortcuts."));
  const second = await browser.newContext({ storageState: auth });
  try {
    const other = await second.newPage();
    await other.goto(page.url());
    const otherSelector = other.getByRole("combobox", { name: "Preset for this browser" });
    await expect(otherSelector).toBeEnabled();
    await expect(otherSelector).toHaveValue("default");
    await otherSelector.selectOption(id);
    await expect(other.getByRole("button", { name: "Assign shortcut: Command search", exact: true })).toHaveText("Alt+K");
    const assign = page.getByRole("button", { name: "Assign shortcut: Command search", exact: true });
    await assign.click(); await page.keyboard.press("Alt+j");
    await expect(other.getByRole("button", { name: "Assign shortcut: Command search", exact: true })).toHaveText("Alt+J");
    await otherSelector.selectOption("default");
    await expect(selector).toHaveValue(id);
    await page.getByRole("textbox", { name: "Preset name", exact: true }).fill("Desktop");
    await page.getByRole("button", { name: "Save a copy", exact: true }).click();
    await expect(selector.locator("option:checked")).toHaveText("Desktop");
    if (process.env.SSHC_PRESETS_VISUAL_DIR) {
      await mkdir(process.env.SSHC_PRESETS_VISUAL_DIR, { recursive: true });
      await page.setViewportSize({ width: 1440, height: 1050 });
      await page.screenshot({ path: `${process.env.SSHC_PRESETS_VISUAL_DIR}/shortcut-presets.png`, fullPage: true });
    }
    await selector.selectOption(id);
    await otherSelector.selectOption(id);
    await page.getByRole("button", { name: "Delete preset", exact: true }).click();
    await page.getByRole("button", { name: "Delete everywhere", exact: true }).click();
    await expect(selector).toHaveValue("default");
    await expect(otherSelector).toHaveValue("default");
    await page.reload();
    await expect(selector).toHaveValue("default");
    expect(JSON.parse(await installation.read("sshc/metadata.json")).shortcutPresets).toHaveLength(1);
  } finally { await second.close(); }
});
