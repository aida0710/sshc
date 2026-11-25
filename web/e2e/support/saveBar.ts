import type { Page, Response } from "@playwright/test";
import { expect } from "./environment";

// saveBar は、変更があるあいだ画面の下に固定される、保存するか破棄するかを選ぶ帯。
export function saveBar(page: Page) {
  return page.locator("[data-save-bar]");
}

export function saveButton(page: Page) {
  return saveBar(page).getByRole("button", { name: "Save", exact: true });
}

// saveEverything は、帯から保存し、書き終えて帯が消えるまで待つ。
export async function saveEverything(page: Page): Promise<void> {
  await saveButton(page).click();
  await expect(saveBar(page)).toHaveCount(0);
}

// saveSettings は、設定の画面の変更を帯から保存し、その書き込みの応答を返す。part は書き込む宛先（プリセットか全体）。
export async function saveSettings(page: Page, part: "preset" | "global" = "preset"): Promise<Response> {
  const path = part === "preset" ? "/api/v1/settings/presets/" : "/api/v1/settings/global";
  const [response] = await Promise.all([
    page.waitForResponse((candidate) =>
      new URL(candidate.url()).pathname.startsWith(path) && candidate.request().method() === "PUT"),
    saveEverything(page),
  ]);
  expect(response.ok()).toBe(true);
  return response;
}
