import { expect, openApplication, test } from "./support/environment";

test("confirms Homebrew updates, streams brew's output while it runs and shows the installed version once after restarting", async ({ page, installation }) => {
  let started = false;
  // finished turns true once the test has seen brew's output while the job still installs.
  let finished = false;
  const brewOutput = "==> Upgrading aida0710/tap/sshc\n  1.0.0 -> 1.2.0\n";
  const currentStatus = () => finished
    ? { current: "v1.2.0", available: false, canUpdate: false, manager: "homebrew", job: { id: "homebrew-job", target: "v1.1.0", installedVersion: "v1.2.0", state: "succeeded", problem: "" } }
    : started
      ? { current: "v1.0.0", latest: "v1.1.0", available: true, canUpdate: false, manager: "homebrew", job: { id: "homebrew-job", target: "v1.1.0", state: "installing", problem: "" } }
      : { current: "v1.0.0", latest: "v1.1.0", available: true, canUpdate: true, manager: "homebrew", pageUrl: "https://github.com/aida0710/sshc/releases/tag/v1.1.0" };
  // About checks GitHub on opening; the check finds the same release.
  await page.route("**/api/v1/update/check", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(currentStatus()) });
  });
  await page.route("**/api/v1/update", async (route) => {
    if (route.request().method() === "POST") {
      expect(route.request().postDataJSON()).toEqual({ target: "v1.1.0" });
      started = true;
      await route.fulfill({ status: 202, contentType: "application/json", body: JSON.stringify({ id: "homebrew-job", target: "v1.1.0", state: "accepted", problem: "" }) });
      return;
    }
    await route.fulfill({ contentType: "application/json", body: JSON.stringify(currentStatus()) });
  });
  await page.route("**/api/v1/update/log?*", async (route) => {
    const cursor = Number(new URL(route.request().url()).searchParams.get("cursor") ?? "0");
    const output = brewOutput.slice(cursor);
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({
      jobId: "homebrew-job", reachedCapacity: false, output,
      cursor: { requested: cursor, start: cursor, next: brewOutput.length, end: brewOutput.length, truncated: false },
    }) });
  });
  await page.route("**/api/v1/update/preview", async (route) => {
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ current: "v1.0.0", target: "v1.1.0", manager: "homebrew", actionToken: "homebrew-confirmation" }) });
  });
  await openApplication(page, installation);
  const navigation = page.getByRole("navigation", { name: "Primary" });
  await expect(navigation.getByRole("link", { name: /v1\.1\.0 is available/ })).toBeVisible();
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await navigation.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/update-notice-open.png` });
    await navigation.getByRole("button", { name: "Toggle update notice" }).click();
    await expect(navigation.getByRole("link", { name: /v1\.1\.0 is available/ })).toBeHidden();
    await navigation.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/update-notice-folded.png` });
    await navigation.getByRole("button", { name: "Toggle update notice" }).click();
  }
  // The sidebar only leads to About; the update is confirmed and followed there.
  await navigation.getByRole("link", { name: "Update in About sshc" }).click();
  await expect(page).toHaveURL(/\/about$/);
  const about = page.getByRole("region", { name: "About sshc", exact: true });
  const openingCheck = [/Checking GitHub for the latest release…/, /The latest release on GitHub is v1\.1\.0\./];
  const progress = about.getByRole("log", { name: "Check and update progress" });
  await expect(progress.getByRole("listitem")).toHaveText(openingCheck);
  await about.getByRole("button", { name: "Update sshc", exact: true }).click();
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
  // brew's output appears while the job is still installing, not once it has ended.
  await expect(about.getByRole("log", { name: "Update output" })).toHaveText(brewOutput);
  await expect(progress.getByRole("listitem")).toHaveText([...openingCheck, /Updating to v1\.1\.0…/]);
  finished = true;
  await expect(progress.getByRole("listitem")).toHaveText([...openingCheck, /Updating to v1\.1\.0…/, /Updated to v1\.2\.0\./]);
  await expect(about.getByRole("log", { name: "Update output" })).toHaveText(brewOutput);
  const result = page.getByRole("dialog", { name: "sshc update" });
  await expect(result).toContainText("Updated to v1.2.0.");
  if (process.env.SSHC_VISUAL_DIR !== undefined) {
    await page.screenshot({ path: `${process.env.SSHC_VISUAL_DIR}/update-result-dialog.png` });
  }
  await result.getByRole("button", { name: "Close" }).click();
  await expect(result).toBeHidden();
  await expect(page.getByRole("link", { name: "Version v1.2.0" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Update sshc", exact: true })).toHaveCount(0);
  // The result opens once per update; it does not stay under the version.
  await page.reload();
  await expect(page.getByRole("link", { name: "Version v1.2.0" })).toBeVisible();
  await expect(page.getByRole("dialog", { name: "sshc update" })).toHaveCount(0);
  await expect(page.getByText("Updated to v1.2.0.")).toHaveCount(0);
});
