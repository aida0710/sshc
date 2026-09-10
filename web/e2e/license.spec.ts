import { mkdir } from "node:fs/promises";
import { expect, openApplication, openSection, test } from "./support/environment";

test("opens License from Others at the bottom of Menu and reads notices without external requests", async ({ page, installation }) => {
  await page.route("https://**/*", (route) => route.abort());
  await openApplication(page, installation);
  await openSection(page, "Menu");
  const others = page.getByRole("region", { name: "Others" });
  await expect(others).toBeVisible();
  expect(await others.evaluate((node) => node === node.parentElement?.lastElementChild)).toBe(true);
  await others.getByRole("link", { name: "Open License" }).click();
  await expect(page).toHaveURL(/\/license$/);
  await page.getByRole("searchbox").fill("Devicon");
  await page.locator("summary").filter({ hasText: "Devicon" }).focus();
  await page.keyboard.press("Enter");
  await expect(page.locator("pre").filter({ hasText: "Copyright (c) 2015 konpa" })).toBeVisible();
  if (process.env.SSHC_LICENSE_VISUAL_DIR !== undefined) {
    await mkdir(process.env.SSHC_LICENSE_VISUAL_DIR, { recursive: true });
    await page.screenshot({ path: `${process.env.SSHC_LICENSE_VISUAL_DIR}/license.png`, fullPage: true });
  }
  await page.reload();
  await expect(page.getByRole("heading", { name: "License", exact: true })).toBeVisible();
  await page.getByRole("searchbox").fill("JetBrains Mono");
  await page.locator("summary").filter({ hasText: "JetBrains Mono" }).click();
  await expect(page.locator("pre").filter({ hasText: "SIL OPEN FONT LICENSE" })).toBeVisible();
});
