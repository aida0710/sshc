import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { drawnSpan, terminalKeyboard } from "./support/terminal";

// The suite draws with the DOM renderer, which sets a joined run of cells apart in its own element.
for (const ligatures of [true, false]) {
  test(`draws -> ${ligatures ? "as one ligature with the setting on" : "a character at a time with the setting off"}`, async ({ page, installation }) => {
    test.skip(process.platform === "win32", "printf is POSIX shell syntax here");
    await installation.write("sshc/metadata.json", JSON.stringify({ schemaVersion: 9, embeddedTerminal: { ligatures, appearance: { font: "jetbrains-mono" } } }));
    await openApplication(page, installation);
    await openLocalShell(page);
    const screen = page.getByRole("region", { name: /^Terminal for / });
    await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
    await terminalKeyboard(page).focus();
    await page.keyboard.type("printf 'arrow: %s\\n' 'a -> b'");
    await page.keyboard.press("Enter");
    await expect(screen).toContainText("arrow: a -> b");

    const arrow = drawnSpan(page, "->").filter({ hasNotText: "arrow" });
    if (!ligatures) {
      await expect(arrow).toHaveCount(0);
      return;
    }
    await expect(arrow.first()).toHaveText("->");
    const features = await arrow.first().evaluate((element) => getComputedStyle(element).fontFeatureSettings);
    // Chrome writes an enabled feature without its value: "calt", "liga".
    expect(features).toMatch(/"calt"(?! 0)/);
  });
}
