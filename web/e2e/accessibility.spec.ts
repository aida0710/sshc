import AxeBuilder from "@axe-core/playwright";
import { expect, openApplication, test } from "./support/environment";

async function expectNoSeriousAccessibilityViolations(page: Parameters<typeof openApplication>[0]) {
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  const violations = result.violations
    .filter((violation) => violation.impact === "serious" || violation.impact === "critical")
    .map((violation) => ({
      id: violation.id,
      impact: violation.impact,
      targets: violation.nodes.map((node) => node.target.join(" ")),
    }));
  expect(violations).toEqual([]);
}

test("primary pages have no serious automated accessibility violations", async ({ page, installation }) => {
  await openApplication(page, installation);

  await expectNoSeriousAccessibilityViolations(page);
  for (const path of ["/connections", "/sftp", "/menu", "/settings/terminal", "/licenses"]) {
    await page.goto(`${installation.url}${path}`);
    await expect(page.locator("main")).toBeVisible();
    await expectNoSeriousAccessibilityViolations(page);
  }
});
