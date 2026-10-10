import { expect, openApplication, test } from "./support/environment";

test("confirms Homebrew updates and displays the installed version after restarting", async ({ page, installation }) => {
  let started = false;
  await page.route("**/api/v1/update", async (route) => {
    if (route.request().method() === "POST") {
      expect(route.request().postDataJSON()).toEqual({ target: "v1.1.0" });
      started = true;
      await route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify({ id: "homebrew-job", target: "v1.1.0", state: "accepted", problem: "" }) });
      return;
    }
    const status = started
      ? { current: "v1.2.0", available: false, canUpdate: false, manager: "homebrew", job: { id: "homebrew-job", target: "v1.1.0", installedVersion: "v1.2.0", state: "succeeded", problem: "" } }
      : { current: "v1.0.0", latest: "v1.1.0", available: true, canUpdate: true, manager: "homebrew" };
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(status) });
  });
  await page.route("**/api/v1/update/preview", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ current: "v1.0.0", target: "v1.1.0", manager: "homebrew", actionToken: "homebrew-confirmation", actionExpiresAt: "2026-10-10T09:00:00Z" }) });
  });
  await openApplication(page, installation);
  await page.getByRole("button", { name: "Update sshc", exact: true }).click();
  const confirmation = page.getByRole("dialog", { name: "Update sshc?" });
  await expect(confirmation).toContainText("Homebrew");
  await expect(confirmation).toContainText("A newer version may be installed");
  await expect(confirmation).toContainText("disconnects all terminals and file transfers");
  await expect(confirmation.getByRole("button", { name: "Update sshc", exact: true })).toBeEnabled();
  expect(started).toBe(false);
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/homebrew-update-confirmation.png` });
  }
  await confirmation.getByRole("button", { name: "Update sshc", exact: true }).click();
  await expect(confirmation).toBeHidden();
  await page.reload();
  await expect(page.getByText("Updated to v1.2.0.", { exact: true })).toBeVisible();
  await expect(page.getByText("Version v1.2.0", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Update sshc", exact: true })).toHaveCount(0);
});
