import AxeBuilder from "@axe-core/playwright";
import { expect, openApplication, test } from "./support/environment";

async function expectNoSeriousAccessibilityViolations(page: Parameters<typeof openApplication>[0]) {
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    // The settings' preview of a terminal draws the colours of the palette chosen, whatever they are.
    .exclude("[data-terminal-preview]")
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
  // Within the page: the address of the installation carries the sign-in in its fragment, so a path added to it opens Home again.
  for (const path of ["/connections", "/files", "/terminal", "/menu", "/settings/terminal", "/about", "/licenses"]) {
    await page.evaluate((target) => {
      window.history.pushState(null, "", target);
      window.dispatchEvent(new PopStateEvent("popstate"));
    }, path);
    // Connections opens on its list of servers (connectionRoute).
    const shownPath = path === "/connections" ? "/connections/servers" : path;
    await expect(page).toHaveURL(new RegExp(`${shownPath}$`));
    await expect(page.locator("main")).toBeVisible();
    // A section's code loads on the first visit, and a placeholder holds its place until then.
    await expect(page.locator(".sshc-skeleton")).toHaveCount(0);
    await expectNoSeriousAccessibilityViolations(page);
  }
});
