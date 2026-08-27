import { expect, test } from "./support/environment";
import { openApplication, openSection, sessionStatus } from "./support/environment";

const sections = [
  "Home",
  "Connections",
  "SFTP",
  "Snippets",
  "Config",
  "Groups",
  "Keys",
  "Known Hosts",
  "Install Key on Server",
  "Ad hoc checks",
  "Secrets",
  "Sync",
  "History",
];

for (const appearance of ["light", "dark"] as const) {
  test(`every section renders in ${appearance}`, async ({ page, installation }) => {
    await openApplication(page, installation);

    await expect(page.locator("[data-session-status-badge]")).toBeVisible();
    await page.getByLabel("Appearance").selectOption(appearance);
    await expect(page.locator("html")).toHaveAttribute("data-theme", appearance);
    await expect(sessionStatus(page)).toContainText("Local session active");

    for (const name of sections) {
      await openSection(page, name);

      await expect(page.locator("html")).toHaveAttribute("data-theme", appearance);
      await expect(page.locator("main")).toBeVisible();

      const painted = await page.evaluate(() => {
        const shell = document.querySelector("main");
        if (shell === null) return null;
        const style = window.getComputedStyle(shell);
        const body = window.getComputedStyle(document.body);
        return { colour: style.color, background: body.backgroundColor };
      });
      expect(painted).not.toBeNull();
      expect(painted?.colour).not.toBe(painted?.background);
      expect(painted?.colour).not.toBe("rgba(0, 0, 0, 0)");
    }

    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.1-appearance-${appearance}.png`,
        fullPage: true,
      });
    }
  });

  test(`structural borders and small supporting text keep their contrast in ${appearance}`, async ({ page, installation }) => {
    await openApplication(page, installation);
    await page.getByLabel("Appearance").selectOption(appearance);

    const contrast = await page.evaluate(() => {
      const root = getComputedStyle(document.documentElement);
      const rgb = (name: string) => {
        const value = root.getPropertyValue(name).trim();
        const match = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(value);
        if (match === null) throw new Error(`unexpected colour ${name}: ${value}`);
        return match.slice(1).map((part) => Number.parseInt(part, 16) / 255);
      };
      const luminance = (colour: number[]) => {
        const linear = colour.map((channel) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4);
        return 0.2126 * linear[0]! + 0.7152 * linear[1]! + 0.0722 * linear[2]!;
      };
      const ratio = (left: string, right: string) => {
        const values = [luminance(rgb(left)), luminance(rgb(right))].sort((a, b) => b - a);
        return (values[0]! + 0.05) / (values[1]! + 0.05);
      };
      return {
        cardBorder: ratio("--ui-line", "--ui-card"),
        controlBorder: ratio("--ui-control-line", "--ui-control"),
        faintOnCard: ratio("--ui-ink-faint", "--ui-card"),
        faintOnSidebar: ratio("--ui-ink-faint", "--ui-sidebar"),
      };
    });

    expect(contrast.cardBorder).toBeGreaterThanOrEqual(1.4);
    expect(contrast.controlBorder).toBeGreaterThanOrEqual(3);
    expect(contrast.faintOnCard).toBeGreaterThanOrEqual(4.5);
    expect(contrast.faintOnSidebar).toBeGreaterThanOrEqual(4.5);

    await openSection(page, "Connections");
    await page
      .getByRole("navigation", { name: "Connections" })
      .getByRole("button", { name: "bastion", exact: true })
      .click();
    const summary = page.locator("[data-connection-summary]");
    await expect(summary).toHaveCSS("border-left-width", "1px");
    await expect(summary).toHaveCSS("border-right-width", "1px");
    await expect(summary).toHaveCSS("border-radius", "12px");
    await expect(page.locator("[data-connection-editor] input").first()).toHaveCSS("border-radius", "8px");
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-connection-summary-${appearance}.png`,
        fullPage: true,
      });
    }

    await page.getByRole("tab", { name: "Settings analysis" }).click();
    await expect(page.getByRole("tabpanel", { name: "Settings analysis" })).toBeVisible();
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-connection-analysis-${appearance}.png`,
        fullPage: true,
      });
    }

    await page.getByRole("tab", { name: "Advanced settings" }).click();
    await expect(page.getByRole("tabpanel", { name: "Advanced settings" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Advanced settings" })).toHaveAttribute("aria-selected", "true");
    await page.waitForTimeout(250);
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-connection-advanced-jump-${appearance}.png`,
        fullPage: true,
      });
    }
    for (const subview of ["Directives", "Raw"] as const) {
      await page.getByRole("tab", { name: subview }).click();
      await expect(page.getByRole("tab", { name: subview })).toHaveAttribute("aria-selected", "true");
      await page.waitForTimeout(250);
      if (process.env.SSHC_VISUAL_DIR !== undefined) {
        await page.screenshot({
          path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-connection-advanced-${subview.toLowerCase()}-${appearance}.png`,
          fullPage: true,
        });
      }
    }

    await openSection(page, "Ad hoc checks");
    const diagnosticCards = page.locator("main .sshc-card");
    await expect(diagnosticCards).toHaveCount(2);
    for (const card of await diagnosticCards.all()) {
      await expect(card).toHaveCSS("border-left-width", "1px");
      await expect(card).toHaveCSS("border-right-width", "1px");
    }
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-diagnostics-${appearance}.png`,
        fullPage: true,
      });
    }

    await openSection(page, "Config");
    const explorer = page.locator("[data-config-explorer]");
    await expect(explorer).toHaveCSS("border-left-width", "1px");
    await expect(explorer).toHaveCSS("border-right-width", "1px");
    await expect(explorer).toHaveCSS("border-radius", "12px");
    const treeHeader = await page.locator('[data-explorer-header="tree"]').boundingBox();
    const fileHeader = await page.locator('[data-explorer-header="file"]').boundingBox();
    expect(treeHeader?.height).toBe(fileHeader?.height);
    if (process.env.SSHC_VISUAL_DIR !== undefined) {
      await page.screenshot({
        path: `${process.env.SSHC_VISUAL_DIR}/sshc-v0.16.2-config-explorer-${appearance}.png`,
        fullPage: true,
      });
    }
  });
}

test("the connections controls are legible in light", async ({ page, installation }) => {
  await openApplication(page, installation);
  await page.getByLabel("Appearance").selectOption("light");
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");

  await openSection(page, "Connections");
  await page.getByRole("button", { name: "bastion", exact: true }).click();
  await expect(page.getByRole("heading", { name: "bastion", level: 2 })).toBeVisible();
  await page.getByRole("button", { name: "New connection" }).click();

  const readable = await page.evaluate(() => {
    const results: { where: string; colour: string; background: string }[] = [];
    for (const selector of [
      "input#create-connection-name",
      "select#create-connection-group",
      "input#create-connection-hostname",
    ]) {
      const element = document.querySelector(selector);
      if (element === null) continue;
      const style = window.getComputedStyle(element);
      results.push({ where: selector, colour: style.color, background: style.backgroundColor });
    }
    return results;
  });

  expect(readable.map((control) => control.where)).toEqual([
    "input#create-connection-name",
    "select#create-connection-group",
    "input#create-connection-hostname",
  ]);
  for (const control of readable) {
    expect(control.colour, `${control.where} text`).not.toBe(control.background);
    expect(control.background, `${control.where} background`).not.toBe("rgb(28, 28, 30)");
  }
});
