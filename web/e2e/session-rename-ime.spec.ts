import { expect, openApplication, openLocalShell, openSection, test } from "./support/environment";

for (const composition of [{ isComposing: true }, { isComposing: false, keyCode: 229 }]) {
  test(`renames a session only after IME composition ends (${JSON.stringify(composition)})`, async ({ page, installation }) => {
    await openApplication(page, installation);
    await openSection(page, "Terminal");
    await openLocalShell(page);
    const navigation = page.getByRole("navigation", { name: "Primary" });
    await navigation.getByRole("button", { name: /^Actions for / }).click();
    await page.getByRole("menuitem", { name: "Rename", exact: true }).click();
    const input = navigation.getByRole("textbox", { name: /^New name for / });
    await input.dispatchEvent("compositionstart");
    await input.fill("開発サーバ");
    await input.dispatchEvent("keydown", { key: "Enter", ...composition });
    await expect(input).toBeFocused();
    await expect(input).toHaveValue("開発サーバ");
    await input.dispatchEvent("keydown", { key: "Escape", ...composition });
    await expect(input).toBeFocused();
    await input.dispatchEvent("compositionend", { data: "開発サーバ" });

    await input.press("Enter");
    await expect(input).toHaveCount(0);
    await expect(navigation.getByRole("button", { name: "開発サーバ", exact: true })).toBeVisible();
    await page.reload();
    await expect(navigation.getByRole("button", { name: "開発サーバ", exact: true })).toBeVisible();
  });
}
