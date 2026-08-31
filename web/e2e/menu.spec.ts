import { expect, openApplication, test } from "./support/environment";

test("keeps sessions in the sidebar and moves product navigation to the Menu page", async ({
  page,
  installation,
}) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await openApplication(page, installation);

  const navigation = page.getByRole("navigation", { name: "Primary" });
  await expect(navigation.getByRole("link", { name: "Home", exact: true })).toBeVisible();
  await expect(navigation.getByRole("link", { name: "Connections", exact: true })).toBeVisible();
  await expect(navigation.getByRole("link", { name: "SFTP", exact: true })).toBeVisible();
  await expect(navigation.getByRole("link", { name: "Menu", exact: true })).toBeVisible();
  await expect(navigation.getByText("Sessions", { exact: true })).toBeVisible();
  await expect(navigation.getByRole("tab")).toHaveCount(0);
  await expect(navigation.getByRole("link", { name: "Terminal", exact: true })).toHaveCount(0);

  await navigation.getByRole("link", { name: "Menu", exact: true }).click();
  await expect(page).toHaveURL(/\/menu$/);
  await expect(page.getByRole("heading", { name: "Menu", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Open Home", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Open Connections", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Open SFTP", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Open Settings", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Open Engine", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Open Terminal", exact: true })).toHaveAttribute(
    "href",
    "/settings/terminal",
  );

  const visualDirectory = process.env.SSHC_VISUAL_DIR;
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-menu-page-desktop.png`, fullPage: true });
  }
  await page.setViewportSize({ width: 360, height: 800 });
  await expect(navigation).not.toBeInViewport();
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-menu-page-mobile.png`, fullPage: true });
  }

  await page.getByRole("button", { name: "Navigation", exact: true }).click();
  await expect(navigation).toBeInViewport();
  await expect.poll(async () => Math.round((await navigation.boundingBox())?.x ?? -1)).toBe(0);
  if (visualDirectory !== undefined) {
    await page.screenshot({ path: `${visualDirectory}/sshc-sessions-drawer-mobile.png`, fullPage: true });
  }
  await navigation.getByRole("button", { name: "Local shell", exact: true }).click();
  await expect(page).toHaveURL(/\/terminal$/);
  await expect(page.getByRole("region", { name: /^Console for / })).toBeVisible();
});
