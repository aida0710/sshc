import { mkdir } from "node:fs/promises";
import { expect, openApplication, openSection, test } from "./support/environment";

// Shortcut presets are shared by every machine; each settings preset chooses the one it uses.
test("shares shortcut presets between settings presets, each choosing the one it uses", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openSection(page, "Menu");
  await page.getByRole("link", { name: "Open Keyboard shortcuts" }).click();
  const selector = page.getByRole("combobox", { name: "Shortcut preset" });
  const presetToEdit = page.getByRole("combobox", { name: "Preset to edit" });
  const commandSearch = page.getByRole("button", { name: "Assign shortcut: Command search", exact: true });
  await expect(selector).toHaveValue("default");
  // Editing the default shortcuts creates a shortcut preset of one's own, chosen by the preset being edited.
  await commandSearch.click();
  await page.keyboard.press("Alt+k");
  await expect(selector.locator("option:checked")).toHaveText("My shortcuts");
  await expect(commandSearch).toHaveText("Alt+K");
  await page.getByRole("textbox", { name: "Preset name", exact: true }).fill("Work laptop");
  await page.getByRole("button", { name: "Rename", exact: true }).click();
  await expect(selector.locator("option:checked")).toHaveText("Work laptop");
  const id = await selector.inputValue();

  // A new settings preset starts as a copy, so it chooses the same shortcut preset, and can choose another.
  await page.getByRole("button", { name: "Actions for Standard" }).click();
  await page.getByRole("menuitem", { name: "New preset" }).click();
  const naming = page.getByRole("dialog", { name: "New preset from Standard" });
  await naming.getByRole("textbox", { name: "Preset name" }).fill("Phone");
  await naming.getByRole("button", { name: "New preset" }).click();
  await expect(presetToEdit.locator("option:checked")).toHaveText("Phone");
  await expect(selector).toHaveValue(id);
  await selector.selectOption("default");
  await expect(commandSearch).not.toHaveText("Alt+K");

  // The shortcut presets themselves are shared: a change made under one settings preset shows under the other.
  await presetToEdit.selectOption({ label: "Standard" });
  await expect(selector).toHaveValue(id);
  await commandSearch.click();
  await page.keyboard.press("Alt+j");
  await expect(commandSearch).toHaveText("Alt+J");
  await presetToEdit.selectOption({ label: "Phone" });
  await expect(selector).toHaveValue("default");
  await selector.selectOption(id);
  await expect(commandSearch).toHaveText("Alt+J");
  if (process.env.SSHC_PRESETS_VISUAL_DIR) {
    await mkdir(process.env.SSHC_PRESETS_VISUAL_DIR, { recursive: true });
    await page.setViewportSize({ width: 1440, height: 1050 });
    await page.screenshot({ path: `${process.env.SSHC_PRESETS_VISUAL_DIR}/shortcut-presets.png`, fullPage: true });
  }

  // Deleting a shortcut preset leaves the default shortcuts in force wherever it was chosen.
  await page.getByRole("button", { name: "Delete preset", exact: true }).click();
  await page.getByRole("button", { name: "Delete everywhere", exact: true }).click();
  await expect(selector).toHaveValue("default");
  await presetToEdit.selectOption({ label: "Standard" });
  await expect(selector).toHaveValue("default");
  await page.reload();
  await expect(selector).toHaveValue("default");
  expect(JSON.parse(await installation.read("sshc/metadata.json")).shortcutPresets ?? []).toHaveLength(0);
});
