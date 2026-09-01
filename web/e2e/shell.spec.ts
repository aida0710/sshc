import { expect, openSection, test, openApplication } from "./support/environment";

const manyHosts = Array.from(
  { length: 40 },
  (_unused, index) => `Host lab-${String(index).padStart(2, "0")}\n\tHostName 198.51.100.${index + 1}\n`,
).join("\n");

async function navigationFooterTopBorders(page: import("@playwright/test").Page): Promise<number> {
  const navigation = page.getByRole("navigation", { name: "Primary" });
  const version = navigation.getByText(/^Version /);
  await expect(version).toBeVisible();
  return version.evaluate((node) => {
    const navigation = node.closest("nav");
    let current: HTMLElement | null = node.parentElement;
    let borders = 0;
    while (current !== null && current !== navigation) {
      if (Number.parseFloat(getComputedStyle(current).borderTopWidth) > 0) borders += 1;
      current = current.parentElement;
    }
    return borders;
  });
}

async function stubUpdateStatus(page: import("@playwright/test").Page) {
  await page.route("**/api/v1/update", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ current: "test", available: false }),
    });
  });
}

test("draws one separator above the desktop navigation version", async ({ page, installation }) => {
  await stubUpdateStatus(page);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.setViewportSize({ width: 1440, height: 900 });
  }
  await openApplication(page, installation);

  const navigation = page.getByRole("navigation", { name: "Primary" });
  const brandMark = navigation.locator("[data-sshc-brand-mark]");
  await expect(brandMark).toBeVisible();
  await expect(brandMark).toHaveAttribute("viewBox", "0 0 512 512");
  await expect(navigation).not.toContainText(">_");
  await expect(navigation.getByText("Home", { exact: true }).first()).toBeVisible();
  await expect(page.locator("[data-app-header]")).toBeHidden();
  await expect.poll(() => navigationFooterTopBorders(page)).toBe(1);

  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/home-desktop.png`, fullPage: true });
  }
});

test("keeps the engine version visible when the release check fails", async ({ page, installation }) => {
  await page.route("**/api/v1/update", async (route) => {
    await route.fulfill({
      status: 502,
      contentType: "application/problem+json",
      body: JSON.stringify({ code: "update_check_failed", message: "request rejected" }),
    });
  });
  await openApplication(page, installation);

  const navigation = page.getByRole("navigation", { name: "Primary" });
  await expect(navigation.getByText(/^Version /)).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/version-with-update-failure.png` });
  }
});

test("shows safe diagnostics for a failed operation", async ({ page, installation }) => {
  await stubUpdateStatus(page);
  await page.route("**/api/v1/config/overview", async (route) => {
    await route.fulfill({
      status: 502,
      contentType: "application/problem+json",
      body: JSON.stringify({
        code: "config_read_failed",
        message: "request rejected",
        detail: "the workspace configuration could not be read",
      }),
    });
  });
  await openApplication(page, installation);

  await expect(page.getByRole("heading", { name: "The operation could not be completed" })).toBeVisible();
  await page.getByText("Show diagnostic details").click();
  const report = page.getByText(/Code: config_read_failed/);
  await expect(report).toContainText("Operation: GET /api/v1/config/overview");
  await expect(report).not.toContainText("?");

  const screenshotPath = process.env.SSHC_DIAGNOSTIC_SCREENSHOT;
  if (screenshotPath !== undefined) await page.screenshot({ path: screenshotPath, fullPage: true });
});

test("keeps the navigation heading and primary navigation still while a panel scrolls", async ({
  page,
  installation,
}) => {
  await installation.write("conf.d/20-lab.conf", manyHosts);
  await openApplication(page, installation);
  await openSection(page, "Connections");
  await expect(
    page.getByRole("navigation", { name: "Connections" }).getByRole("button", { name: "lab-00" }),
  ).toBeVisible();

  const heading = page.locator("[data-navigation-heading]");
  const results = page.locator("[data-connection-results]");
  const resting = await heading.boundingBox();
  expect(resting).not.toBeNull();

  const overflow = await results.evaluate((element) => element.scrollHeight - element.clientHeight);
  expect(overflow, "the fixture is not tall enough to scroll the list").toBeGreaterThan(0);

  const documentOverflow = await page.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement;
    return root.scrollHeight - root.clientHeight;
  });
  expect(documentOverflow, "the document scrolls, so the navigation can leave the viewport").toBe(0);

  const windowOffset = await page.evaluate(() => {
    window.scrollTo(0, 10_000);
    return window.scrollY;
  });
  expect(windowOffset).toBe(0);

  await results.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  expect(await results.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);

  expect(await heading.boundingBox()).toEqual(resting);
  await expect(page.getByRole("navigation", { name: "Primary" })).toBeInViewport();
  await expect(page.getByRole("link", { name: "Menu", exact: true })).toBeInViewport();

  await expect(
    page.getByRole("region", { name: "Connection results" }).getByRole("button", { name: "lab-39" }),
  ).toBeInViewport();
});

test("keeps start links fixed and sessions reachable when the viewport is short", async ({
  page,
  installation,
}) => {
  await page.setViewportSize({ width: 1280, height: 320 });
  await openApplication(page, installation);
  await openSection(page, "Connections");

  const navigation = page.getByRole("navigation", { name: "Primary" });
  const sections = navigation.locator("div.overflow-y-auto");
  const overflow = await sections.evaluate((element) => element.scrollHeight - element.clientHeight);
  expect(overflow).toBeGreaterThan(0);

  await expect(navigation.getByRole("link", { name: "Home", exact: true })).toBeInViewport();
  await expect(navigation.getByRole("link", { name: "Menu", exact: true })).toBeInViewport();
  await sections.evaluate((element) => element.scrollTo(0, element.scrollHeight));
  await expect(navigation.getByRole("button", { name: "Local shell", exact: true })).toBeInViewport();
  await expect(page.locator("[data-navigation-heading]")).toBeInViewport();
  expect(await sections.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
});

test("resizes and restores the permanent desktop navigation", async ({ page, installation }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await openApplication(page, installation);

  const navigation = page.getByRole("navigation", { name: "Primary" });
  const handle = page.getByRole("separator", { name: "Resize navigation" });
  await expect(navigation).toBeVisible();
  await expect(handle).toHaveAttribute("aria-valuenow", "240");

  const handleBox = await handle.boundingBox();
  expect(handleBox).not.toBeNull();
  if (handleBox === null) return;
  await page.mouse.move(handleBox.x + handleBox.width / 2, handleBox.y + handleBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(handleBox.x + handleBox.width / 2 + 80, handleBox.y + handleBox.height / 2);
  await page.mouse.up();

  await expect(handle).toHaveAttribute("aria-valuenow", "320");
  await expect
    .poll(() => navigation.evaluate((element) => Math.round(element.getBoundingClientRect().width)))
    .toBe(320);

  await page.reload();
  await expect(page.getByRole("separator", { name: "Resize navigation" })).toHaveAttribute(
    "aria-valuenow",
    "320",
  );

  await expect(page.getByRole("navigation", { name: "Primary" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Hide navigation" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Show navigation" })).toHaveCount(0);
  await expect(page.getByRole("separator", { name: "Resize navigation" })).toHaveAttribute(
    "aria-valuenow",
    "320",
  );
});
