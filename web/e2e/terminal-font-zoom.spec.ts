import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { drawnRowFont, screenRect } from "./support/terminal";

const fontSizeOf = async (page: import("@playwright/test").Page) => Number.parseFloat((await drawnRowFont(page)).fontSize);

test("the wheel turned with Ctrl over a terminal changes the size of its text and not the page's, without saving it", async ({ page, installation }) => {
  await openApplication(page, installation);
  await openLocalShell(page);
  await expect(page.getByRole("region", { name: /^Terminal for / })).toBeVisible();
  const before = await fontSizeOf(page);
  const saves: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET" && /\/api\/v1\/(settings|metadata)/u.test(new URL(request.url()).pathname)) saves.push(request.url());
  });
  const zoomBefore = await page.evaluate(() => window.visualViewport?.scale ?? 1);

  const screen = await screenRect(page);
  await page.mouse.move(screen.x + screen.width / 2, screen.y + screen.height / 2);
  await page.keyboard.down("Control");
  await page.mouse.wheel(0, -100);
  await page.mouse.wheel(0, -100);
  await page.keyboard.up("Control");

  await expect.poll(() => fontSizeOf(page)).toBe(before + 2);
  expect(await page.evaluate(() => window.visualViewport?.scale ?? 1)).toBe(zoomBefore);

  // The size is this view's: nothing of the settings is saved.
  expect(saves).toEqual([]);
});
