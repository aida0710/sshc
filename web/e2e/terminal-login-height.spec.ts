import { expect, masterPassword, openApplication, openLocalShell, openSection, test } from "./support/environment";
import { maxTerminalUnusedHeightPx, terminalDrawingRects, terminalKeyboard } from "./support/terminal";
import { expectTerminalPTYSize, watchTerminalStream } from "./support/terminalStream";

test.use({ viewport: { width: 1440, height: 900 } });

test("preserves a hidden shell's size through vault unlock and fits it when shown", async ({ page, installation }) => {
  const observed = watchTerminalStream(page);
  await openApplication(page, installation);
  await openLocalShell(page);
  await expect(terminalKeyboard(page)).toBeAttached();
  await expect.poll(() => observed.sizes.length).toBeGreaterThan(0);
  await expectTerminalPTYSize(page, observed);
  const before = observed.sizes.at(-1)!;
  expect(before.rows).toBeGreaterThan(24);
  await openSection(page, "Home");
  await expect(terminalKeyboard(page)).toBeHidden();
  await page.keyboard.press("Control+k");
  await page.getByRole("searchbox", { name: "Search sessions, hosts, files, snippets and settings" }).fill("Lock the vault");
  await page.getByRole("option", { name: /Lock the vault/ }).click();
  await expect(page.getByLabel("Master password", { exact: true })).toBeVisible();
  const priorFits = observed.sizes.length;
  const priorStreams = observed.attachedStreams();
  await page.getByLabel("Master password", { exact: true }).fill(masterPassword);
  await page.getByRole("button", { name: "Open", exact: true }).click();
  await expect(page.getByRole("navigation", { name: "Primary" })).toBeVisible();
  await expect.poll(observed.attachedStreams).toBeGreaterThan(priorStreams);
  expect(observed.sizes.slice(priorFits)).toEqual([]);
  await openSection(page, "Terminal");
  await expect(terminalKeyboard(page)).toBeVisible();
  await expect.poll(() => observed.sizes.length).toBeGreaterThan(priorFits);
  expect(observed.sizes.at(-1)).toEqual(before);
  const { screen, host } = await terminalDrawingRects(page);
  expect(host.y + host.height).toBeLessThanOrEqual(900);
  expect(screen.y + screen.height).toBeLessThanOrEqual(host.y + host.height);
  expect(host.height - screen.height).toBeLessThan(maxTerminalUnusedHeightPx);
  await expectTerminalPTYSize(page, observed);
});
