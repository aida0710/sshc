import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { standardPresetMetadata } from "./support/settings";
import { drawnRowFont, terminalKeyboard } from "./support/terminal";

const sample = [
  "日本語の表示を確かめる。漢字、ひらがな、カタカナ、ＡＢＣ。",
  "path\\to\\file ~/src 0O1lI| {}[]() -> != ===",
  "┌──────────┬────────┐",
  "│ 接続先   │ 状態   │",
  "└──────────┴────────┘",
].join("\n");

test("draws Japanese with BIZ UDGothic and Latin letters with JetBrains Mono once the preset chooses the pair", async ({ page, installation }) => {
  test.skip(process.platform === "win32", "cat is a POSIX command here");
  await installation.write("sshc/metadata.json", standardPresetMetadata({ terminal: { appearance: { font: "jetbrains-mono-biz-udgothic" } } }));
  await writeFile(join(installation.home, "japanese.txt"), `${sample}\n`);
  await openApplication(page, installation);
  await openLocalShell(page);
  const screen = page.getByRole("region", { name: /^Terminal for / });
  await expect(screen).toContainText(/[$#%>]/, { timeout: 20_000 });
  // Both bundled families are fetched before the terminal uses the font, while the screen holds no Japanese yet: a
  // family that arrived with its first character would leave WebGL's texture atlas with what another font drew.
  await expect.poll(async () => (await drawnRowFont(page)).fontFamily).toMatch(/^"?JetBrains Mono"?, "?BIZ UDGothic"?,/);
  expect(await page.evaluate(() => [...document.fonts].filter((face) => face.status === "loaded").map((face) => face.family.replace(/"/g, ""))))
    .toEqual(expect.arrayContaining(["JetBrains Mono", "BIZ UDGothic"]));

  await terminalKeyboard(page).focus();
  await page.keyboard.type("cat ~/japanese.txt");
  await page.keyboard.press("Enter");
  await expect(screen).toContainText("path\\to\\file");
  await expect(screen).toContainText("接");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/biz-udgothic.png` });
  }
});
