import { expect, type Locator } from "@playwright/test";

// fixTouchMode は、ターミナル terminal の［…］のプリセットの設定で、タッチの自動判定を切り、タッチモードを mode に
// 固定する。固定したタッチモードは、同じプリセットを使うターミナルのすべてに効く。
export async function fixTouchMode(terminal: Locator, mode: "Use by touch" | "Use by mouse"): Promise<void> {
  await terminal.getByRole("button", { name: "More terminal actions", exact: true }).click();
  const preset = terminal.page().getByRole("menu", { name: "More terminal actions" }).getByRole("group", { name: /^Preset: / });
  const detect = preset.getByRole("menuitemcheckbox", { name: "Detect touch automatically" });
  if (await detect.isChecked()) await detect.click();
  await preset.getByRole("menuitemradio", { name: mode }).click();
  // タッチで使うターミナルには、指で選ぶための文字の層が重なる。
  const fingerLayers = terminal.page().locator(".sshc-select-overlay");
  if (mode === "Use by touch") await expect(fingerLayers).not.toHaveCount(0);
  else await expect(fingerLayers).toHaveCount(0);
}
