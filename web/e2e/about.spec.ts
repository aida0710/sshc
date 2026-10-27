import AxeBuilder from "@axe-core/playwright";
import { expect, openApplication, test } from "./support/environment";

test("opens About from the menu and version link, checks for updates on opening and reads plain release notes", async ({ page, installation }) => {
  const current = "v1.0.0";
  let checks = 0;
  const checked = () => checks > 0;
  const status = () => ({
    current, latest: checked() ? "v1.2.0" : "v1.1.0", available: true, canUpdate: false, reason: "update_unmanaged",
    checkedAt: checked() ? "2026-10-19T02:00:00Z" : "2026-10-19T01:00:00Z",
    releases: [
      { version: checked() ? "v1.2.0" : "v1.1.0", body: checked() ? "# Notes\n<script>window.aboutInjected = true</script>\n  plain text" : "Stored release notes", pageUrl: "https://github.com/aida0710/sshc/releases/tag/v1.1.0", publishedAt: "2026-10-18T00:00:00Z" },
      { version: current, body: "Current release notes", pageUrl: "https://github.com/aida0710/sshc/releases/tag/v1.0.0" },
    ],
  });
  await page.route("**/api/v1/about", (route) => route.fulfill({ json: {
    version: current, revision: "abcdef0123456789abcdef0123456789abcdef0123", goVersion: "go1.26.9", os: "linux", arch: "amd64", manager: "unknown", executable: "/fixture/sshc",
  } }));
  await page.route("**/api/v1/update", (route) => route.fulfill({ json: status() }));
  await page.route("**/api/v1/update/check", async (route) => {
    expect(route.request().method()).toBe("POST");
    checks += 1;
    await route.fulfill({ json: status() });
  });
  await openApplication(page, installation);
  await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "Version v1.0.0" }).click();
  await expect(page).toHaveURL(/\/about$/);
  const about = page.getByRole("region", { name: "About sshc", exact: true });
  await expect(about.getByText("/fixture/sshc")).toBeVisible();
  await expect(about.getByRole("link", { name: "abcdef0" })).toHaveAttribute("href", /\/commit\/abcdef/);
  await expect(about.getByRole("heading", { level: 3 })).toHaveText(["Build", "Updates", "Release notes"]);
  // Opening the page checks GitHub once, so it shows the release found now, not the stored one.
  const progress = about.getByRole("log", { name: "Check and update progress" });
  await expect(progress.getByRole("listitem")).toHaveText([/Checking GitHub for the latest release…/, /The latest release on GitHub is v1\.2\.0\./]);
  await expect(progress.locator("time").first()).toHaveAttribute("datetime", /^\d{4}-\d{2}-\d{2}T/);
  expect(checks).toBe(1);
  await expect(about.getByText("Stored release notes")).toHaveCount(0);
  await expect(about.locator("details").first()).toHaveAttribute("open", "");
  await expect(about.getByText("# Notes", { exact: false })).toContainText("<script>window.aboutInjected = true</script>");
  await expect(about.locator("script")).toHaveCount(0);
  await expect(about.locator("details").last()).not.toHaveAttribute("open", "");
  await about.locator("details").last().locator("summary").click();
  await expect(about.getByText("Current release notes")).toBeVisible();
  // Each further check leaves the time it started and what it found, even when nothing else on the page changes.
  await about.getByRole("button", { name: "Check for updates" }).click();
  await expect(progress.getByRole("listitem")).toHaveCount(4);
  expect(checks).toBe(2);
  // Include the progress log after a repeated check in the accessibility audit.
  const audit = await new AxeBuilder({ page }).include("main").withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"]).analyze();
  expect(audit.violations.filter((violation) => violation.impact === "serious" || violation.impact === "critical").map((violation) => violation.id)).toEqual([]);
  await page.getByRole("navigation", { name: "Primary" }).getByRole("link", { name: "Menu", exact: true }).click();
  await page.getByRole("link", { name: "Open About sshc" }).click();
  await expect(about).toBeVisible();
  await expect.poll(() => checks).toBe(3);
});
